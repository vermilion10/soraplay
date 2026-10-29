// Package progress sends queued watch progress to AniList in the background,
// so a flaky connection or AniList rate limit never loses an update.
package progress

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/store"
)

type Worker struct {
	Store   *store.Store
	AniList *anilist.Client
	wake    chan struct{}
}

func NewWorker(s *store.Store, al *anilist.Client) *Worker {
	return &Worker{Store: s, AniList: al, wake: make(chan struct{}, 1)}
}

// Wake asks the worker to flush the queue now.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run flushes the queue on Wake and once a minute until ctx ends.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		w.flush(ctx)
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-t.C:
		}
	}
}

func (w *Worker) flush(ctx context.Context) {
	token, err := w.Store.Get(ctx, "anilist_token")
	if err != nil || token == "" {
		return
	}
	queue, err := w.Store.PendingProgress(ctx)
	if err != nil {
		log.Printf("progress: read queue: %v", err)
		return
	}
	for _, q := range queue {
		if err := w.send(ctx, token, q); err != nil {
			log.Printf("progress: anime %d ep %d: %v", q.AnilistID, q.Progress, err)
			w.Store.FailProgress(ctx, q.AnilistID, err.Error())
			var ae *anilist.Error
			if errors.As(err, &ae) && ae.Status == 429 {
				return // rate limited; the next tick retries
			}
		}
	}
}

func (w *Worker) send(ctx context.Context, token string, q store.QueuedProgress) error {
	m, err := w.AniList.Media(ctx, token, q.AnilistID)
	if err != nil {
		return err
	}
	if m.MediaListEntry != nil && m.MediaListEntry.Progress >= q.Progress &&
		m.MediaListEntry.Status != "PLANNING" {
		return w.Store.DoneProgress(ctx, q.AnilistID, q.Progress)
	}
	status := NextStatus(m.MediaListEntry, q.Progress, m.Episodes)
	if _, err := w.AniList.SaveProgress(ctx, token, q.AnilistID, q.Progress, status); err != nil {
		return err
	}
	log.Printf("progress: anime %d set to ep %d (%s)", q.AnilistID, q.Progress, status)
	return w.Store.DoneProgress(ctx, q.AnilistID, q.Progress)
}

// NextStatus is the list status after watching episode progress of a show
// with total episodes (0 if unknown).
func NextStatus(cur *anilist.ListEntry, progress, total int) string {
	switch {
	case total > 0 && progress >= total:
		return "COMPLETED"
	case cur != nil && cur.Status == "REPEATING":
		return "REPEATING"
	default:
		return "CURRENT"
	}
}
