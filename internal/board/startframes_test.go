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
	early := &Board{BPM: 120, Offset: -1.5, Shots: []Shot{{ID: "a", Beats: [2]float64{0, 2}}, {ID: "b", Beats: [2]float64{2, 4}}}}
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

func TestStartFramesSkipShotsThatPlaceDrops(t *testing.T) {
	// At 120 bpm and 30 fps a beat is 15 frames. s2 is a tenth of a beat (1.5 frames), and
	// its end rounds onto its own start, so place.js gives it no frame. s3 starts where s2
	// starts, so only s3 is a boundary there.
	sb := &Board{BPM: 120, Shots: []Shot{
		{ID: "s1", Beats: [2]float64{0, 2}},
		{ID: "s2", Beats: [2]float64{2, 2.02}},
		{ID: "s3", Beats: [2]float64{2.02, 4}},
	}}
	got := sb.StartFrames(30, 0)
	if len(got) != 1 || got[0].ID != "s3" || got[0].Frame != 30 {
		t.Fatalf("got %+v, want only s3 at 30", got)
	}
	// A shot that ends before time 0 is dropped too.
	early := &Board{BPM: 120, Offset: -4, Shots: []Shot{{ID: "a", Beats: [2]float64{0, 2}}, {ID: "b", Beats: [2]float64{2, 4}}, {ID: "c", Beats: [2]float64{4, 12}}}}
	if got = early.StartFrames(30, 0); len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("early: %+v", got)
	}
}
