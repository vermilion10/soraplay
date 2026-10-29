package aniskip

import "testing"

func res(start, end, length float64) result {
	var r result
	r.Interval.StartTime, r.Interval.EndTime, r.EpisodeLength = start, end, length
	return r
}

func TestFit(t *testing.T) {
	// Opening: stays anchored to the start.
	if iv := fit(res(3, 93, 1510), 1442); iv.Start != 3 || iv.End != 93 {
		t.Errorf("op = %+v", iv)
	}
	// Ending: keeps its distance from the end of a 68 s shorter video.
	if iv := fit(res(1417, 1507, 1510), 1442); iv.Start != 1349 || iv.End != 1439 {
		t.Errorf("ed = %+v", iv)
	}
	// Same length: unchanged.
	if iv := fit(res(1417, 1507, 1510), 1510); iv.Start != 1417 {
		t.Errorf("same length = %+v", iv)
	}
	// A submission running past its own end is clipped to ours, and dropped
	// when what is left is too short to be worth a button.
	if iv := fit(res(1508, 1530, 1510), 1500); iv != nil {
		t.Errorf("clipped = %+v; want nil", iv)
	}
}
