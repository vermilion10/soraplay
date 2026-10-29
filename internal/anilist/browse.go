package anilist

import (
	"context"
	"fmt"
)

// Airing is one episode in the airing schedule.
type Airing struct {
	Episode  int   `json:"episode"`
	AiringAt int64 `json:"airingAt"`
	Media    Media `json:"media"`
}

// Schedule returns the episodes airing between two Unix times, earliest
// first. A week is a few hundred entries, read 50 at a time.
func (c *Client) Schedule(ctx context.Context, from, to int64) ([]Airing, error) {
	key := fmt.Sprintf("schedule/%d/%d", from, to)
	if v, ok := c.page(key); ok {
		return v.([]Airing), nil
	}
	q := `query ($page: Int, $from: Int, $to: Int) {
		Page(page: $page, perPage: 50) {
			pageInfo { hasNextPage }
			airingSchedules(airingAt_greater: $from, airingAt_lesser: $to, sort: TIME) {
				episode airingAt media {` + mediaFields + ` }
			}
		}
	}`
	out := []Airing{}
	for page := 1; page <= 10; page++ {
		var res struct {
			Page struct {
				PageInfo        struct{ HasNextPage bool } `json:"pageInfo"`
				AiringSchedules []Airing                   `json:"airingSchedules"`
			}
		}
		if err := c.query(ctx, "", q, map[string]any{"page": page, "from": from, "to": to}, &res); err != nil {
			return nil, err
		}
		out = append(out, res.Page.AiringSchedules...)
		if !res.Page.PageInfo.HasNextPage {
			break
		}
	}
	c.putPage(key, out)
	return out, nil
}

// SeasonQuery picks a page of a season's anime.
type SeasonQuery struct {
	Season string   // WINTER, SPRING, SUMMER, FALL
	Year   int      //
	Sort   string   // a MediaSort, e.g. POPULARITY_DESC
	Format []string // MediaFormats to keep; empty keeps all
	Page   int
}

// Seasonal returns one page of a season's anime, without adult titles.
func (c *Client) Seasonal(ctx context.Context, sq SeasonQuery) ([]Media, bool, error) {
	key := fmt.Sprintf("season/%s/%d/%s/%v/%d", sq.Season, sq.Year, sq.Sort, sq.Format, sq.Page)
	type result struct {
		media   []Media
		hasNext bool
	}
	if v, ok := c.page(key); ok {
		r := v.(result)
		return r.media, r.hasNext, nil
	}
	vars := map[string]any{"season": sq.Season, "year": sq.Year, "sort": []string{sq.Sort}, "page": sq.Page}
	if len(sq.Format) > 0 {
		vars["format"] = sq.Format
	}
	q := `query ($season: MediaSeason, $year: Int, $sort: [MediaSort], $format: [MediaFormat], $page: Int) {
		Page(page: $page, perPage: 30) {
			pageInfo { hasNextPage }
			media(season: $season, seasonYear: $year, type: ANIME, sort: $sort, format_in: $format, isAdult: false) {` + mediaFields + ` }
		}
	}`
	var res struct {
		Page struct {
			PageInfo struct{ HasNextPage bool } `json:"pageInfo"`
			Media    []Media                    `json:"media"`
		}
	}
	if err := c.query(ctx, "", q, vars, &res); err != nil {
		return nil, false, err
	}
	for _, m := range res.Page.Media {
		c.put(m)
	}
	r := result{res.Page.Media, res.Page.PageInfo.HasNextPage}
	if r.media == nil {
		r.media = []Media{}
	}
	c.putPage(key, r)
	return r.media, r.hasNext, nil
}

// Extras is what the anime page shows beyond the media itself.
type Extras struct {
	Source          string     `json:"source"` // MANGA, LIGHT_NOVEL, ORIGINAL, ...
	Studios         []string   `json:"studios"`
	Relations       []Relation `json:"relations"`
	Recommendations []Media    `json:"recommendations"`
}

type Relation struct {
	Type  string `json:"type"` // SEQUEL, PREQUEL, SIDE_STORY, ...
	Media Media  `json:"media"`
}

const smallMediaFields = `id type format status episodes seasonYear isAdult
	title { romaji english native userPreferred } coverImage { large color }`

// Extras returns an anime's studios, related anime and recommendations.
// Related manga and adult recommendations are left out.
func (c *Client) Extras(ctx context.Context, id int) (*Extras, error) {
	key := fmt.Sprintf("extras/%d", id)
	if v, ok := c.page(key); ok {
		return v.(*Extras), nil
	}
	q := `query ($id: Int) { Media(id: $id, type: ANIME) {
		source
		studios(isMain: true) { nodes { name } }
		relations { edges { relationType(version: 2) node { ` + smallMediaFields + ` } } }
		recommendations(perPage: 12, sort: RATING_DESC) { nodes { mediaRecommendation { ` + smallMediaFields + ` } } }
	} }`
	type typed struct {
		Media
		Type string `json:"type"`
	}
	var res struct {
		Media struct {
			Source  string `json:"source"`
			Studios struct {
				Nodes []struct{ Name string } `json:"nodes"`
			} `json:"studios"`
			Relations struct {
				Edges []struct {
					RelationType string `json:"relationType"`
					Node         typed  `json:"node"`
				} `json:"edges"`
			} `json:"relations"`
			Recommendations struct {
				Nodes []struct {
					MediaRecommendation *typed `json:"mediaRecommendation"`
				} `json:"nodes"`
			} `json:"recommendations"`
		}
	}
	if err := c.query(ctx, "", q, map[string]any{"id": id}, &res); err != nil {
		return nil, err
	}
	ex := &Extras{Source: res.Media.Source, Studios: []string{}, Relations: []Relation{}, Recommendations: []Media{}}
	for _, s := range res.Media.Studios.Nodes {
		ex.Studios = append(ex.Studios, s.Name)
	}
	for _, e := range res.Media.Relations.Edges {
		if e.Node.Type == "ANIME" {
			ex.Relations = append(ex.Relations, Relation{Type: e.RelationType, Media: e.Node.Media})
		}
	}
	for _, n := range res.Media.Recommendations.Nodes {
		if r := n.MediaRecommendation; r != nil && r.Type == "ANIME" && !r.IsAdult {
			ex.Recommendations = append(ex.Recommendations, r.Media)
		}
	}
	c.putPage(key, ex)
	return ex, nil
}
