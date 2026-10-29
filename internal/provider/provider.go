// Package provider defines the interface every streaming source implements.
package provider

import "context"

// Result is one anime returned by a provider search.
type Result struct {
	ID       string // provider-specific ID, usually the page slug
	Title    string
	Episodes int // 0 if the provider does not say
	Poster   string
}

// Anime is a provider's detail page for one show.
type Anime struct {
	ID       string
	Title    string
	Status   string
	Poster   string
	Episodes []Episode // sorted by Number, ascending
}

// Episode is one playable episode on a provider.
type Episode struct {
	ID     string // provider-specific ID, usually the episode page URL
	Number float64
	Title  string
}

// Stream is a resolved, playable source for one episode.
type Stream struct {
	URL     string            // HLS master playlist or direct video file
	Type    string            // "hls" or "mp4"
	Headers map[string]string // headers the CDN expects (Referer, Origin, ...)
	SubLang string            // e.g. "id-hardsub"
	Server  string            // human-readable server name
}

// Provider is a site that can be searched and streamed from.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string) ([]Result, error)
	Anime(ctx context.Context, id string) (*Anime, error)
	Streams(ctx context.Context, ep Episode) ([]Stream, error)
}
