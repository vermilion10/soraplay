// Package server is the HTTP API and the static frontend.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/config"
	"github.com/vermilion10/nonton/internal/progress"
	"github.com/vermilion10/nonton/internal/provider"
	"github.com/vermilion10/nonton/internal/store"
)

type Server struct {
	Cfg      config.Config
	Store    *store.Store
	AniList  *anilist.Client
	Provider provider.Provider
	// AdultProvider serves AniList entries marked adult; nil sends them to Provider.
	AdultProvider provider.Provider
	Progress      *progress.Worker
	Static        fs.FS // built frontend; index.html is served for unknown paths

	relayOnce     sync.Once
	relayKeyBytes []byte

	mu        sync.Mutex
	animeMemo map[string]memo // provider anime pages, keyed by provider ID
	missMemo  map[int]time.Time
}

type memo struct {
	a  *provider.Anime
	at time.Time
}

const (
	animeTTL = 10 * time.Minute // new episodes show up within this time
	missTTL  = 30 * time.Minute // how long a failed automatic match is remembered
)

func (s *Server) Handler() http.Handler {
	s.animeMemo = map[string]memo{}
	s.missMemo = map[int]time.Time{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/login", s.login)
	mux.HandleFunc("GET /auth/callback", s.callback)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/list", s.list)
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/seasonal", s.seasonal)
	mux.HandleFunc("GET /api/anime/{id}", s.anime)
	mux.HandleFunc("GET /api/anime/{id}/candidates", s.candidates)
	mux.HandleFunc("PUT /api/anime/{id}/mapping", s.saveMapping)
	mux.HandleFunc("DELETE /api/anime/{id}/mapping", s.deleteMapping)
	mux.HandleFunc("GET /api/anime/{id}/episodes/{ep}/stream", s.stream)
	mux.HandleFunc("POST /api/anime/{id}/progress", s.progress)
	mux.HandleFunc("GET /api/relay", s.relay)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "no such endpoint")
	})
	mux.Handle("/", s.static())
	return logRequests(privacyHeaders(requireJSON(mux)))
}

// privacyHeaders stops browsers from telling other sites about this app:
// with no-referrer, requests for video, cover images and posters carry no
// Referer, and cross-origin requests send "Origin: null" instead of the
// app's address.
func privacyHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// requireJSON rejects state-changing requests that are not JSON. Browsers
// cannot send a cross-site JSON request without a CORS preflight, which this
// server never grants, so this blocks cross-site request forgery.
func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeErr(w, http.StatusUnsupportedMediaType, "send application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		// Relayed video segments are left out: one every few seconds per viewer.
		if (strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/relay") || strings.HasPrefix(r.URL.Path, "/auth/") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := s.Static.Open(p); err == nil {
				f.Close()
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		// Client-side routes all load the app shell.
		index, err := fs.ReadFile(s.Static, "index.html")
		if err != nil {
			http.Error(w, "frontend not built: run npm run build in web/", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}

// ---- auth ----

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.ClientID == "" || s.Cfg.ClientSecret == "" {
		writeErr(w, http.StatusServiceUnavailable, "ANILIST_CLIENT_ID and ANILIST_CLIENT_SECRET are not set")
		return
	}
	http.Redirect(w, r, anilist.AuthorizeURL(s.Cfg.ClientID, s.Cfg.RedirectURI()), http.StatusFound)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		writeErr(w, http.StatusBadRequest, "missing code")
		return
	}
	ctx := r.Context()
	token, expires, err := s.AniList.ExchangeCode(ctx, s.Cfg.ClientID, s.Cfg.ClientSecret, s.Cfg.RedirectURI(), code)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	v, err := s.AniList.Viewer(ctx, token)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	user, _ := json.Marshal(v)
	for k, val := range map[string]string{
		"anilist_token":   token,
		"anilist_expires": strconv.FormatInt(expires.Unix(), 10),
		"anilist_user":    string(user),
	} {
		if err := s.Store.Set(ctx, k, val); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.Progress.Wake()
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Delete(r.Context(), "anilist_token", "anilist_expires", "anilist_user"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.AniList.ForgetUser()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// session returns the stored token and user, or "" and nil when logged out
// or expired.
func (s *Server) session(ctx context.Context) (string, *anilist.Viewer) {
	token, _ := s.Store.Get(ctx, "anilist_token")
	if token == "" {
		return "", nil
	}
	if exp, _ := s.Store.Get(ctx, "anilist_expires"); exp != "" {
		if n, err := strconv.ParseInt(exp, 10, 64); err == nil && time.Now().Unix() > n {
			return "", nil
		}
	}
	raw, _ := s.Store.Get(ctx, "anilist_user")
	var v anilist.Viewer
	if json.Unmarshal([]byte(raw), &v) != nil {
		return "", nil
	}
	return token, &v
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	_, v := s.session(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"user":       v,
		"configured": s.Cfg.ClientID != "" && s.Cfg.ClientSecret != "",
	})
}

// ---- AniList data ----

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	token, v := s.session(r.Context())
	if v == nil {
		writeErr(w, http.StatusUnauthorized, "not logged in")
		return
	}
	statuses := []string{"CURRENT", "REPEATING"}
	if st := r.URL.Query().Get("status"); st != "" {
		statuses = strings.Split(strings.ToUpper(st), ",")
	}
	items, err := s.AniList.List(r.Context(), token, v.ID, statuses)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	if items == nil {
		items = []anilist.ListItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []anilist.Media{})
		return
	}
	token, _ := s.session(r.Context())
	ms, err := s.AniList.Search(r.Context(), token, q)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

func (s *Server) seasonal(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	season := [...]string{"WINTER", "WINTER", "WINTER", "SPRING", "SPRING", "SPRING",
		"SUMMER", "SUMMER", "SUMMER", "FALL", "FALL", "FALL"}[now.Month()-1]
	token, _ := s.session(r.Context())
	ms, err := s.AniList.Seasonal(r.Context(), token, season, now.Year(), 30)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"season": season, "year": now.Year(), "media": ms})
}

// ---- helpers ----

func pathID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	return id, err == nil && id > 0
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeAniListErr(w http.ResponseWriter, err error) {
	var ae *anilist.Error
	if errors.As(err, &ae) {
		switch ae.Status {
		case http.StatusUnauthorized:
			writeErr(w, http.StatusUnauthorized, "AniList session expired, log in again")
			return
		case http.StatusTooManyRequests:
			w.Header().Set("Retry-After", "60")
			writeErr(w, http.StatusTooManyRequests, "AniList is limiting requests right now. Wait a minute and try again.")
			return
		case http.StatusNotFound:
			writeErr(w, http.StatusNotFound, "AniList has no anime with this ID.")
			return
		}
	}
	writeErr(w, http.StatusBadGateway, err.Error())
}
