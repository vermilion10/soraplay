package server

import (
	"context"
	"testing"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/config"
	"github.com/vermilion10/nonton/internal/store"
)

func item(id, progress int, updated time.Time, adult bool) anilist.ListItem {
	it := anilist.ListItem{ListEntry: anilist.ListEntry{Progress: progress, UpdatedAt: updated.Unix()}}
	it.Media.ID, it.Media.IsAdult = id, adult
	return it
}

func TestWanted(t *testing.T) {
	ctx := context.Background()
	s := testServer(t)
	s.Cfg = config.Config{PrefetchAhead: 2}
	now, old := time.Now(), time.Now().Add(-30*24*time.Hour)
	for id, latest := range map[int]float64{1: 12, 2: 6, 3: 12, 4: 12, 5: 12} {
		s.Store.SetAvailability(ctx, id, latest)
	}
	// Show 5 was last touched on AniList long ago but played here yesterday.
	s.Store.SavePosition(ctx, store.Position{AnilistID: 5, Episode: 3, Position: 60, Duration: 1400})

	got, err := s.wanted(ctx, []anilist.ListItem{
		item(1, 4, now, false), // next two: 5 and 6
		item(2, 5, now, false), // only 6 is on the source
		item(3, 4, old, false), // not watched lately
		item(4, 4, now, true),  // adult: never saved
		item(5, 2, old, false), // recent through the saved position
		item(6, 0, now, false), // source never checked
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[wantKey]bool{{1, 5}: true, {1, 6}: true, {2, 6}: true, {5, 3}: true, {5, 4}: true}
	if len(got) != len(want) {
		t.Fatalf("wanted = %v; want %v", got, want)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing %v in %v", k, got)
		}
	}
}

func TestMediaRejectsOtherFiles(t *testing.T) {
	s := testServer(t)
	s.Cfg = config.Config{PrefetchDir: t.TempDir()}
	h := s.Handler()
	for _, path := range []string{"/media/1/2/../../../etc/passwd", "/media/1/2/index.m3u8x", "/media/x/2/index.m3u8"} {
		// The router answers ".." with a redirect to the cleaned path, which
		// is not a media route; anything else must be a 404.
		if code := httptestRecorder(h, path); code != 404 && code != 307 {
			t.Errorf("GET %s = %d; want 404 or a redirect", path, code)
		}
	}
}
