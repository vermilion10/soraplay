package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/httpc"
	"github.com/vermilion10/nonton/internal/prefetch"
	"github.com/vermilion10/nonton/internal/provider"
)

// Saving episodes ahead of time: the next unwatched episodes of shows
// watched recently are downloaded to PREFETCH_DIR, played from there, and
// deleted once watched or when the size cap needs room. Adult titles are
// never saved.

const (
	saveEvery = time.Hour
	// recentFor is how long after watching a show its next episodes are kept saved.
	recentFor = 14 * 24 * time.Hour
	// episodeEstimate is the space reserved for an episode not saved yet.
	episodeEstimate = 400 << 20
)

var mediaFileRe = regexp.MustCompile(`^[a-z0-9]+\.(m3u8|ts|m4s|mp4|aac|key|vtt)$`)

type wantKey struct {
	id int
	ep float64
}

func (s *Server) episodeDir(id int, ep float64) string {
	return filepath.Join(s.Cfg.PrefetchDir, strconv.Itoa(id), strconv.FormatFloat(ep, 'f', -1, 64))
}

// savedStream returns the local copy of an episode, or nil.
func (s *Server) savedStream(ctx context.Context, id int, ep float64) *provider.Stream {
	if s.Cfg.PrefetchDir == "" {
		return nil
	}
	ok, err := s.Store.HasDownload(ctx, id, ep)
	if err != nil || !ok {
		return nil
	}
	if _, err := os.Stat(filepath.Join(s.episodeDir(id, ep), "index.m3u8")); err != nil {
		s.Store.RemoveDownload(ctx, id, ep) // removed from disk by hand
		return nil
	}
	return &provider.Stream{
		URL:     fmt.Sprintf("/media/%d/%s/index.m3u8", id, strconv.FormatFloat(ep, 'f', -1, 64)),
		Type:    "hls",
		SubLang: "id-hardsub",
		Server:  "saved on the homelab",
	}
}

// media serves saved episode files.
func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	id, err1 := strconv.Atoi(r.PathValue("id"))
	ep, err2 := strconv.ParseFloat(r.PathValue("ep"), 64)
	file := r.PathValue("file")
	if s.Cfg.PrefetchDir == "" || err1 != nil || err2 != nil || !mediaFileRe.MatchString(file) {
		http.NotFound(w, r)
		return
	}
	if filepath.Ext(file) == ".m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=86400")
	}
	http.ServeFile(w, r, filepath.Join(s.episodeDir(id, ep), file))
}

// Save keeps upcoming episodes on disk, hourly until ctx ends. Start it after Handler.
func (s *Server) Save(ctx context.Context) {
	if s.Cfg.PrefetchDir == "" {
		return
	}
	t := time.NewTimer(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := s.saveOnce(ctx); err != nil {
			log.Printf("saver: %v", err)
		}
		t.Reset(saveEvery)
	}
}

func (s *Server) saveOnce(ctx context.Context) error {
	token, v := s.session(ctx)
	if v == nil {
		return nil
	}
	items, err := s.AniList.List(ctx, token, v.ID, []string{"CURRENT", "REPEATING"})
	if err != nil {
		return err
	}
	want, err := s.wanted(ctx, items)
	if err != nil {
		return err
	}

	// Drop what is no longer wanted: watched, or not watched lately.
	saved, err := s.Store.Downloads(ctx)
	if err != nil {
		return err
	}
	var used int64
	have := map[wantKey]bool{}
	for _, d := range saved {
		k := wantKey{d.AnilistID, d.Episode}
		if _, ok := want[k]; !ok {
			s.deleteSaved(ctx, d.AnilistID, d.Episode)
			continue
		}
		have[k] = true
		used += d.Bytes
	}

	byID := map[int]anilist.Media{}
	for _, it := range items {
		byID[it.Media.ID] = it.Media
	}
	limit := int64(s.Cfg.PrefetchMaxGB) << 30
	for k := range want {
		if have[k] {
			continue
		}
		if used+episodeEstimate > limit {
			log.Printf("saver: size cap of %d GB reached", s.Cfg.PrefetchMaxGB)
			return nil
		}
		m := byID[k.id]
		n, err := s.saveEpisode(ctx, &m, k.ep)
		if err != nil {
			log.Printf("saver: anime %d ep %g: %v", k.id, k.ep, err)
			continue
		}
		used += n
		log.Printf("saver: saved anime %d ep %g (%d MB)", k.id, k.ep, n>>20)
	}
	return nil
}

// wanted lists the episodes that should be on disk: the next PrefetchAhead
// unwatched episodes of non-adult shows watched in the last two weeks,
// where the source is known to have them.
func (s *Server) wanted(ctx context.Context, items []anilist.ListItem) (map[wantKey]bool, error) {
	positions, err := s.Store.LatestPositions(ctx)
	if err != nil {
		return nil, err
	}
	avail, err := s.Store.AllAvailability(ctx)
	if err != nil {
		return nil, err
	}
	since := time.Now().Add(-recentFor).Unix()
	want := map[wantKey]bool{}
	for _, it := range items {
		m := it.Media
		if m.IsAdult {
			continue
		}
		recent := it.UpdatedAt >= since
		if p, ok := positions[m.ID]; ok && p.UpdatedAt >= since {
			recent = true
		}
		a, known := avail[m.ID]
		if !recent || !known {
			continue
		}
		for k := 1; k <= s.Cfg.PrefetchAhead; k++ {
			ep := float64(it.Progress + k)
			if ep > a.LatestEp {
				break
			}
			want[wantKey{m.ID, ep}] = true
		}
	}
	return want, nil
}

func (s *Server) saveEpisode(ctx context.Context, m *anilist.Media, ep float64) (int64, error) {
	p := s.providerFor(m)
	_, streams, _, err := s.sourceStreams(ctx, m, p, ep)
	if err != nil {
		return 0, err
	}
	st := streams[0]
	f := &prefetch.Fetcher{
		HTTP:      &http.Client{Timeout: 2 * time.Minute},
		UserAgent: httpc.UserAgent,
		Headers:   st.Headers,
	}
	dir := s.episodeDir(m.ID, ep)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return 0, err
	}
	n, err := f.Download(ctx, st.URL, dir)
	if err != nil {
		return 0, err
	}
	return n, s.Store.AddDownload(ctx, m.ID, ep, n)
}

func (s *Server) deleteSaved(ctx context.Context, id int, ep float64) {
	if err := os.RemoveAll(s.episodeDir(id, ep)); err != nil {
		log.Printf("saver: delete anime %d ep %g: %v", id, ep, err)
		return
	}
	os.Remove(filepath.Dir(s.episodeDir(id, ep))) // the anime folder, once empty
	s.Store.RemoveDownload(ctx, id, ep)
	log.Printf("saver: deleted anime %d ep %g", id, ep)
}
