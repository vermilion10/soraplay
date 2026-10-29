//go:build live

package anilist

import (
	"context"
	"testing"
	"time"
)

func TestLiveSearchAndMedia(t *testing.T) {
	c := New()
	ms, err := c.Search(context.Background(), "", "frieren")
	if err != nil || len(ms) == 0 {
		t.Fatalf("search: %v (%d)", err, len(ms))
	}
	for _, m := range ms {
		t.Logf("%d %s / %s %d eps %s %d syn=%v", m.ID, m.Title.Romaji, m.Title.English, m.Episodes, m.Format, m.SeasonYear, m.Synonyms)
	}
	m, err := c.Media(context.Background(), "", ms[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("media: %s", m.Title.Romaji)
}

func TestLiveBrowse(t *testing.T) {
	c := New()
	ctx := context.Background()
	now := time.Now()
	week, err := c.Schedule(ctx, now.Unix(), now.Add(7*24*time.Hour).Unix())
	if err != nil || len(week) == 0 {
		t.Fatalf("schedule: %v (%d)", err, len(week))
	}
	t.Logf("schedule: %d episodes, first %s EP %d", len(week), week[0].Media.Title.Romaji, week[0].Episode)

	ms, more, err := c.Seasonal(ctx, SeasonQuery{Season: "FALL", Year: 2026, Sort: "SCORE_DESC", Format: []string{"TV"}, Page: 1})
	if err != nil || len(ms) == 0 {
		t.Fatalf("seasonal: %v (%d)", err, len(ms))
	}
	t.Logf("seasonal: %d, more=%v, top %s (%s)", len(ms), more, ms[0].Title.Romaji, ms[0].Format)

	ex, err := c.Extras(ctx, 154587) // Frieren
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("extras: source=%s studios=%v relations=%d recs=%d", ex.Source, ex.Studios, len(ex.Relations), len(ex.Recommendations))
	for _, r := range ex.Relations {
		t.Logf("  %s: %s", r.Type, r.Media.Title.Romaji)
	}
}
