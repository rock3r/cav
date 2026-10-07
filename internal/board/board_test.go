package board

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestTimeFromBPMAndFromGrid(t *testing.T) {
	sb := &Board{BPM: 120, Offset: 0.25, Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 4}}}}
	if got := sb.Time(4); got != 2.25 {
		t.Fatalf("bpm time %g", got)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "grid.json"), []byte(`{"beats":[0.1,0.6,1.0,1.6]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "sb.json"), []byte(`{"grid":"grid.json","shots":[{"id":"s1","beats":[0,2]},{"id":"s2","beats":[2,5]}]}`), 0o644)
	sb, err := Load(filepath.Join(dir, "sb.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sb.Time(1.5); math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("between grid beats: %g", got)
	}
	// Past the last detected beat, the last spacing continues.
	if got := sb.Time(5); math.Abs(got-2.8) > 1e-9 {
		t.Fatalf("after the grid: %g", got)
	}
}

func TestValidate(t *testing.T) {
	bad := []*Board{
		{BPM: 120},
		{BPM: 120, Shots: []Shot{{ID: "s1", Beats: [2]float64{4, 4}}}},
		{BPM: 120, Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 8}}, {ID: "s2", Beats: [2]float64{6, 10}}}},
		{BPM: 120, Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 4}}, {ID: "s1", Beats: [2]float64{4, 8}}}},
		{Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 4}}}},
	}
	for i, b := range bad {
		if b.Validate() == nil {
			t.Errorf("board %d should not validate", i)
		}
	}
}
