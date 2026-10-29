package progress

import (
	"testing"

	"github.com/vermilion10/nonton/internal/anilist"
)

func TestNextStatus(t *testing.T) {
	cases := []struct {
		cur             *anilist.ListEntry
		progress, total int
		want            string
	}{
		{nil, 1, 12, "CURRENT"},
		{&anilist.ListEntry{Status: "PLANNING"}, 1, 12, "CURRENT"},
		{&anilist.ListEntry{Status: "CURRENT", Progress: 11}, 12, 12, "COMPLETED"},
		{&anilist.ListEntry{Status: "REPEATING", Progress: 2}, 3, 12, "REPEATING"},
		{&anilist.ListEntry{Status: "CURRENT"}, 30, 0, "CURRENT"}, // ongoing, total unknown
	}
	for _, c := range cases {
		if got := NextStatus(c.cur, c.progress, c.total); got != c.want {
			t.Errorf("NextStatus(%+v, %d, %d) = %s; want %s", c.cur, c.progress, c.total, got, c.want)
		}
	}
}
