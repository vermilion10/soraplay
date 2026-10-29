// Package match finds the provider show that corresponds to an AniList entry.
//
// Titles are compared as token sets after pulling out season and part
// numbers, which must agree: "Sousou no Frieren 2nd Season" should match
// "Sousou No Frieren 2Nd Season" and never "Sousou No Frieren".
package match

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/provider"
)

// Threshold is the minimum score for an automatic match.
const Threshold = 0.6

type Candidate struct {
	provider.Result
	Score float64
}

var (
	ordinalSeasonRe = regexp.MustCompile(`\b(\d+)(?:st|nd|rd|th)\s+season\b`)
	seasonNumRe     = regexp.MustCompile(`\bseason\s+(\d+)\b`)
	shortSeasonRe   = regexp.MustCompile(`\bs(\d+)\b`)
	partRe          = regexp.MustCompile(`\b(?:part|cour)\s+(\d+)\b`)
	romanTailRe     = regexp.MustCompile(`\s(ii|iii|iv|v|vi)$`)
	tailNumRe       = regexp.MustCompile(`\s([2-9])$`)
	romanValue      = map[string]int{"ii": 2, "iii": 3, "iv": 4, "v": 5, "vi": 6}
	// Words that carry no identity for matching.
	stopTokens = map[string]bool{"sub": true, "indo": true, "subtitle": true, "indonesia": true, "the": true, "tv": true}
)

// parsed is a normalized title.
type parsed struct {
	tokens []string // compared tokens, spelling-folded
	raw    []string // the same words as written, for search queries
	season int
	part   int
	movie  bool
}

func parse(title string) parsed {
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	s = " " + strings.Join(strings.Fields(s), " ") + " "

	p := parsed{season: 1, part: 1}
	take := func(re *regexp.Regexp, dst *int) {
		if m := re.FindStringSubmatch(s); m != nil {
			*dst, _ = strconv.Atoi(m[1])
			s = re.ReplaceAllString(s, " ")
		}
	}
	take(ordinalSeasonRe, &p.season)
	take(seasonNumRe, &p.season)
	take(shortSeasonRe, &p.season)
	take(partRe, &p.part)
	s = strings.TrimSpace(s)
	seasonSet := p.season != 1
	if m := romanTailRe.FindStringSubmatch(s); m != nil {
		p.season, seasonSet = romanValue[m[1]], true
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
	}
	// "Otome Game Sekai ... Desu 2": a small trailing number is a season.
	if m := tailNumRe.FindStringSubmatch(s); m != nil && !seasonSet && strings.Count(s, " ") >= 1 {
		p.season, _ = strconv.Atoi(m[1])
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
	}
	for _, t := range strings.Fields(s) {
		switch {
		case t == "movie":
			p.movie = true
		case t == "ii" || t == "iii" || t == "iv":
			p.season = romanValue[t]
		case stopTokens[t]:
		default:
			p.tokens = append(p.tokens, romanize(t))
			p.raw = append(p.raw, t)
		}
	}
	return p
}

// romanize folds common romaji spelling variants together
// (kyou/kyo, kuuki/kuki, wo/o).
func romanize(t string) string {
	if t == "wo" {
		return "o"
	}
	t = strings.ReplaceAll(t, "ou", "o")
	return strings.ReplaceAll(t, "uu", "u")
}

// similarity compares AniList title tokens al with provider title tokens pv.
// It is the Sørensen–Dice coefficient, adjusted two ways:
//   - raised when the provider title is the AniList title with words dropped,
//     since sites shorten long romaji ("Mushoku Tensei II: Isekai..." → "Mushoku Tensei S2");
//   - lowered when each side has words the other lacks, which usually means a
//     different entry of the same franchise ("... Dear Friend no Yume wo Minai"
//     vs "... Odekake Sister no Yume wo Minai").
func similarity(al, pv []string) float64 {
	if len(al) == 0 || len(pv) == 0 {
		return 0
	}
	alSet, pvSet := map[string]bool{}, map[string]bool{}
	for _, t := range al {
		alSet[t] = true
	}
	for _, t := range pv {
		pvSet[t] = true
	}
	inter := 0
	for t := range pvSet {
		if alSet[t] {
			inter++
		}
	}
	alOnly, pvOnly := len(alSet)-inter, len(pvSet)-inter
	d := 2 * float64(inter) / float64(len(alSet)+len(pvSet))
	switch {
	case pvOnly == 0 && len(pvSet) >= 2:
		d = max(d, 0.85)
	case alOnly > 0 && pvOnly > 0:
		d *= max(0, 1-0.15*float64(min(alOnly, pvOnly)))
	}
	return d
}

