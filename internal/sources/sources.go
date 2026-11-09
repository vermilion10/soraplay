// Package sources wires the streaming sites this build plays from.
//
// This build ships without any. A source is a type that implements
// provider.Provider: search a site by title, list a show's episodes, and resolve
// an episode to a playable HLS or MP4 stream. Add your own providers under
// internal/provider and return them from Load; the rest of the app (AniList
// matching, progress sync, the player, prefetching) works with whatever is here.
package sources

import "github.com/vermilion10/nonton/internal/provider"

// Set is the sources the server uses.
type Set struct {
	Main      provider.Provider   // tried first; nil when the build has no sources
	Fallbacks []provider.Provider // tried in order when Main lacks a show or episode
	Adult     provider.Provider   // serves AniList entries marked adult; nil sends them to Main
}

// Load builds the sources. This build has none, so the app can browse AniList
// and track progress but cannot play episodes.
func Load() Set { return Set{} }
