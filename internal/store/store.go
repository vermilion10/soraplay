// Package store keeps the app's state in SQLite: the AniList session,
// AniList-to-provider mappings and the queue of progress updates.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS kv (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS mappings (
	anilist_id  INTEGER NOT NULL,
	provider    TEXT    NOT NULL,
	provider_id TEXT    NOT NULL,
	title       TEXT    NOT NULL DEFAULT '',
	ep_offset   REAL    NOT NULL DEFAULT 0,
	manual      INTEGER NOT NULL DEFAULT 0,
	updated_at  INTEGER NOT NULL,
	PRIMARY KEY (anilist_id, provider)
);
CREATE TABLE IF NOT EXISTS positions (
	anilist_id INTEGER NOT NULL,
	episode    REAL    NOT NULL,
	position   REAL    NOT NULL,
	duration   REAL    NOT NULL,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (anilist_id, episode)
);
CREATE TABLE IF NOT EXISTS progress_queue (
	anilist_id INTEGER PRIMARY KEY,
	progress   INTEGER NOT NULL,
	attempts   INTEGER NOT NULL DEFAULT 0,
	last_error TEXT    NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL
);
`

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Get returns the value for key, or "" if it is not set.
func (s *Store) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM kv WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) Set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) Delete(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM kv WHERE key = ?`, k); err != nil {
			return err
		}
	}
	return nil
}

// Mapping links an AniList entry to a show on a provider. EpOffset is added
// to the AniList episode number to get the provider's episode number.
type Mapping struct {
	AnilistID  int
	Provider   string
	ProviderID string
	Title      string
	EpOffset   float64
	Manual     bool
	UpdatedAt  time.Time
}

// Mapping returns the mapping, or nil if there is none.
func (s *Store) Mapping(ctx context.Context, anilistID int, provider string) (*Mapping, error) {
	m := Mapping{AnilistID: anilistID, Provider: provider}
	var updated int64
	err := s.db.QueryRowContext(ctx,
		`SELECT provider_id, title, ep_offset, manual, updated_at FROM mappings WHERE anilist_id = ? AND provider = ?`,
		anilistID, provider).Scan(&m.ProviderID, &m.Title, &m.EpOffset, &m.Manual, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.UpdatedAt = time.Unix(updated, 0)
	return &m, nil
}

func (s *Store) SaveMapping(ctx context.Context, m Mapping) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO mappings (anilist_id, provider, provider_id, title, ep_offset, manual, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (anilist_id, provider) DO UPDATE SET
			provider_id = excluded.provider_id, title = excluded.title, ep_offset = excluded.ep_offset,
			manual = excluded.manual, updated_at = excluded.updated_at`,
		m.AnilistID, m.Provider, m.ProviderID, m.Title, m.EpOffset, m.Manual, time.Now().Unix())
	return err
}

func (s *Store) DeleteMapping(ctx context.Context, anilistID int, provider string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM mappings WHERE anilist_id = ? AND provider = ?`, anilistID, provider)
	return err
}

// QueuedProgress is a progress update waiting to be sent to AniList.
type QueuedProgress struct {
	AnilistID int
	Progress  int
	Attempts  int
	LastError string
}

// QueueProgress records that episode progress was watched. A later call with
// a lower number never lowers a queued value.
func (s *Store) QueueProgress(ctx context.Context, anilistID, progress int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO progress_queue (anilist_id, progress, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (anilist_id) DO UPDATE SET
			progress = max(progress, excluded.progress), attempts = 0, last_error = '', updated_at = excluded.updated_at`,
		anilistID, progress, time.Now().Unix())
	return err
}

func (s *Store) PendingProgress(ctx context.Context) ([]QueuedProgress, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT anilist_id, progress, attempts, last_error FROM progress_queue ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueuedProgress
	for rows.Next() {
		var q QueuedProgress
		if err := rows.Scan(&q.AnilistID, &q.Progress, &q.Attempts, &q.LastError); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// DoneProgress removes a queued update, unless a newer value was queued
// while it was being sent.
func (s *Store) DoneProgress(ctx context.Context, anilistID, progress int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM progress_queue WHERE anilist_id = ? AND progress <= ?`, anilistID, progress)
	return err
}

func (s *Store) FailProgress(ctx context.Context, anilistID int, msg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE progress_queue SET attempts = attempts + 1, last_error = ? WHERE anilist_id = ?`, msg, anilistID)
	return err
}

// Position is where playback of one episode stopped, in seconds.
type Position struct {
	AnilistID int     `json:"-"`
	Episode   float64 `json:"episode"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
	UpdatedAt int64   `json:"updatedAt"`
}

func (s *Store) SavePosition(ctx context.Context, p Position) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO positions (anilist_id, episode, position, duration, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (anilist_id, episode) DO UPDATE SET
			position = excluded.position, duration = excluded.duration, updated_at = excluded.updated_at`,
		p.AnilistID, p.Episode, p.Position, p.Duration, time.Now().Unix())
	return err
}

// Position returns the saved position of one episode, or nil.
func (s *Store) Position(ctx context.Context, anilistID int, episode float64) (*Position, error) {
	p := Position{AnilistID: anilistID, Episode: episode}
	err := s.db.QueryRowContext(ctx,
		`SELECT position, duration, updated_at FROM positions WHERE anilist_id = ? AND episode = ?`,
		anilistID, episode).Scan(&p.Position, &p.Duration, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// LatestPositions returns, for each anime, the episode watched most recently.
func (s *Store) LatestPositions(ctx context.Context) (map[int]Position, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT anilist_id, episode, position, duration, updated_at FROM positions p
		WHERE updated_at = (SELECT max(updated_at) FROM positions q WHERE q.anilist_id = p.anilist_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]Position{}
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.AnilistID, &p.Episode, &p.Position, &p.Duration, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out[p.AnilistID] = p
	}
	return out, rows.Err()
}