// Titles returns the Latin-script titles of m worth comparing against.
func Titles(m anilist.Media) []string {
	var out []string
	add := func(t string) {
		if t == "" || !isLatin(t) {
			return
		}
		for _, o := range out {
			if strings.EqualFold(o, t) {
				return
			}
		}
		out = append(out, t)
	}
	add(m.Title.Romaji)
	add(m.Title.English)
	for _, s := range m.Synonyms {
		add(s)
	}
	return out
}

func isLatin(s string) bool {
	letters, latin := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if r < 0x250 {
				latin++
			}
		}
	}
	return letters > 0 && latin*10 >= letters*9
}

// Score rates how well a provider result matches m, from 0 to 1.
func Score(m anilist.Media, r provider.Result) float64 {
	c := parse(r.Title)
	best := 0.0
	for _, t := range Titles(m) {
		a := parse(t)
		s := similarity(a.tokens, c.tokens)
		if a.season != c.season {
			s *= 0.3
		}
		if a.part != c.part {
			s *= 0.5
		}
		if s > best {
			best = s
		}
	}
	isMovie := m.Format == "MOVIE"
	if isMovie != c.movie {
		best *= 0.8
	}
	if m.Episodes > 0 && r.Episodes > 0 && r.Episodes > m.Episodes {
		best *= 0.8 // provider has more episodes than the show can have
	}
	return best
}

// Queries returns the search strings to try for m, broadest first. The site's
// search matches every word, so titles are cut down to their core tokens.
func Queries(m anilist.Media) []string {
	var out []string
	seen := map[string]bool{}
	add := func(q string) {
		q = strings.TrimSpace(q)
		if len(q) >= 3 && !seen[strings.ToLower(q)] {
			seen[strings.ToLower(q)] = true
			out = append(out, q)
		}
	}
	head := func(t string) {
		toks := parse(t).raw
		add(strings.Join(toks[:min(len(toks), 4)], " "))
	}
	head(m.Title.Romaji)
	add(m.Title.Romaji)
	if m.Title.English != "" && isLatin(m.Title.English) {
		head(m.Title.English)
	}
	// The last words of a long title are usually its most distinctive, and
	// they survive a different spelling of the first word ("Otomege" vs "Otome Game").
	if toks := parse(m.Title.Romaji).raw; len(toks) >= 5 {
		add(strings.Join(toks[len(toks)-3:], " "))
	}
	syn := 0
	for _, s := range m.Synonyms {
		if syn < 2 && isLatin(s) {
			head(s)
			syn++
		}
	}
	return out
}

// Find searches p for m and returns the candidates by descending score.
// The first candidate is a confident match only if its Score >= Threshold.
func Find(ctx context.Context, p provider.Provider, m anilist.Media) ([]Candidate, error) {
	byID := map[string]Candidate{}
	var lastErr error
	for _, q := range Queries(m) {
		rs, err := p.Search(ctx, q)
		if err != nil {
			lastErr = err
			continue
		}
		for _, r := range rs {
			if _, ok := byID[r.ID]; !ok {
				byID[r.ID] = Candidate{Result: r, Score: Score(m, r)}
			}
		}
		if best := top(byID); best >= 0.95 {
			break
		}
	}
	if len(byID) == 0 && lastErr != nil {
		return nil, lastErr
	}
	out := make([]Candidate, 0, len(byID))
	for _, c := range byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

func top(cs map[string]Candidate) float64 {
	best := 0.0
	for _, c := range cs {
		best = max(best, c.Score)
	}
	return best
}
