package store

import (
	"context"
	"time"
)

// Rating is the viewer's own score for one episode, 1 to 10, with an
// optional note. AniList only scores whole anime, so these stay here.
type Rating struct {
	Episode   float64 `json:"episode"`
	Score     int     `json:"score"`
	Note      string  `json:"note"`
	UpdatedAt int64   `json:"updatedAt"`
}

func (s *Store) SaveRating(ctx context.Context, anilistID int, r Rating) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ratings (anilist_id, episode, score, note, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (anilist_id, episode) DO UPDATE SET
			score = excluded.score, note = excluded.note, updated_at = excluded.updated_at`,
		anilistID, r.Episode, r.Score, r.Note, time.Now().Unix())
	return err
}

func (s *Store) DeleteRating(ctx context.Context, anilistID int, episode float64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ratings WHERE anilist_id = ? AND episode = ?`, anilistID, episode)
	return err
}

// Ratings returns an anime's episode ratings in episode order.
func (s *Store) Ratings(ctx context.Context, anilistID int) ([]Rating, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT episode, score, note, updated_at FROM ratings WHERE anilist_id = ? ORDER BY episode`, anilistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rating{}
	for rows.Next() {
		var r Rating
		if err := rows.Scan(&r.Episode, &r.Score, &r.Note, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
