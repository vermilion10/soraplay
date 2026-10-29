package server

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/match"
	"github.com/vermilion10/nonton/internal/provider"
	"github.com/vermilion10/nonton/internal/store"
)

type mappingJSON struct {
	ProviderID string  `json:"providerId"`
	Title      string  `json:"title"`
	EpOffset   float64 `json:"epOffset"`
	Manual     bool    `json:"manual"`
}

type sourceJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type episodeJSON struct {
	Number float64 `json:"number"` // AniList numbering
	Title  string  `json:"title"`
}

type candidateJSON struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Episodes int     `json:"episodes"`
	Poster   string  `json:"poster"`
	Score    float64 `json:"score"`
}

// providerFor picks the source for an anime: adult entries come from the
// adult source when one is configured, everything else from the main one.
func (s *Server) providerFor(m *anilist.Media) provider.Provider {
	if m.IsAdult && s.AdultProvider != nil {
		return s.AdultProvider
	}
	return s.Provider
}

// animeAndProvider loads the AniList entry in the request path and its source.
func (s *Server) animeAndProvider(w http.ResponseWriter, r *http.Request) (*anilist.Media, provider.Provider, bool) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return nil, nil, false
	}
	token, _ := s.session(r.Context())
	m, err := s.AniList.Media(r.Context(), token, id)
	if err != nil {
		writeAniListErr(w, err)
		return nil, nil, false
	}
	return m, s.providerFor(m), true
}

