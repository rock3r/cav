package board

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSpansFollowTheBeatsAndSkipWhatPlaceDrops(t *testing.T) {
	// 120 bpm is 0.5 s a beat: at 30 fps a beat is 15 frames.
	sb := &Board{BPM: 120, Shots: []Shot{
		{ID: "s1", Beats: [2]float64{0, 2}},
		{ID: "s2", Beats: [2]float64{2, 8}, Hold: true},
		{ID: "tiny", Beats: [2]float64{8, 8.01}},
		{ID: "s3", Beats: [2]float64{8.01, 10}},
	}}
	got := sb.Spans(30, 100)
	want := []ShotSpan{{ID: "s1", From: 100, To: 130}, {ID: "s2", From: 130, To: 220, Hold: true}, {ID: "s3", From: 220, To: 250}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("shot %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestHoldIsWrittenOnlyWhenSet(t *testing.T) {
	b, _ := json.Marshal(Shot{ID: "s1"})
	if strings.Contains(string(b), "hold") {
		t.Errorf("a shot without hold writes %s", b)
	}
	b, _ = json.Marshal(Shot{ID: "s1", Hold: true})
	if !strings.Contains(string(b), `"hold":true`) {
		t.Errorf("a hold shot writes %s", b)
	}
}
