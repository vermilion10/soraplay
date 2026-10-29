// Package anilist is a small client for the AniList GraphQL API and OAuth.
package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	apiURL       = "https://graphql.anilist.co"
	authorizeURL = "https://anilist.co/api/v2/oauth/authorize"
	tokenURL     = "https://anilist.co/api/v2/oauth/token"
)

type Title struct {
	Romaji        string `json:"romaji"`
	English       string `json:"english"`
	Native        string `json:"native"`
	UserPreferred string `json:"userPreferred"`
}

type Media struct {
	ID         int      `json:"id"`
	IDMal      int      `json:"idMal"`
	Title      Title    `json:"title"`
	Synonyms   []string `json:"synonyms"`
	Format     string   `json:"format"`
	Status     string   `json:"status"`
	Episodes   int      `json:"episodes"`
	Duration   int      `json:"duration"`
	Season     string   `json:"season"`
	SeasonYear int      `json:"seasonYear"`
	CoverImage struct {
		Large string `json:"large"`
		Color string `json:"color"`
	} `json:"coverImage"`
	BannerImage       string   `json:"bannerImage"`
	Description       string   `json:"description"`
	Genres            []string `json:"genres"`
	AverageScore      int      `json:"averageScore"`
	IsAdult           bool     `json:"isAdult"`
	NextAiringEpisode *struct {
		Episode  int   `json:"episode"`
		AiringAt int64 `json:"airingAt"`
	} `json:"nextAiringEpisode"`
	MediaListEntry *ListEntry `json:"mediaListEntry"`
}

type ListEntry struct {
	Progress  int    `json:"progress"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updatedAt"`
}

// ListItem is one entry of the viewer's list together with its media.
type ListItem struct {
	ListEntry
	Media Media `json:"media"`
}

type Viewer struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Avatar struct {
		Medium string `json:"medium"`
	} `json:"avatar"`
}

const mediaFields = `
	id idMal
	title { romaji english native userPreferred }
	synonyms format status episodes duration season seasonYear
	coverImage { large color }
	bannerImage
	description(asHtml: false)
	genres averageScore isAdult
	nextAiringEpisode { episode airingAt }`

// Client talks to AniList. Token may be empty for public queries.
//
// AniList allows about 30 requests a minute, so media is cached: for an hour
// without the viewer's list entry, and for a minute with it, so progress
// shown on anime pages is never more than a minute behind.
type Client struct {
	HTTP *http.Client

	mu        sync.Mutex
	cache     map[int]cachedMedia // without mediaListEntry
	userCache map[int]cachedMedia // with the viewer's mediaListEntry
}

type cachedMedia struct {
	m  Media
	at time.Time
}

const (
	mediaTTL = time.Hour
	userTTL  = time.Minute
)

func New() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		cache:     map[int]cachedMedia{},
		userCache: map[int]cachedMedia{},
	}
}

// Error is a GraphQL or HTTP error from AniList.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("anilist: %d %s", e.Status, e.Message) }

