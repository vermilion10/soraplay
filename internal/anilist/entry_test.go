package anilist

import (
	"encoding/json"
	"testing"
)

func TestEntryVarsLeaveOutUnsetFields(t *testing.T) {
	var u EntryUpdate
	if err := json.Unmarshal([]byte(`{"status":"PAUSED","score":7.5,"startedAt":{"year":2026,"month":10}}`), &u); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(entryVars(42, u))
	want := `{"id":42,"score":7.5,"startedAt":{"day":null,"month":10,"year":2026},"status":"PAUSED"}`
	if string(raw) != want {
		t.Errorf("vars = %s\nwant %s", raw, want)
	}

	// A zero date clears it rather than being left out.
	u = EntryUpdate{CompletedAt: &FuzzyDate{}}
	raw, _ = json.Marshal(entryVars(1, u))
	if want := `{"completedAt":{"day":null,"month":null,"year":null},"id":1}`; string(raw) != want {
		t.Errorf("vars = %s\nwant %s", raw, want)
	}
}
