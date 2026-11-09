package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/store"
)

const (
	trackEvery = 30 * time.Minute
	// trackGap spaces source requests so a run never hammers a site.
	trackGap = 3 * time.Second
	// recheckFinished is how often a finished show still missing episodes on
	// the source is looked at again.
	recheckFinished = 24 * time.Hour
)

// Track checks the sources for new episodes of the shows on the Watching
// list, every half hour until ctx ends. Start it after Handler.
func (s *Server) Track(ctx context.Context) {
	t := time.NewTimer(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := s.trackOnce(ctx); err != nil {
			log.Printf("tracker: %v", err)
		}
		t.Reset(trackEvery)
	}
}

func (s *Server) trackOnce(ctx context.Context) error {
	token, v := s.session(ctx)
	if v == nil {
		return nil
	}
	items, err := s.AniList.List(ctx, token, v.ID, []string{"CURRENT", "REPEATING"})
	if err != nil {
		return err
	}
	avail, err := s.Store.AllAvailability(ctx)
	if err != nil {
		return err
	}
	for _, it := range items {
		m := it.Media
		a, seen := avail[m.ID]
		if seen && !needsCheck(m, a) {
			continue
		}
		if err := s.trackOne(ctx, it, seen); err != nil {
			log.Printf("tracker: anime %d: %v", m.ID, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(trackGap):
		}
	}
	return nil
}

// needsCheck says whether a show can have gained episodes since a.
func needsCheck(m anilist.Media, a store.Availability) bool {
	if m.Status == "RELEASING" {
		return true
	}
	if m.Episodes > 0 && a.LatestEp >= float64(m.Episodes) {
		return false // finished, and the source has every episode
	}
	return time.Since(a.CheckedAt) >= recheckFinished
}

func (s *Server) trackOne(ctx context.Context, it anilist.ListItem, seen bool) error {
	m := it.Media
	p := s.providerFor(&m)
	if p == nil {
		return nil
	}
	// Only shows still airing are matched here; others wait until their page is opened.
	mp, err := s.Store.Mapping(ctx, m.ID, p.Name())
	if err == nil && mp == nil && m.Status == "RELEASING" {
		mp, _, err = s.mapping(ctx, p, &m)
	}
	if err != nil || mp == nil {
		return err
	}
	pa, err := p.Anime(ctx, mp.ProviderID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.animeMemo[p.Name()+"/"+mp.ProviderID] = memo{a: pa, at: time.Now()}
	s.mu.Unlock()

	latest := 0.0
	for _, e := range pa.Episodes {
		latest = max(latest, e.Number-mp.EpOffset)
	}
	prev, err := s.Store.SetAvailability(ctx, m.ID, latest)
	if err != nil {
		return err
	}
	// The first check of a show only sets the baseline.
	if seen && latest > prev && latest > float64(it.Progress) {
		title := m.Title.UserPreferred
		if title == "" {
			title = m.Title.Romaji
		}
		s.notify(ctx, fmt.Sprintf("%s episode %g is up on %s\n%s/watch/%d/%g",
			title, latest, p.Label(), s.Cfg.PublicURL, m.ID, latest))
	}
	return nil
}

// notify posts a message to the Discord webhook, if one is set.
func (s *Server) notify(ctx context.Context, msg string) {
	log.Printf("new episode: %s", msg)
	if s.Cfg.DiscordWebhook == "" {
		return
	}
	body, _ := json.Marshal(map[string]string{"content": msg})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Cfg.DiscordWebhook, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("notify: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("notify: Discord answered %s", resp.Status)
	}
}
