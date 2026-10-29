// Package aniskip fetches opening and ending timestamps from AniSkip
// (api.aniskip.com), a community database keyed by MyAnimeList ID.
package aniskip

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
)

// Interval is a part of an episode to skip, in seconds.
type Interval struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Times holds the opening and ending of one episode; either may be nil.
type Times struct {
	OP *Interval `json:"op"`
	ED *Interval `json:"ed"`
}

type result struct {
	Interval struct {
		StartTime float64 `json:"startTime"`
		EndTime   float64 `json:"endTime"`
	} `json:"interval"`
	SkipType      string  `json:"skipType"`
	EpisodeLength float64 `json:"episodeLength"`
}

type Client struct {
	HTTP *http.Client

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	rs []result
	at time.Time
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 8 * time.Second}, cache: map[string]cached{}}
}

// Get returns skip times for episode ep of MAL anime malID, fitted to a
// video of the given length (0 if unknown).
func (c *Client) Get(ctx context.Context, malID int, ep float64, length float64) (*Times, error) {
	rs, err := c.results(ctx, malID, ep)
	if err != nil {
		return nil, err
	}
	t := &Times{}
	for _, r := range rs {
		iv := fit(r, length)
		switch {
		case r.SkipType == "op" && t.OP == nil:
			t.OP = iv
		case r.SkipType == "ed" && t.ED == nil:
			t.ED = iv
		}
	}
	return t, nil
}

func (c *Client) results(ctx context.Context, malID int, ep float64) ([]result, error) {
	key := fmt.Sprintf("%d/%g", malID, ep)
	c.mu.Lock()
	hit, ok := c.cache[key]
	c.mu.Unlock()
	if ok && time.Since(hit.at) < 24*time.Hour {
		return hit.rs, nil
	}
	url := fmt.Sprintf("https://api.aniskip.com/v2/skip-times/%d/%g?types=op&types=ed&episodeLength=0", malID, ep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Found   bool     `json:"found"`
		Results []result `json:"results"`
	}
	// AniSkip answers 404 with found:false when nobody submitted times.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return nil, fmt.Errorf("aniskip: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("aniskip: %w", err)
	}
	c.mu.Lock()
	c.cache[key] = cached{rs: body.Results, at: time.Now()}
	c.mu.Unlock()
	return body.Results, nil
}

// fit moves an interval timed on a video of another length onto ours.
// Releases mostly differ by material added at the start or end (a fansub
// logo, a preview), so an interval in the first half keeps its distance from
// the start and one in the second half keeps its distance from the end.
func fit(r result, length float64) *Interval {
	iv := &Interval{Start: r.Interval.StartTime, End: r.Interval.EndTime}
	if length <= 0 || r.EpisodeLength <= 0 || math.Abs(length-r.EpisodeLength) < 1 {
		return iv
	}
	if iv.Start > r.EpisodeLength/2 {
		shift := length - r.EpisodeLength
		iv.Start += shift
		iv.End += shift
	}
	iv.Start = math.Max(0, iv.Start)
	iv.End = math.Min(length, iv.End)
	if iv.End-iv.Start < 5 {
		return nil
	}
	return iv
}