func (c *Client) query(ctx context.Context, token, q string, vars map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{"query": q, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
			Status  int    `json:"status"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return &Error{Status: resp.StatusCode, Message: string(raw)}
	}
	if len(env.Errors) > 0 {
		st := env.Errors[0].Status
		if st == 0 {
			st = resp.StatusCode
		}
		return &Error{Status: st, Message: env.Errors[0].Message}
	}
	if resp.StatusCode != http.StatusOK {
		return &Error{Status: resp.StatusCode, Message: resp.Status}
	}
	return json.Unmarshal(env.Data, out)
}

func (c *Client) Viewer(ctx context.Context, token string) (*Viewer, error) {
	var out struct{ Viewer Viewer }
	if err := c.query(ctx, token, `query { Viewer { id name avatar { medium } } }`, nil, &out); err != nil {
		return nil, err
	}
	return &out.Viewer, nil
}

// List returns the user's entries with the given statuses (CURRENT, PLANNING, ...).
func (c *Client) List(ctx context.Context, token string, userID int, statuses []string) ([]ListItem, error) {
	q := `query ($user: Int, $status: [MediaListStatus]) {
		MediaListCollection(userId: $user, type: ANIME, status_in: $status, sort: UPDATED_TIME_DESC) {
			lists { entries { progress status updatedAt media {` + mediaFields + ` } } }
		}
	}`
	var out struct {
		MediaListCollection struct {
			Lists []struct {
				Entries []ListItem `json:"entries"`
			} `json:"lists"`
		}
	}
	if err := c.query(ctx, token, q, map[string]any{"user": userID, "status": statuses}, &out); err != nil {
		return nil, err
	}
	var items []ListItem
	for _, l := range out.MediaListCollection.Lists {
		items = append(items, l.Entries...)
	}
	for _, it := range items {
		c.put(it.Media)
		m := it.Media
		entry := it.ListEntry
		m.MediaListEntry = &entry
		c.putUser(m)
	}
	return items, nil
}

// Media returns one anime. With a token it includes the viewer's list entry.
func (c *Client) Media(ctx context.Context, token string, id int) (*Media, error) {
	if m, ok := c.get(id, token != ""); ok {
		return &m, nil
	}
	entry := ""
	if token != "" {
		entry = "mediaListEntry { progress status updatedAt }"
	}
	q := `query ($id: Int) { Media(id: $id, type: ANIME) {` + mediaFields + ` ` + entry + ` } }`
	var out struct{ Media Media }
	if err := c.query(ctx, token, q, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	c.put(out.Media)
	if token != "" {
		c.putUser(out.Media)
	}
	return &out.Media, nil
}

// Forget drops the cached list entry for id, after it changed or before it
// must be read fresh.
func (c *Client) Forget(id int) {
	c.mu.Lock()
	delete(c.userCache, id)
	c.mu.Unlock()
}

// ForgetUser drops every cached list entry, when the viewer logs out.
func (c *Client) ForgetUser() {
	c.mu.Lock()
	c.userCache = map[int]cachedMedia{}
	c.mu.Unlock()
}

func (c *Client) Search(ctx context.Context, token, search string) ([]Media, error) {
	q := `query ($q: String) { Page(perPage: 30) { media(search: $q, type: ANIME, sort: SEARCH_MATCH) {` + mediaFields + ` } } }`
	var out struct {
		Page struct {
			Media []Media `json:"media"`
		}
	}
	if err := c.query(ctx, token, q, map[string]any{"q": search}, &out); err != nil {
		return nil, err
	}
	return out.Page.Media, nil
}

// Seasonal returns the most popular anime of a season (WINTER, SPRING, SUMMER, FALL).
func (c *Client) Seasonal(ctx context.Context, token, season string, year, perPage int) ([]Media, error) {
	q := `query ($season: MediaSeason, $year: Int, $n: Int) {
		Page(perPage: $n) { media(season: $season, seasonYear: $year, type: ANIME, sort: POPULARITY_DESC, isAdult: false) {` + mediaFields + ` } }
	}`
	var out struct {
		Page struct {
			Media []Media `json:"media"`
		}
	}
	if err := c.query(ctx, token, q, map[string]any{"season": season, "year": year, "n": perPage}, &out); err != nil {
		return nil, err
	}
	for _, m := range out.Page.Media {
		c.put(m)
	}
	return out.Page.Media, nil
}

// SaveProgress sets progress and status on the viewer's list entry.
func (c *Client) SaveProgress(ctx context.Context, token string, mediaID, progress int, status string) (*ListEntry, error) {
	q := `mutation ($id: Int, $p: Int, $s: MediaListStatus) {
		SaveMediaListEntry(mediaId: $id, progress: $p, status: $s) { progress status updatedAt }
	}`
	var out struct{ SaveMediaListEntry ListEntry }
	if err := c.query(ctx, token, q, map[string]any{"id": mediaID, "p": progress, "s": status}, &out); err != nil {
		return nil, err
	}
	return &out.SaveMediaListEntry, nil
}

func (c *Client) get(id int, withEntry bool) (Media, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cache, ttl := c.cache, mediaTTL
	if withEntry {
		cache, ttl = c.userCache, userTTL
	}
	cm, ok := cache[id]
	if !ok || time.Since(cm.at) > ttl {
		return Media{}, false
	}
	return cm.m, true
}

func (c *Client) put(m Media) {
	m.MediaListEntry = nil
	c.mu.Lock()
	c.cache[m.ID] = cachedMedia{m: m, at: time.Now()}
	c.mu.Unlock()
}

func (c *Client) putUser(m Media) {
	c.mu.Lock()
	c.userCache[m.ID] = cachedMedia{m: m, at: time.Now()}
	c.mu.Unlock()
}

// AuthorizeURL is where the browser is sent to log in.
func AuthorizeURL(clientID, redirectURI string) string {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("redirect_uri", redirectURI)
	v.Set("response_type", "code")
	return authorizeURL + "?" + v.Encode()
}

// ExchangeCode trades an authorization code for an access token.
func (c *Client) ExchangeCode(ctx context.Context, clientID, clientSecret, redirectURI, code string) (string, time.Time, error) {
	id, err := strconv.Atoi(clientID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("anilist: client id must be a number")
	}
	body, _ := json.Marshal(map[string]any{
		"grant_type":    "authorization_code",
		"client_id":     id,
		"client_secret": clientSecret,
		"redirect_uri":  redirectURI,
		"code":          code,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       string `json:"error"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", time.Time{}, fmt.Errorf("anilist: token response: %w", err)
	}
	if out.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("anilist: token exchange failed: %s %s", out.Error, out.Message)
	}
	return out.AccessToken, time.Now().Add(time.Duration(out.ExpiresIn) * time.Second), nil
}
