package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
)

// AniList browsing (schedule, seasons, related anime), list editing and the
// saved-episodes overview.

var (
	seasons      = map[string]bool{"WINTER": true, "SPRING": true, "SUMMER": true, "FALL": true}
	seasonSorts  = map[string]bool{"POPULARITY_DESC": true, "SCORE_DESC": true, "TRENDING_DESC": true, "START_DATE": true}
	mediaFormats = map[string]bool{"TV": true, "TV_SHORT": true, "MOVIE": true, "ONA": true, "OVA": true, "SPECIAL": true}
)

func currentSeason(t time.Time) string {
	return [...]string{"WINTER", "WINTER", "WINTER", "SPRING", "SPRING", "SPRING",
		"SUMMER", "SUMMER", "SUMMER", "FALL", "FALL", "FALL"}[t.Month()-1]
}

// seasonal lists a season's anime: ?season=FALL&year=2026&sort=SCORE_DESC&format=TV,ONA&page=2.
// Without parameters it is the current season by popularity.
func (s *Server) seasonal(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	q := r.URL.Query()
	sq := anilist.SeasonQuery{Season: currentSeason(now), Year: now.Year(), Sort: "POPULARITY_DESC", Page: 1}
	if v := strings.ToUpper(q.Get("season")); v != "" {
		if !seasons[v] {
			writeErr(w, http.StatusBadRequest, "season must be WINTER, SPRING, SUMMER or FALL")
			return
		}
		sq.Season = v
	}
	if v := q.Get("year"); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil || y < 1940 || y > now.Year()+2 {
			writeErr(w, http.StatusBadRequest, "bad year")
			return
		}
		sq.Year = y
	}
	if v := strings.ToUpper(q.Get("sort")); v != "" {
		if !seasonSorts[v] {
			writeErr(w, http.StatusBadRequest, "bad sort")
			return
		}
		sq.Sort = v
	}
	if v := q.Get("format"); v != "" {
		for _, f := range strings.Split(strings.ToUpper(v), ",") {
			if !mediaFormats[f] {
				writeErr(w, http.StatusBadRequest, "bad format "+f)
				return
			}
			sq.Format = append(sq.Format, f)
		}
	}
	if v := q.Get("page"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 20 {
			writeErr(w, http.StatusBadRequest, "bad page")
			return
		}
		sq.Page = p
	}
	ms, hasNext, err := s.AniList.Seasonal(r.Context(), sq)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"season": sq.Season, "year": sq.Year, "media": ms, "hasNext": hasNext,
	})
}

// schedule lists episodes airing between ?from= and ?to= (Unix seconds),
// at most eight days apart. The browser picks the window, so weeks start
// at midnight in the viewer's own time zone.
func (s *Server) schedule(w http.ResponseWriter, r *http.Request) {
	from, err1 := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, err2 := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err1 != nil || err2 != nil || from <= 0 || to <= from || to-from > 8*24*3600 {
		writeErr(w, http.StatusBadRequest, "from and to must be Unix seconds, at most eight days apart")
		return
	}
	items, err := s.AniList.Schedule(r.Context(), from, to)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) extras(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return
	}
	ex, err := s.AniList.Extras(r.Context(), id)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

func (s *Server) saveEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return
	}
	token, v := s.session(r.Context())
	if v == nil {
		writeErr(w, http.StatusUnauthorized, "not logged in")
		return
	}
	var u anilist.EntryUpdate
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeErr(w, http.StatusBadRequest, "bad entry: "+err.Error())
		return
	}
	switch {
	case u.Status != nil && !anilist.Statuses[*u.Status]:
		writeErr(w, http.StatusBadRequest, "bad status")
		return
	case u.Progress != nil && *u.Progress < 0, u.Repeat != nil && *u.Repeat < 0:
		writeErr(w, http.StatusBadRequest, "progress and rewatches cannot be negative")
		return
	case u.Score != nil && (*u.Score < 0 || *u.Score > 100):
		writeErr(w, http.StatusBadRequest, "bad score")
		return
	}
	entry, err := s.AniList.SaveEntry(r.Context(), token, id, u)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return
	}
	token, v := s.session(r.Context())
	if v == nil {
		writeErr(w, http.StatusUnauthorized, "not logged in")
		return
	}
	// The entry ID is needed, so read the entry fresh.
	s.AniList.Forget(id)
	m, err := s.AniList.Media(r.Context(), token, id)
	if err != nil {
		writeAniListErr(w, err)
		return
	}
	if m.MediaListEntry != nil {
		if err := s.AniList.DeleteEntry(r.Context(), token, m.MediaListEntry.ID); err != nil {
			writeAniListErr(w, err)
			return
		}
	}
	s.AniList.Forget(id)
	w.WriteHeader(http.StatusNoContent)
}

// saved describes the episodes kept on disk.
func (s *Server) saved(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Media   *anilist.Media `json:"media"`
		Episode float64        `json:"episode"`
		Bytes   int64          `json:"bytes"`
		SavedAt int64          `json:"savedAt"`
		UsedAt  int64          `json:"usedAt"`
	}
	out := map[string]any{
		"enabled":  s.Cfg.PrefetchDir != "",
		"maxBytes": int64(s.Cfg.PrefetchMaxGB) << 30,
		"ahead":    s.Cfg.PrefetchAhead,
		"items":    []item{},
	}
	if s.Cfg.PrefetchDir == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ds, err := s.Store.Downloads(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := []item{}
	var used int64
	for _, d := range ds {
		used += d.Bytes
		// Public media is cached for an hour, and saved shows are on the
		// Watching list, which fills that cache.
		m, err := s.AniList.Media(r.Context(), "", d.AnilistID)
		if err != nil {
			m = &anilist.Media{ID: d.AnilistID}
		}
		items = append(items, item{m, d.Episode, d.Bytes, d.SavedAt.Unix(), d.UsedAt.Unix()})
	}
	out["items"], out["usedBytes"] = items, used
	writeJSON(w, http.StatusOK, out)
}
