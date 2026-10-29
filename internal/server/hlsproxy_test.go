package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vermilion10/nonton/internal/store"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Server{Store: st}
}

func TestRewritePlaylist(t *testing.T) {
	s := testServer(t)
	base, _ := url.Parse("https://cdn.example/hls/abc/master.m3u8?t=1")
	pl := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=1\n" +
		"low/index.m3u8?t=1\n" +
		`#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=2,URI="https://cdn.example/hls/abc/iframes.m3u8?t=1"` + "\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="/keys/k1"` + "\n" +
		"seg0.ts\n"
	out := s.rewritePlaylist(pl, base)

	for _, want := range []string{
		"https://cdn.example/hls/abc/low/index.m3u8?t=1",
		"https://cdn.example/hls/abc/iframes.m3u8?t=1",
		"https://cdn.example/keys/k1",
		"https://cdn.example/hls/abc/seg0.ts",
	} {
		if !strings.Contains(out, s.relayURL(want)) {
			t.Errorf("missing relayed %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\nlow/index.m3u8") || strings.Contains(out, "\nseg0.ts") {
		t.Errorf("unrewritten URI left in:\n%s", out)
	}
	if !strings.HasPrefix(out, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n") {
		t.Errorf("tags changed:\n%s", out)
	}
}

func TestRelaySignatureIsStable(t *testing.T) {
	s := testServer(t)
	a := s.sign("https://cdn.example/x")
	if a != s.sign("https://cdn.example/x") || a == s.sign("https://cdn.example/y") {
		t.Fatal("signature not deterministic per URL")
	}
	// A new server on the same database keeps the key.
	s2 := &Server{Store: s.Store}
	if s2.sign("https://cdn.example/x") != a {
		t.Fatal("key not persisted")
	}
}

func httptestRecorder(h http.Handler, path string) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code
}
