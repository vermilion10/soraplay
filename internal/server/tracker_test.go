package server

import (
	"testing"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/store"
)

func TestNeedsCheck(t *testing.T) {
	fresh := store.Availability{LatestEp: 5, CheckedAt: time.Now()}
	old := store.Availability{LatestEp: 5, CheckedAt: time.Now().Add(-2 * recheckFinished)}
	cases := []struct {
		name string
		m    anilist.Media
		a    store.Availability
		want bool
	}{
		{"airing", anilist.Media{Status: "RELEASING", Episodes: 12}, fresh, true},
		{"finished, all on source", anilist.Media{Status: "FINISHED", Episodes: 5}, old, false},
		{"finished, missing, checked today", anilist.Media{Status: "FINISHED", Episodes: 12}, fresh, false},
		{"finished, missing, checked long ago", anilist.Media{Status: "FINISHED", Episodes: 12}, old, true},
	}
	for _, c := range cases {
		if got := needsCheck(c.m, c.a); got != c.want {
			t.Errorf("%s: needsCheck = %v; want %v", c.name, got, c.want)
		}
	}
}
