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

func TestPositions(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if p, err := s.Position(ctx, 1, 2); err != nil || p != nil {
		t.Fatalf("empty = %+v, %v", p, err)
	}
	s.SavePosition(ctx, Position{AnilistID: 1, Episode: 2, Position: 100, Duration: 1400})
	s.SavePosition(ctx, Position{AnilistID: 1, Episode: 2, Position: 300, Duration: 1400})
	p, _ := s.Position(ctx, 1, 2)
	if p == nil || p.Position != 300 {
		t.Fatalf("position = %+v; want 300", p)
	}
	s.SavePosition(ctx, Position{AnilistID: 9, Episode: 1, Position: 5, Duration: 60})
	latest, err := s.LatestPositions(ctx)
	if err != nil || len(latest) != 2 || latest[1].Episode != 2 || latest[9].Position != 5 {
		t.Fatalf("latest = %+v, %v", latest, err)
	}
}

func TestAvailability(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if prev, err := s.SetAvailability(ctx, 5, 3); err != nil || prev != 0 {
		t.Fatalf("first set: prev %v, %v", prev, err)
	}
	if prev, _ := s.SetAvailability(ctx, 5, 4); prev != 3 {
		t.Fatalf("second set: prev %v; want 3", prev)
	}
	all, err := s.AllAvailability(ctx)
	if err != nil || all[5].LatestEp != 4 || all[5].CheckedAt.IsZero() {
		t.Fatalf("all = %+v, %v", all, err)
	}
}
