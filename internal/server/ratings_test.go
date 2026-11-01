package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRatingsAPI(t *testing.T) {
	h := testServer(t).Handler()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for _, body := range []string{`{"score":0}`, `{"score":11}`, `{"score":5,"note":"` + strings.Repeat("あ", 281) + `"}`} {
		if rec := do(http.MethodPut, "/api/anime/7/episodes/2/rating", body); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %.30s = %d, want 400", body, rec.Code)
		}
	}
	if rec := do(http.MethodPut, "/api/anime/7/episodes/2/rating", `{"score":9,"note":"  the pool scene  "}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body)
	}
	rec := do(http.MethodGet, "/api/anime/7/ratings", "")
	if got := rec.Body.String(); !strings.Contains(got, `"score":9`) || !strings.Contains(got, `"note":"the pool scene"`) {
		t.Errorf("GET = %s", got)
	}
	if rec := do(http.MethodDelete, "/api/anime/7/episodes/2/rating", `{}`); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE = %d", rec.Code)
	}
	if got := do(http.MethodGet, "/api/anime/7/ratings", "").Body.String(); strings.TrimSpace(got) != "[]" {
		t.Errorf("after delete = %s", got)
	}
}
