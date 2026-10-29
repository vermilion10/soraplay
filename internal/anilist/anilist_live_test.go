//go:build live

package anilist

import (
	"context"
	"testing"
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
