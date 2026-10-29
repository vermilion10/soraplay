package anilist

import "context"

// EntryUpdate is a change to a list entry. Nil fields are left as they are.
type EntryUpdate struct {
	Status      *string    `json:"status"`
	Progress    *int       `json:"progress"`
	Score       *float64   `json:"score"` // in the viewer's score format
	Repeat      *int       `json:"repeat"`
	StartedAt   *FuzzyDate `json:"startedAt"` // a zero date clears it
	CompletedAt *FuzzyDate `json:"completedAt"`
}

// Statuses are the list statuses AniList accepts.
var Statuses = map[string]bool{
	"CURRENT": true, "PLANNING": true, "COMPLETED": true,
	"DROPPED": true, "PAUSED": true, "REPEATING": true,
}

// SaveEntry creates or changes the viewer's list entry for an anime.
func (c *Client) SaveEntry(ctx context.Context, token string, mediaID int, u EntryUpdate) (*ListEntry, error) {
	q := `mutation ($id: Int, $status: MediaListStatus, $progress: Int, $score: Float, $repeat: Int,
		$startedAt: FuzzyDateInput, $completedAt: FuzzyDateInput) {
		SaveMediaListEntry(mediaId: $id, status: $status, progress: $progress, score: $score,
			repeat: $repeat, startedAt: $startedAt, completedAt: $completedAt) { ` + entryFields + ` }
	}`
	var out struct{ SaveMediaListEntry ListEntry }
	if err := c.query(ctx, token, q, entryVars(mediaID, u), &out); err != nil {
		return nil, err
	}
	c.Forget(mediaID)
	return &out.SaveMediaListEntry, nil
}

// entryVars builds the mutation's variables. Variables left out of the map
// leave that field unchanged; a null would clear it.
func entryVars(mediaID int, u EntryUpdate) map[string]any {
	vars := map[string]any{"id": mediaID}
	if u.Status != nil {
		vars["status"] = *u.Status
	}
	if u.Progress != nil {
		vars["progress"] = *u.Progress
	}
	if u.Score != nil {
		vars["score"] = *u.Score
	}
	if u.Repeat != nil {
		vars["repeat"] = *u.Repeat
	}
	if u.StartedAt != nil {
		vars["startedAt"] = u.StartedAt.input()
	}
	if u.CompletedAt != nil {
		vars["completedAt"] = u.CompletedAt.input()
	}
	return vars
}

// DeleteEntry removes a list entry by its entry ID (not the media ID).
func (c *Client) DeleteEntry(ctx context.Context, token string, entryID int) error {
	q := `mutation ($id: Int) { DeleteMediaListEntry(id: $id) { deleted } }`
	var out struct{ DeleteMediaListEntry struct{ Deleted bool } }
	return c.query(ctx, token, q, map[string]any{"id": entryID}, &out)
}

// input turns unknown (zero) parts into nulls, as FuzzyDateInput expects.
func (d FuzzyDate) input() map[string]any {
	part := func(n int) any {
		if n == 0 {
			return nil
		}
		return n
	}
	return map[string]any{"year": part(d.Year), "month": part(d.Month), "day": part(d.Day)}
}
