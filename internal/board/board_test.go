package board

import (
	"context"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
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

func TestShotFramesHandOffWithoutOverlap(t *testing.T) {
	// 120 bpm: a beat is 0.5 s, so 30 frames at 60 fps.
	sb := &Board{BPM: 120, Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 4}}, {ID: "s2", Beats: [2]float64{4, 8}}, {ID: "s3", Beats: [2]float64{10, 12}}}}
	want := [][2]int{{0, 119}, {120, 239}, {300, 359}}
	for i, s := range sb.Shots {
		in, out := sb.ShotFrames(s, 60)
		if in != want[i][0] || out != want[i][1] {
			t.Errorf("%s: frames %d-%d, want %d-%d", s.ID, in, out, want[i][0], want[i][1])
		}
	}
	// An offset moves every shot; odd frame rates round to the nearest frame.
	sb.Offset = 0.25
	if in, out := sb.ShotFrames(sb.Shots[1], 25); in != 56 || out != 105 {
		t.Errorf("offset at 25 fps: %d-%d, want 56-105", in, out)
	}
	// A shot shorter than a frame still shows for one frame.
	short := Shot{ID: "x", Beats: [2]float64{0, 0.01}}
	if in, out := sb.ShotFrames(short, 24); out != in {
		t.Errorf("tiny shot: %d-%d", in, out)
	}
}

func TestFitScale(t *testing.T) {
	cases := []struct {
		iw, ih, cw, ch int
		want           float64
	}{
		{1280, 720, 1920, 1080, 1.5},
		{1920, 1080, 1080, 1920, 0.5625}, // landscape frame in a portrait comp: fit the width
		{1024, 1024, 1920, 1080, 1080.0 / 1024},
		{0, 0, 1920, 1080, 0},
	}
	for _, c := range cases {
		if got := FitScale(c.iw, c.ih, c.cw, c.ch); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("FitScale(%d,%d,%d,%d) = %g, want %g", c.iw, c.ih, c.cw, c.ch, got, c.want)
		}
	}
}

func TestPlacements(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	f, _ := os.Create(filepath.Join(dir, "s1.png"))
	png.Encode(f, img)
	f.Close()
	os.WriteFile(filepath.Join(dir, "sb.json"), []byte(`{"bpm":120,"shots":[
		{"id":"s1","beats":[0,4],"frame":"s1.png"},{"id":"s2","beats":[4,8]}]}`), 0o644)
	sb, err := Load(filepath.Join(dir, "sb.json"))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := sb.Placements(context.Background(), 30, 1920, 1080, map[string]bool{"s1": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].In != 0 || ps[0].Out != 59 || ps[0].Scale != 3 || !filepath.IsAbs(ps[0].Path) {
		t.Fatalf("placements: %+v", ps)
	}
	if _, err := sb.Placements(context.Background(), 30, 1920, 1080, nil); err == nil || !strings.Contains(err.Error(), "s2 has no frame") {
		t.Errorf("a shot without a frame should fail: %v", err)
	}
	if _, err := sb.Placements(context.Background(), 30, 1920, 1080, map[string]bool{"s9": true}); err == nil {
		t.Error("an unknown shot id should fail")
	}
}
