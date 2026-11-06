package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/aniskip"
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

// sources lists the sources for an anime, preferred first: adult entries
// come from the adult source when one is configured, everything else from
// the main one and then the fallbacks.
func (s *Server) sources(m *anilist.Media) []provider.Provider {
	if m.IsAdult && s.AdultProvider != nil {
		return []provider.Provider{s.AdultProvider}
	}
	return append([]provider.Provider{s.Provider}, s.Fallbacks...)
}

// providerFor is the preferred source for an anime.
func (s *Server) providerFor(m *anilist.Media) provider.Provider {
	return s.sources(m)[0]
}

func (s *Server) sourcesJSON(m *anilist.Media) []sourceJSON {
	var out []sourceJSON
	for _, p := range s.sources(m) {
		out = append(out, sourceJSON{p.Name(), p.Label()})
	}
	return out
}

// animeAndProvider loads the AniList entry in the request path and the
// source named by ?source=, or the preferred one.
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
	name := r.URL.Query().Get("source")
	if name == "" {
		return m, s.providerFor(m), true
	}
	for _, p := range s.sources(m) {
		if p.Name() == name {
			return m, p, true
		}
	}
	writeErr(w, http.StatusBadRequest, "no source "+name+" for this anime")
	return nil, nil, false
}

func missKey(p provider.Provider, id int) string {
	return p.Name() + "/" + strconv.Itoa(id)
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
	missAt, missed := s.missMemo[missKey(p, m.ID)]
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
	s.missMemo[missKey(p, m.ID)] = time.Now()
	s.mu.Unlock()
	return nil, cands, nil
}

// anime is the anime page: the AniList entry and its episodes on a source.
// Without ?source=, a show the preferred source lacks (or has no episodes
// of) is shown from the next source that has it.
func (s *Server) anime(w http.ResponseWriter, r *http.Request) {
	m, p, ok := s.animeAndProvider(w, r)
	if !ok {
		return
	}
	resp, found := s.animeOn(r.Context(), m, p)
	if r.URL.Query().Get("source") == "" && !found {
		for _, alt := range s.sources(m)[1:] {
			if other, ok := s.animeOn(r.Context(), m, alt); ok {
				resp = other
				break
			}
		}
	}
	resp["sources"] = s.sourcesJSON(m)
	writeJSON(w, http.StatusOK, resp)
}

// animeOn describes m on source p; found reports whether p has episodes of it.
func (s *Server) animeOn(ctx context.Context, m *anilist.Media, p provider.Provider) (map[string]any, bool) {
	found := false
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
			found = true
		}
	}
	return resp, found
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
	delete(s.missMemo, missKey(p, m.ID))
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
	delete(s.missMemo, missKey(p, m.ID))
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
	named := r.URL.Query().Get("source") != ""
	var title string
	var streams []provider.Stream
	used := sourceJSON{"saved", "Saved on the homelab"}
	if local := s.savedStream(ctx, m.ID, ep); local != nil && !named {
		title, streams = fmt.Sprintf("Episode %g", ep), []provider.Stream{*local}
	} else {
		// A named source is used alone; otherwise each source is tried in
		// turn, and the most telling error is reported.
		try := []provider.Provider{p}
		if !named {
			try = s.sources(m)
		}
		var firstErr error
		firstStatus := 0
		for _, src := range try {
			t, sts, status, err := s.sourceStreams(ctx, m, src, ep)
			if err == nil {
				title, streams, used = t, sts, sourceJSON{src.Name(), src.Label()}
				break
			}
			// "Not on this source" says less than a later source's real failure.
			if firstErr == nil || firstStatus == http.StatusNotFound {
				firstErr, firstStatus = err, status
			}
			log.Printf("stream: anime %d ep %g on %s: %v", m.ID, ep, src.Name(), err)
		}
		if streams == nil {
			writeErr(w, firstStatus, firstErr.Error())
			return
		}
		for i := range streams {
			if streams[i].Relay || s.Cfg.RelayAll {
				streams[i].URL = s.relayURL(streams[i].URL)
			}
		}
	}
	resume, err := s.Store.Position(ctx, m.ID, ep)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"episode": ep, "title": title, "streams": streams, "resume": resume,
		"source": used, "sources": s.sourcesJSON(m),
	})
}

// episodeSources tells, per source, whether it has episode ep: "ready" when
// it does, "missing" when it has the show but not that episode, and
// "unlinked" when no show on it was matched (one may be picked by hand).
// The player asks before offering another source.
func (s *Server) episodeSources(w http.ResponseWriter, r *http.Request) {
	ep, err := strconv.ParseFloat(r.PathValue("ep"), 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad episode number")
		return
	}
	m, _, ok := s.animeAndProvider(w, r)
	if !ok {
		return
	}
	type entry struct {
		sourceJSON
		Status string `json:"status"`
	}
	out := []entry{}
	for _, p := range s.sources(m) {
		out = append(out, entry{sourceJSON{p.Name(), p.Label()}, s.episodeStatus(r.Context(), m, p, ep)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) episodeStatus(ctx context.Context, m *anilist.Media, p provider.Provider, ep float64) string {
	mp, _, err := s.mapping(ctx, p, m)
	if err != nil || mp == nil {
		return "unlinked"
	}
	pa, err := s.providerAnime(ctx, p, mp.ProviderID)
	if err != nil {
		return "missing"
	}
	for _, e := range pa.Episodes {
		if e.Number == ep+mp.EpOffset {
			return "ready"
		}
	}
	return "missing"
}

// sourceStreams resolves episode ep (AniList numbering) of m on its source.
// On failure it also returns the HTTP status that fits the error.
func (s *Server) sourceStreams(ctx context.Context, m *anilist.Media, p provider.Provider, ep float64) (string, []provider.Stream, int, error) {
	// Match here too: the player can be opened straight from the home page
	// for an anime whose page was never visited.
	mp, _, err := s.mapping(ctx, p, m)
	if err != nil {
		return "", nil, http.StatusBadGateway, err
	}
	if mp == nil {
		return "", nil, http.StatusNotFound, errors.New("this anime is not linked to a source yet")
	}
	pa, err := s.providerAnime(ctx, p, mp.ProviderID)
	if err != nil {
		return "", nil, http.StatusBadGateway, err
	}
	for _, e := range pa.Episodes {
		if e.Number == ep+mp.EpOffset {
			streams, err := p.Streams(ctx, e)
			if err != nil {
				return "", nil, http.StatusBadGateway, err
			}
			return e.Title, streams, http.StatusOK, nil
		}
	}
	return "", nil, http.StatusNotFound, errors.New("episode is not on the source yet")
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

// skipTimes returns the opening and ending of an episode from AniSkip,
// fitted to the length of the video being played (?length= in seconds).
// Missing data is an empty answer, never an error: skipping is optional.
func (s *Server) skipTimes(w http.ResponseWriter, r *http.Request) {
	ep, err := strconv.ParseFloat(r.PathValue("ep"), 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad episode number")
		return
	}
	m, _, ok := s.animeAndProvider(w, r)
	if !ok {
		return
	}
	length, _ := strconv.ParseFloat(r.URL.Query().Get("length"), 64)
	times := &aniskip.Times{}
	if m.IDMal > 0 && s.AniSkip != nil {
		if t, err := s.AniSkip.Get(r.Context(), m.IDMal, ep, length); err == nil {
			times = t
		} else {
			log.Printf("aniskip: anime %d ep %g: %v", m.ID, ep, err)
		}
	}
	writeJSON(w, http.StatusOK, times)
}
