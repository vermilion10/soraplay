package anilist

import (
	"testing"
	"time"
)

func TestUserCacheKeepsEntryAndExpires(t *testing.T) {
	c := New()
	m := Media{ID: 1, MediaListEntry: &ListEntry{Progress: 3}}
	c.put(m)
	c.putUser(m)

	if got, ok := c.get(1, false); !ok || got.MediaListEntry != nil {
		t.Fatalf("public cache = %+v, %v; want media without entry", got, ok)
	}
	if got, ok := c.get(1, true); !ok || got.MediaListEntry == nil || got.MediaListEntry.Progress != 3 {
		t.Fatalf("user cache = %+v, %v; want entry with progress 3", got, ok)
	}

	c.Forget(1)
	if _, ok := c.get(1, true); ok {
		t.Fatal("Forget left the entry cached")
	}

	c.putUser(m)
	c.mu.Lock()
	c.userCache[1] = cachedMedia{m: m, at: time.Now().Add(-2 * userTTL)}
	c.mu.Unlock()
	if _, ok := c.get(1, true); ok {
		t.Fatal("expired user entry was returned")
	}
	if _, ok := c.get(1, false); !ok {
		t.Fatal("public entry should outlive the user entry")
	}
}