func (s *Server) providerAnime(ctx context.Context, p provider.Provider, id string) (*provider.Anime, error) {
	key := p.Name() + "/" + id
	s.mu.Lock()
	m, ok := s.animeMemo[key]
	s.mu.Unlock()
	if ok && time.Since(m.at) < animeTTL {
		return m.a, nil
	}
	a, err := p.Anime(ctx, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.animeMemo[key] = memo{a: a, at: time.Now()}
	s.mu.Unlock()
	return a, nil
}

// mapping returns the stored mapping for m on p, matching automatically the
// first time. When there is no confident match it returns nil and the candidates.
func (s *Server) mapping(ctx context.Context, p provider.Provider, m *anilist.Media) (*store.Mapping, []match.Candidate, error) {
	mp, err := s.Store.Mapping(ctx, m.ID, p.Name())
	if err != nil || mp != nil {
		return mp, nil, err
	}
	s.mu.Lock()
	missAt, missed := s.missMemo[m.ID]
	s.mu.Unlock()
	if missed && time.Since(missAt) < missTTL {
		return nil, nil, nil
	}
	cands, err := match.Find(ctx, p, *m)
	if err != nil {
		return nil, nil, err
	}
	if len(cands) > 0 && cands[0].Score >= match.Threshold {
		mp = &store.Mapping{AnilistID: m.ID, Provider: p.Name(), ProviderID: cands[0].ID, Title: cands[0].Title}
		if err := s.Store.SaveMapping(ctx, *mp); err != nil {
			return nil, nil, err
		}
		return mp, nil, nil
	}
	s.mu.Lock()
	s.missMemo[m.ID] = time.Now()
	s.mu.Unlock()
	return nil, cands, nil
}

func (s *Server) anime(w http.ResponseWriter, r *http.Request) {
	m, p, ok := s.animeAndProvider(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	resp := map[string]any{
		"media":    m,
		"source":   sourceJSON{p.Name(), p.Label()},
		"mapping":  nil,
		"episodes": []episodeJSON{},
	}
	mp, cands, err := s.mapping(ctx, p, m)
	switch {
	case err != nil:
		resp["sourceError"] = err.Error()
	case mp == nil:
		resp["candidates"] = candidatesJSON(cands, 5)
	default:
		resp["mapping"] = mappingJSON{mp.ProviderID, mp.Title, mp.EpOffset, mp.Manual}
		pa, err := s.providerAnime(ctx, p, mp.ProviderID)
		if err != nil {
			resp["sourceError"] = err.Error()
			break
		}
		var eps []episodeJSON
		for _, e := range pa.Episodes {
			if n := e.Number - mp.EpOffset; n > 0 {
				eps = append(eps, episodeJSON{Number: n, Title: e.Title})
			}
		}
		if eps != nil {
			resp["episodes"] = eps
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func candidatesJSON(cs []match.Candidate, limit int) []candidateJSON {
	out := []candidateJSON{}
	for i, c := range cs {
		if i == limit {
			break
		}
		out = append(out, candidateJSON{c.ID, c.Title, c.Episodes, c.Poster, math.Round(c.Score*100) / 100})
	}
	return out
}

// candidates lists source shows for picking a mapping by hand. With ?q= it
// searches that text; without it, it reruns the automatic search.
func (s *Server) candidates(w http.ResponseWriter, r *http.Request) {
	m, p, ok := s.animeAndProvider(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var cands []match.Candidate
	var err error
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		rs, err := p.Search(ctx, q)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		for _, res := range rs {
			cands = append(cands, match.Candidate{Result: res, Score: match.Score(*m, res)})
		}
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	} else if cands, err = match.Find(ctx, p, *m); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, candidatesJSON(cands, 20))
}

func (s *Server) saveMapping(w http.ResponseWriter, r *http.Request) {
	m, p, ok := s.animeAndProvider(w, r)
	if !ok {
		return
	}
	var body struct {
		ProviderID string  `json:"providerId"`
		EpOffset   float64 `json:"epOffset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProviderID == "" {
		writeErr(w, http.StatusBadRequest, "providerId is required")
		return
	}
	pa, err := s.providerAnime(r.Context(), p, body.ProviderID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "cannot load that show: "+err.Error())
		return
	}
	mp := store.Mapping{AnilistID: m.ID, Provider: p.Name(), ProviderID: body.ProviderID,
		Title: pa.Title, EpOffset: body.EpOffset, Manual: true}
	if err := s.Store.SaveMapping(r.Context(), mp); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	delete(s.missMemo, m.ID)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, mappingJSON{mp.ProviderID, mp.Title, mp.EpOffset, mp.Manual})
}

// deleteMapping forgets the mapping so the next visit matches again.
func (s *Server) deleteMapping(w http.ResponseWriter, r *http.Request) {
	m, p, ok := s.animeAndProvider(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteMapping(r.Context(), m.ID, p.Name()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	delete(s.missMemo, m.ID)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	ep, err := strconv.ParseFloat(r.PathValue("ep"), 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad episode number")
		return
	}
	m, p, ok := s.animeAndProvider(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	mp, err := s.Store.Mapping(ctx, m.ID, p.Name())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if mp == nil {
		writeErr(w, http.StatusNotFound, "this anime is not linked to a source yet")
		return
	}
	pa, err := s.providerAnime(ctx, p, mp.ProviderID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	var pe *provider.Episode
	for i := range pa.Episodes {
		if pa.Episodes[i].Number == ep+mp.EpOffset {
			pe = &pa.Episodes[i]
		}
	}
	if pe == nil {
		writeErr(w, http.StatusNotFound, "episode is not on the source yet")
		return
	}
	streams, err := p.Streams(ctx, *pe)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	for i := range streams {
		if streams[i].Relay || s.Cfg.RelayAll {
			streams[i].URL = s.relayURL(streams[i].URL)
		}
	}
	resume, err := s.Store.Position(ctx, m.ID, ep)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"episode": ep, "title": pe.Title, "streams": streams, "resume": resume})
}

// progress queues "episode watched" for AniList. The worker sends it.
func (s *Server) progress(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return
	}
	var body struct {
		Episode float64 `json:"episode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Episode < 1 {
		writeErr(w, http.StatusBadRequest, "episode must be 1 or more")
		return
	}
	// AniList counts whole episodes; a recap "12.5" counts as 12.
	if err := s.Store.QueueProgress(r.Context(), id, int(body.Episode)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Progress.Wake()
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}

// savePosition records where playback of an episode is, so another device
// can resume there.
func (s *Server) savePosition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	ep, err := strconv.ParseFloat(r.PathValue("ep"), 64)
	if !ok || err != nil {
		writeErr(w, http.StatusBadRequest, "bad anime id or episode")
		return
	}
	var body struct {
		Position float64 `json:"position"`
		Duration float64 `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Position < 0 || body.Duration <= 0 {
		writeErr(w, http.StatusBadRequest, "position and duration are required")
		return
	}
	p := store.Position{AnilistID: id, Episode: ep, Position: body.Position, Duration: body.Duration}
	if err := s.Store.SavePosition(r.Context(), p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
