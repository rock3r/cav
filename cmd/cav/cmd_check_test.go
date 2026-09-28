package main

import "testing"

func TestStillGaps(t *testing.T) {
	// One large layer moves 0-30; one small dot pulses 30-100; three small glyphs move 60-80.
	segs := [][3]float64{{0, 30, 0}, {30, 100, 1}, {60, 80, 1}, {60, 80, 1}, {60, 80, 1}}
	got := stillGaps(segs, 0, 100)
	want := [][2]float64{{30, 60}, {80, 100}}
	if len(got) != len(want) {
		t.Fatalf("gaps = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("gaps = %v, want %v", got, want)
		}
	}
	if g := stillGaps([][3]float64{{0, 100, 0}}, 0, 100); len(g) != 0 {
		t.Fatalf("fully moving piece has gaps %v", g)
	}
}
