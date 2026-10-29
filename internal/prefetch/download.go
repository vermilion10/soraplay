// Package prefetch saves upcoming episodes to disk ahead of time, so they
// start instantly and play even when the source is slow or down.
package prefetch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	uriAttrRe   = regexp.MustCompile(`URI="([^"]+)"`)
	bandwidthRe = regexp.MustCompile(`BANDWIDTH=(\d+)`)
)

// Fetcher downloads one stream's files.
type Fetcher struct {
	HTTP      *http.Client
	UserAgent string
	Headers   map[string]string // sent with every request (e.g. Referer)
}

// Download saves the HLS stream at src into dir: the best variant's playlist
// as index.m3u8 next to its segments, renamed to local file names. Files go
// to dir+".part" first, which is renamed to dir only when everything is
// there, so dir never holds a partial episode. It returns the bytes written.
func (f *Fetcher) Download(ctx context.Context, src, dir string) (int64, error) {
	media, base, err := f.mediaPlaylist(ctx, src)
	if err != nil {
		return 0, err
	}
	part := dir + ".part"
	os.RemoveAll(part)
	if err := os.MkdirAll(part, 0o755); err != nil {
		return 0, err
	}
	var total int64
	fail := func(err error) (int64, error) {
		os.RemoveAll(part)
		return 0, err
	}

	var out []string
	n := 0
	save := func(ref, prefix string) (string, error) {
		u, err := base.Parse(strings.TrimSpace(ref))
		if err != nil {
			return "", err
		}
		name := fmt.Sprintf("%s%05d%s", prefix, n, ext(u.Path))
		n++
		size, err := f.saveFile(ctx, u.String(), filepath.Join(part, name))
		total += size
		return name, err
	}
	segments, ended := 0, false
	for _, line := range strings.Split(media, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			continue
		case t == "#EXT-X-ENDLIST":
			ended = true
		case strings.HasPrefix(t, "#EXT-X-MAP:") || strings.HasPrefix(t, "#EXT-X-KEY:"):
			if m := uriAttrRe.FindStringSubmatch(t); m != nil {
				name, err := save(m[1], "x")
				if err != nil {
					return fail(err)
				}
				t = strings.Replace(t, m[0], `URI="`+name+`"`, 1)
			}
		case !strings.HasPrefix(t, "#"):
			name, err := save(t, "s")
			if err != nil {
				return fail(err)
			}
			t = name
			segments++
		}
		out = append(out, t)
	}
	if segments == 0 {
		return fail(fmt.Errorf("prefetch: playlist has no segments"))
	}
	if !ended {
		out = append(out, "#EXT-X-ENDLIST")
	}
	if err := os.WriteFile(filepath.Join(part, "index.m3u8"), []byte(strings.Join(out, "\n")+"\n"), 0o644); err != nil {
		return fail(err)
	}
	os.RemoveAll(dir)
	if err := os.Rename(part, dir); err != nil {
		return fail(err)
	}
	return total, nil
}

// mediaPlaylist returns the media playlist for src, picking the variant
// with the highest bandwidth when src is a master playlist.
func (f *Fetcher) mediaPlaylist(ctx context.Context, src string) (string, *url.URL, error) {
	base, err := url.Parse(src)
	if err != nil {
		return "", nil, err
	}
	body, err := f.get(ctx, src)
	if err != nil {
		return "", nil, err
	}
	pl := string(body)
	if !strings.Contains(pl, "#EXT-X-STREAM-INF") {
		return pl, base, nil
	}
	best, bestBW := "", -1
	sc := bufio.NewScanner(strings.NewReader(pl))
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(t, "#EXT-X-STREAM-INF") {
			continue
		}
		bw := 0
		if m := bandwidthRe.FindStringSubmatch(t); m != nil {
			bw, _ = strconv.Atoi(m[1])
		}
		for sc.Scan() {
			if next := strings.TrimSpace(sc.Text()); next != "" && !strings.HasPrefix(next, "#") {
				if bw > bestBW {
					best, bestBW = next, bw
				}
				break
			}
		}
	}
	if best == "" {
		return "", nil, fmt.Errorf("prefetch: master playlist has no variants")
	}
	vu, err := base.Parse(best)
	if err != nil {
		return "", nil, err
	}
	body, err = f.get(ctx, vu.String())
	if err != nil {
		return "", nil, err
	}
	return string(body), vu, nil
}

func (f *Fetcher) request(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	for k, v := range f.Headers {
		req.Header.Set(k, v)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("prefetch: GET %s: %s", u, resp.Status)
	}
	return resp, nil
}

func (f *Fetcher) get(ctx context.Context, u string) ([]byte, error) {
	resp, err := f.request(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// saveFile downloads u to path, retrying a few times: one bad segment
// should not throw away a whole episode.
func (f *Fetcher) saveFile(ctx context.Context, u, path string) (int64, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(time.Duration(attempt*5) * time.Second):
			}
		}
		n, err := f.saveOnce(ctx, u, path)
		if err == nil {
			return n, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func (f *Fetcher) saveOnce(ctx context.Context, u, path string) (int64, error) {
	resp, err := f.request(ctx, u)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	file, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(file, resp.Body)
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	return n, err
}

// ext keeps a short, safe extension from the remote path, defaulting to .ts.
func ext(p string) string {
	e := strings.ToLower(filepath.Ext(p))
	switch e {
	case ".ts", ".m4s", ".mp4", ".aac", ".key", ".vtt":
		return e
	}
	return ".ts"
}
