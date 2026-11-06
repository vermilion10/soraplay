package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/vermilion10/nonton/internal/httpc"
)

// Some hosts tie a stream's token to the User-Agent and address that asked
// for it, so the viewer's browser cannot fetch it directly. For those, the
// server relays the playlists and segments itself. Every relayed URL is
// signed, so the relay only fetches URLs the server handed out.

var uriAttrRe = regexp.MustCompile(`URI="([^"]+)"`)

// relayClient has no overall timeout: a relayed MP4 can stream for as long
// as the episode, and ends with the viewer's request.
var relayClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ResponseHeaderTimeout: 30 * time.Second,
	IdleConnTimeout:       90 * time.Second,
}}

// relayKey returns the signing key, creating it on first use.
func (s *Server) relayKey() []byte {
	s.relayOnce.Do(func() {
		ctx := context.Background()
		if v, _ := s.Store.Get(ctx, "relay_key"); v != "" {
			if k, err := hex.DecodeString(v); err == nil {
				s.relayKeyBytes = k
				return
			}
		}
		k := make([]byte, 32)
		rand.Read(k)
		s.Store.Set(ctx, "relay_key", hex.EncodeToString(k))
		s.relayKeyBytes = k
	})
	return s.relayKeyBytes
}

func (s *Server) sign(u string) string {
	m := hmac.New(sha256.New, s.relayKey())
	m.Write([]byte(u))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// relayURL is the app URL that relays upstream URL u.
func (s *Server) relayURL(u string) string {
	return "/api/relay?u=" + url.QueryEscape(u) + "&s=" + s.sign(u)
}

func (s *Server) relay(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("u")
	sig := r.URL.Query().Get("s")
	if u == "" || !hmac.Equal([]byte(sig), []byte(s.sign(u))) {
		writeErr(w, http.StatusForbidden, "bad relay signature")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Header.Set("User-Agent", httpc.UserAgent)
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}
	resp, err := relayClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	if isPlaylist(u, resp.Header.Get("Content-Type")) && resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		base, _ := url.Parse(u)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, s.rewritePlaylist(string(body), base))
		return
	}

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func isPlaylist(u, contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "mpegurl") || strings.Contains(strings.ToLower(strings.SplitN(u, "?", 2)[0]), ".m3u8")
}

// rewritePlaylist points every URI in an HLS playlist at the relay.
func (s *Server) rewritePlaylist(pl string, base *url.URL) string {
	abs := func(ref string) string {
		if u, err := base.Parse(strings.TrimSpace(ref)); err == nil {
			return u.String()
		}
		return ref
	}
	lines := strings.Split(pl, "\n")
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "":
		case strings.HasPrefix(t, "#"):
			lines[i] = uriAttrRe.ReplaceAllStringFunc(l, func(m string) string {
				ref := uriAttrRe.FindStringSubmatch(m)[1]
				return `URI="` + s.relayURL(abs(ref)) + `"`
			})
		default:
			lines[i] = s.relayURL(abs(t))
		}
	}
	return strings.Join(lines, "\n")
}
