package store

import (
	"context"
	"path/filepath"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProgressQueueKeepsHighest(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	for _, p := range []int{3, 5, 4} {
		if err := s.QueueProgress(ctx, 1, p); err != nil {
			t.Fatal(err)
		}
	}
	q, err := s.PendingProgress(ctx)
	if err != nil || len(q) != 1 || q[0].Progress != 5 {
		t.Fatalf("pending = %+v, %v; want progress 5", q, err)
	}

	// An older send finishing must not drop a newer queued value.
	s.QueueProgress(ctx, 1, 6)
	s.DoneProgress(ctx, 1, 5)
	q, _ = s.PendingProgress(ctx)
	if len(q) != 1 || q[0].Progress != 6 {
		t.Fatalf("after stale done: %+v; want progress 6 still queued", q)
	}
	s.DoneProgress(ctx, 1, 6)
	if q, _ = s.PendingProgress(ctx); len(q) != 0 {
		t.Fatalf("queue not empty: %+v", q)
	}
}

func TestMappingRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if m, err := s.Mapping(ctx, 7, "sourcea"); err != nil || m != nil {
		t.Fatalf("empty store returned %+v, %v", m, err)
	}
	want := Mapping{AnilistID: 7, Provider: "sourcea", ProviderID: "abc", Title: "ABC", EpOffset: -12, Manual: true}
	if err := s.SaveMapping(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Mapping(ctx, 7, "sourcea")
	if err != nil || got.ProviderID != "abc" || got.EpOffset != -12 || !got.Manual {
		t.Fatalf("got %+v, %v", got, err)
	}
}
