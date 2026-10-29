package prefetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloadPicksBestVariantAndRewrites(t *testing.T) {
	var flaky atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=200000\nlow/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=900000\nhigh/index.m3u8?t=1\n"))
	})
	mux.HandleFunc("/high/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://site/" {
			http.Error(w, "no referer", http.StatusForbidden)
			return
		}
		w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:10,\nseg0\n#EXTINF:10,\n/abs/seg1.ts\n"))
	})
	mux.HandleFunc("/high/init.mp4", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("INIT")) })
	mux.HandleFunc("/high/seg0", func(w http.ResponseWriter, r *http.Request) {
		// Fails once, to exercise the retry.
		if flaky.Add(1) == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("SEG0"))
	})
	mux.HandleFunc("/abs/seg1.ts", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("SEG1!")) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "ep1")
	f := &Fetcher{HTTP: srv.Client(), UserAgent: "test", Headers: map[string]string{"Referer": "https://site/"}}
	n, err := f.Download(context.Background(), srv.URL+"/master.m3u8", dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len("INIT")+len("SEG0")+len("SEG1!")) {
		t.Errorf("bytes = %d", n)
	}
	pl, _ := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	want := "#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXT-X-MAP:URI=\"x00000.mp4\"\n#EXTINF:10,\ns00001.ts\n#EXTINF:10,\ns00002.ts\n#EXT-X-ENDLIST\n"
	if string(pl) != want {
		t.Errorf("playlist =\n%s\nwant\n%s", pl, want)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "s00002.ts")); string(b) != "SEG1!" {
		t.Errorf("segment content %q", b)
	}
	if _, err := os.Stat(dir + ".part"); !os.IsNotExist(err) {
		t.Error(".part folder left behind")
	}
}

func TestDownloadFailureLeavesNothing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXTINF:10,\nmissing.ts\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "ep2")
	f := &Fetcher{HTTP: srv.Client(), UserAgent: "test"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // skip the retry waits
	if _, err := f.Download(ctx, srv.URL+"/index.m3u8", dir); err == nil {
		t.Fatal("expected an error")
	}
	for _, p := range []string{dir, dir + ".part"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after a failed download", strings.TrimPrefix(p, filepath.Dir(dir)))
		}
	}
}
