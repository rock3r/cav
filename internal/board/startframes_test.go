package board

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStartFramesFollowTheBeatsAndTheCompStart(t *testing.T) {
	// 120 bpm is 0.5 s a beat: at 30 fps a beat is 15 frames.
	sb := &Board{BPM: 120, Shots: []Shot{
		{ID: "s1", Beats: [2]float64{0, 2}},
		{ID: "s2", Beats: [2]float64{2, 4}},
		{ID: "s3", Beats: [2]float64{4.2, 6}, Cut: true},
	}}
	got := sb.StartFrames(30, 100)
	want := []ShotFrame{{ID: "s2", Frame: 130}, {ID: "s3", Frame: 163, Cut: true}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// A shot that starts before time 0 starts at the comp start (as in assets/board/place.js).
	early := &Board{BPM: 120, Offset: -4, Shots: []Shot{{ID: "a", Beats: [2]float64{0, 2}}, {ID: "b", Beats: [2]float64{2, 4}}}}
	if got = early.StartFrames(30, 5); got[0].Frame != 5 {
		t.Fatalf("early shot starts at %d", got[0].Frame)
	}
}

func TestCutIsWrittenOnlyWhenSet(t *testing.T) {
	b, _ := json.Marshal(Shot{ID: "s1"})
	if strings.Contains(string(b), "cut") {
		t.Errorf("a shot without cut writes %s", b)
	}
	b, _ = json.Marshal(Shot{ID: "s1", Cut: true})
	if !strings.Contains(string(b), `"cut":true`) {
		t.Errorf("a cut shot writes %s", b)
	}
}
