package match

import (
	"testing"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/provider"
)

func media(romaji, english string, eps int, format string, syn ...string) anilist.Media {
	m := anilist.Media{Episodes: eps, Format: format, Synonyms: syn}
	m.Title.Romaji, m.Title.English = romaji, english
	return m
}

func TestParse(t *testing.T) {
	cases := []struct {
		in           string
		season, part int
		tokens       string
	}{
		{"Sousou No Frieren 2Nd Season", 2, 1, "soso no frieren"},
		{"Frieren: Beyond Journey’s End Season 2", 2, 1, "frieren beyond jorneys end"},
		{"Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", 2, 2, "mushoku tensei isekai ittara honki dasu"},
		{"Mushoku Tensei S2 Part 2 Sub Indo", 2, 2, "mushoku tensei"},
		{"Overlord IV", 4, 1, "overlord"},
	}
	for _, c := range cases {
		p := parse(c.in)
		got := ""
		for i, tk := range p.tokens {
			if i > 0 {
				got += " "
			}
			got += tk
		}
		if p.season != c.season || p.part != c.part || got != c.tokens {
			t.Errorf("parse(%q) = season %d part %d %q; want %d %d %q", c.in, p.season, p.part, got, c.season, c.part, c.tokens)
		}
	}
}

func TestScorePicksRightSeason(t *testing.T) {
	s1 := media("Sousou no Frieren", "Frieren: Beyond Journey’s End", 28, "TV")
	s2 := media("Sousou no Frieren 2nd Season", "Frieren: Beyond Journey’s End Season 2", 10, "TV")
	r1 := provider.Result{ID: "a", Title: "Sousou No Frieren", Episodes: 28}
	r2 := provider.Result{ID: "b", Title: "Sousou No Frieren 2Nd Season", Episodes: 10}

	if a, b := Score(s1, r1), Score(s1, r2); a < Threshold || b >= a {
		t.Errorf("season 1: right %.2f wrong %.2f", a, b)
	}
	if a, b := Score(s2, r2), Score(s2, r1); a < Threshold || b >= a {
		t.Errorf("season 2: right %.2f wrong %.2f", a, b)
	}
}

func TestScoreMovieAndSynonym(t *testing.T) {
	m := media("Kimi no Na wa.", "Your Name.", 1, "MOVIE")
	if s := Score(m, provider.Result{Title: "Kimi No Na Wa Movie"}); s < Threshold {
		t.Errorf("movie score %.2f", s)
	}
	syn := media("Ore dake Level Up na Ken", "Solo Leveling", 12, "TV")
	if s := Score(syn, provider.Result{Title: "Solo Leveling"}); s < Threshold {
		t.Errorf("english title score %.2f", s)
	}
}

func TestTitlesSkipsNonLatin(t *testing.T) {
	m := media("Sousou no Frieren", "", 28, "TV", "葬送のフリーレン", "Frieren", "Фрирен")
	got := Titles(m)
	if len(got) != 2 || got[0] != "Sousou no Frieren" || got[1] != "Frieren" {
		t.Errorf("Titles = %q", got)
	}
}

func TestQueries(t *testing.T) {
	m := media("Sousou no Frieren 2nd Season", "Frieren: Beyond Journey’s End Season 2", 10, "TV")
	q := Queries(m)
	if len(q) == 0 || q[0] != "sousou no frieren" {
		t.Errorf("Queries = %q", q)
	}
}

func TestScoreShortenedTitle(t *testing.T) {
	m := media("Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2", "Mushoku Tensei: Jobless Reincarnation Season 2 Part 2", 12, "TV")
	right := Score(m, provider.Result{Title: "Mushoku Tensei S2 Part 2"})
	wrongPart := Score(m, provider.Result{Title: "Mushoku Tensei S2"})
	if right < Threshold || wrongPart >= right {
		t.Errorf("right %.2f, wrong part %.2f", right, wrongPart)
	}
}

func TestScoreRejectsSiblingEntries(t *testing.T) {
	m := media("Seishun Buta Yarou wa Dear Friend no Yume wo Minai", "", 13, "TV")
	if s := Score(m, provider.Result{Title: "Seishun Buta Yarou Wa Odekake Sister No Yume Wo Minai", Episodes: 1}); s >= Threshold {
		t.Errorf("sibling movie scored %.2f", s)
	}
	gits := media("Koukaku Kidoutai: THE GHOST IN THE SHELL", "GHOST IN THE SHELL", 0, "TV")
	if s := Score(gits, provider.Result{Title: "Ghost In The Shell Stand Alone Complex 2Nd Gig", Episodes: 26}); s >= Threshold {
		t.Errorf("longer provider title scored %.2f", s)
	}
}

func TestTrailingNumberIsSeason(t *testing.T) {
	m := media("Otomege Sekai wa Mob ni Kibishii Sekai desu 2", "", 12, "TV", "Otome Game Sekai wa Mob ni Kibishii Sekai desu 2")
	right := Score(m, provider.Result{Title: "Otome Game Sekai Wa Mob Ni Kibishii Sekai Desu 2", Episodes: 12})
	s1 := Score(m, provider.Result{Title: "Otome Game Sekai Wa Mob Ni Kibishii Sekai Desu", Episodes: 12})
	if right < Threshold || s1 >= right {
		t.Errorf("season 2 %.2f, season 1 %.2f", right, s1)
	}
}

func TestApostropheJoinsWord(t *testing.T) {
	m := media("Vivy: Fluorite Eye’s Song", "Vivy -Fluorite Eye's Song-", 13, "TV")
	if s := Score(m, provider.Result{Title: "Vivy Fluorite Eyes Song", Episodes: 13}); s < Threshold {
		t.Errorf("score %.2f", s)
	}
	if q := Queries(m); q[0] != "vivy fluorite eyes song" {
		t.Errorf("Queries = %q", q)
	}
}

func TestOVADoesNotMatchParentSeries(t *testing.T) {
	ova := media("Inari, Konkon, Koi Iroha.: Inari, Konkon, Semishigure", "", 1, "OVA")
	if s := Score(ova, provider.Result{Title: "Inari Konkon Koi Iroha", Episodes: 10}); s >= Threshold {
		t.Errorf("OVA matched the 10-episode series with %.2f", s)
	}
	// A site counting one bonus episode must still match.
	tv := media("Durarara!!", "Durarara!!", 24, "TV")
	if s := Score(tv, provider.Result{Title: "Durarara!!", Episodes: 26}); s < Threshold {
		t.Errorf("Durarara score %.2f", s)
	}
}
