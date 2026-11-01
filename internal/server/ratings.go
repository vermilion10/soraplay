package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/vermilion10/nonton/internal/store"
)

// Episode ratings are SoraPlay's own: AniList scores only whole anime.

const maxNote = 280 // characters

func (s *Server) ratings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad anime id")
		return
	}
	rs, err := s.Store.Ratings(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) saveRating(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	ep, err := strconv.ParseFloat(r.PathValue("ep"), 64)
	if !ok || err != nil || ep <= 0 {
		writeErr(w, http.StatusBadRequest, "bad anime id or episode")
		return
	}
	var body struct {
		Score int    `json:"score"`
		Note  string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Score < 1 || body.Score > 10 {
		writeErr(w, http.StatusBadRequest, "score must be 1 to 10")
		return
	}
	note := strings.TrimSpace(body.Note)
	if utf8.RuneCountInString(note) > maxNote {
		writeErr(w, http.StatusBadRequest, "the note is longer than 280 characters")
		return
	}
	rt := store.Rating{Episode: ep, Score: body.Score, Note: note}
	if err := s.Store.SaveRating(r.Context(), id, rt); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

func (s *Server) deleteRating(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	ep, err := strconv.ParseFloat(r.PathValue("ep"), 64)
	if !ok || err != nil {
		writeErr(w, http.StatusBadRequest, "bad anime id or episode")
		return
	}
	if err := s.Store.DeleteRating(r.Context(), id, ep); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
