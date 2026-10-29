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

func (s *Server) providerAnime(ctx context.Context, id string) (*provider.Anime, error) {
	s.mu.Lock()
	m, ok := s.animeMemo[id]
	s.mu.Unlock()
	if ok && time.Since(m.at) < animeTTL {
		return m.a, nil
	}
	a, err := s.Provider.Anime(ctx, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.animeMemo[id] = memo{a: a, at: time.Now()}
	s.mu.Unlock()
	return a, nil
}

// mapping returns the stored mapping for m, matching automatically the first
// time. When there is no confident match it returns nil and the candidates.
func (s *Server) mapping(ctx context.Context, m *anilist.Media) (*store.Mapping, []match.Candidate, error) {
	mp, err := s.Store.Mapping(ctx, m.ID, s.Provider.Name())
	if err != nil || mp != nil {
		return mp, nil, err
	}
	s.mu.Lock()
	missAt, missed := s.missMemo[m.ID]
	s.mu.Unlock()
	if missed && time.Since(missAt) < missTTL {
		return nil, nil, nil
	}
	cands, err := match.Find(ctx, s.Provider, *m)
	if err != nil {
		return nil, nil, err
	}
	if len(cands) > 0 && cands[0].Score >= match.Threshold {
		mp = &store.Mapping{AnilistID: m.ID, Provider: s.Provider.Name(), ProviderID: cands[0].ID, Title: cands[0].Title}
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
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return
	}
	ctx := r.Context()
	token, _ := s.session(ctx)
	m, err := s.AniList.Media(ctx, token, id)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	resp := map[string]any{"media": m, "mapping": nil, "episodes": []episodeJSON{}}
	mp, cands, err := s.mapping(ctx, m)
	switch {
	case err != nil:
		resp["sourceError"] = err.Error()
	case mp == nil:
		resp["candidates"] = candidatesJSON(cands, 5)
	default:
		resp["mapping"] = mappingJSON{mp.ProviderID, mp.Title, mp.EpOffset, mp.Manual}
		pa, err := s.providerAnime(ctx, mp.ProviderID)
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

// candidates lists provider shows for picking a mapping by hand. With ?q= it
// searches that text; without it, it reruns the automatic search.
func (s *Server) candidates(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return
	}
	ctx := r.Context()
	token, _ := s.session(ctx)
	m, err := s.AniList.Media(ctx, token, id)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	var cands []match.Candidate
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		rs, err := s.Provider.Search(ctx, q)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		for _, res := range rs {
			cands = append(cands, match.Candidate{Result: res, Score: match.Score(*m, res)})
		}
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	} else if cands, err = match.Find(ctx, s.Provider, *m); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, candidatesJSON(cands, 20))
}

func (s *Server) saveMapping(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
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
	pa, err := s.providerAnime(r.Context(), body.ProviderID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "cannot load that show: "+err.Error())
		return
	}
	mp := store.Mapping{AnilistID: id, Provider: s.Provider.Name(), ProviderID: body.ProviderID,
		Title: pa.Title, EpOffset: body.EpOffset, Manual: true}
	if err := s.Store.SaveMapping(r.Context(), mp); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	delete(s.missMemo, id)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, mappingJSON{mp.ProviderID, mp.Title, mp.EpOffset, mp.Manual})
}

// deleteMapping forgets the mapping so the next visit matches again.
func (s *Server) deleteMapping(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return
	}
	if err := s.Store.DeleteMapping(r.Context(), id, s.Provider.Name()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	delete(s.missMemo, id)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	ep, err := strconv.ParseFloat(r.PathValue("ep"), 64)
	if !ok || err != nil {
		writeErr(w, http.StatusBadRequest, "bad anime id or episode")
		return
	}
	ctx := r.Context()
	mp, err := s.Store.Mapping(ctx, id, s.Provider.Name())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if mp == nil {
		writeErr(w, http.StatusNotFound, "this anime is not linked to a source yet")
		return
	}
	pa, err := s.providerAnime(ctx, mp.ProviderID)
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
	streams, err := s.Provider.Streams(ctx, *pe)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"episode": ep, "title": pe.Title, "streams": streams})
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
