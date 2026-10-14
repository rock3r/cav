package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/bridge"
)

func TestSeamSpike(t *testing.T) {
	cases := []struct {
		name              string
		before, at, after float64
		threshold         float64
		want              bool
	}{
		{"pop-in: nothing before or after", 0, 0.30, 0, 0.05, true},
		{"pan: three similar changes", 0.30, 0.32, 0.31, 0.05, false},
		{"cross-fade: three equal changes", 0.4, 0.4, 0.4, 0.05, false},
		{"small change below the threshold", 0, 0.04, 0, 0.05, false},
		{"exactly twice the neighbours", 0.1, 0.2, 0.05, 0.05, true},
		{"just under twice the neighbours", 0.1, 0.19, 0.05, 0.05, false},
		{"a busy frame after hides a pop", 0, 0.3, 0.2, 0.05, false},
	}
	for _, c := range cases {
		if got := seamSpike(c.before, c.at, c.after, c.threshold); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestBoundaryFrames(t *testing.T) {
	d := structureData{Start: 0, End: 120, FPS: 30, Comp: "compNode#1", Layers: []structureLayer{
		{Name: "intro", Type: "group", Comp: "compNode#1", Parent: "compNode#1", Boundaries: []int{0, 40}},
		{Name: "middle", Type: "group", Comp: "compNode#1", Parent: "", Boundaries: []int{45, 90}},
		{Name: "late", Type: "compositionReference", Comp: "compNode#1", Parent: "compNode#1", Boundaries: []int{200, 220}},
		{Name: "inner", Type: "group", Comp: "compNode#1", Parent: "group#1", Boundaries: []int{50, 60}},
		{Name: "dot", Type: "basicShape", Comp: "compNode#1", Parent: "compNode#1", Boundaries: []int{30, 60}},
		{Name: "other comp", Type: "group", Comp: "compNode#2", Parent: "compNode#2", Boundaries: []int{70, 80}},
		{Name: "twin", Type: "group", Comp: "compNode#1", Parent: "compNode#1", Boundaries: []int{45, 90}},
	}}
	frames := func(bs []seamBoundary) []int {
		var out []int
		for _, b := range bs {
			out = append(out, b.Frame)
		}
		return out
	}
	// Without a storyboard: top-level groups and pre-comps, in range, once each.
	got := boundaryFrames(d, nil, map[int]bool{45: true})
	if f := frames(got); len(f) != 1 || f[0] != 45 || got[0].Name != "middle" || got[0].Shot || !got[0].Intended {
		t.Fatalf("group boundaries: %+v", got)
	}
	// With a storyboard, the shot starts win and the layers are ignored.
	sb := &board.Board{BPM: 120, Shots: []board.Shot{
		{ID: "s1", Beats: [2]float64{0, 2}},
		{ID: "s2", Beats: [2]float64{2, 4}},
		{ID: "s3", Beats: [2]float64{4, 6}, Cut: true},
		{ID: "s4", Beats: [2]float64{6, 8}},
		{ID: "s5", Beats: [2]float64{20, 22}}, // starts after the comp ends
	}}
	got = boundaryFrames(d, sb, map[int]bool{90: true})
	if f := frames(got); len(f) != 3 || f[0] != 30 || f[1] != 60 || f[2] != 90 {
		t.Fatalf("shot boundaries: %+v", got)
	}
	if got[0].Intended || !got[1].Intended || !got[2].Intended || !got[0].Shot || got[1].Name != "s3" {
		t.Errorf("intended marks: %+v", got)
	}
	// A storyboard with one shot has no boundaries of its own: the layers are used.
	one := &board.Board{BPM: 120, Shots: sb.Shots[:1]}
	if f := frames(boundaryFrames(d, one, nil)); len(f) != 1 || f[0] != 45 {
		t.Errorf("one shot: %v", f)
	}
	// Frames the check renders: two before, the frame itself and one after, inside the comp.
	if f := seamFrames(d, []seamBoundary{{Frame: 1}, {Frame: 120}}); len(f) != 6 || f[0] != 0 || f[5] != 120 {
		t.Errorf("seam frames: %v", f)
	}
}

// writeSeamFrames writes a small grey PNG for each requested frame, as Cavalry would.
func writeSeamFrames(t *testing.T, req bridge.Request, grey func(int) uint8) []string {
	t.Helper()
	b, err := os.ReadFile(req.File)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`var sample = (\{[^\n]*\});`).FindSubmatch(b)
	if len(m) != 2 {
		t.Fatal("no sample in the render job")
	}
	var sample struct {
		Frames []int  `json:"frames"`
		Dir    string `json:"dir"`
	}
	if err = json.Unmarshal(m[1], &sample); err != nil {
		t.Fatal(err)
	}
	dir := filepath.FromSlash(sample.Dir)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range sample.Frames {
		img := image.NewGray(image.Rect(0, 0, 8, 8))
		for x := range 8 {
			for y := range 8 {
				img.SetGray(x, y, color.Gray{Y: grey(f)})
			}
		}
		p := filepath.Join(dir, "f_"+itoa(f)+".png")
		out, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(out, img); err != nil {
			t.Fatal(err)
		}
		out.Close()
		paths = append(paths, filepath.ToSlash(p))
	}
	return paths
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// seamGrey is the picture over time: s2 (frame 30) pops, s3 (frame 60) cross-fades, and
// s4 (frame 90) is a hard cut.
func seamGrey(f int) uint8 {
	switch {
	case f < 30:
		return 255
	case f <= 58:
		return 0
	case f == 59:
		return 60
	case f == 60:
		return 120
	case f == 61:
		return 180
	case f < 90:
		return 240
	}
	return 0
}

type seamCheckOut struct {
	Findings []finding        `json:"findings"`
	Intended []finding        `json:"intended"`
	Seams    []seamCheck      `json:"seams"`
	Clean    bool             `json:"clean"`
	Skipped  []map[string]any `json:"skipped"`
	Failures []map[string]any `json:"failures"`
}

// seamFixture answers the check's jobs: structure, visual checks, scene info and renders.
func seamFixture(t *testing.T) (renders *int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	fixtureBridge(t, func(req bridge.Request) {
		b, err := os.ReadFile(req.File)
		if err != nil {
			t.Error(err)
			return
		}
		code := string(b)
		switch {
		case strings.Contains(code, "sample.frames"):
			mu.Lock()
			n++
			mu.Unlock()
			fixtureResult(t, req.ID, writeSeamFrames(t, req, seamGrey))
		case strings.Contains(code, "expected.comp"):
			fixtureResult(t, req.ID, map[string]any{"start": 0, "end": 120, "fps": 30, "layers": 0, "segs": []any{}, "failures": []any{}, "skipped": []any{}})
		case strings.Contains(code, "precomp-coverage"):
			fixtureResult(t, req.ID, map[string]any{"start": 0, "end": 120, "fps": 30, "comp": "compNode#1", "scenePath": "scratch.cv", "totalLayers": 2, "failures": []any{}, "skipped": []any{},
				"layers": []any{map[string]any{"id": "basicShape#1", "name": "box", "type": "basicShape", "comp": "compNode#1"},
					map[string]any{"id": "group#1", "name": "Pre", "type": "group", "comp": "compNode#1", "parent": "", "boundaries": []int{45, 90}}}, "edges": []any{}})
		default:
			fixtureResult(t, req.ID, map[string]any{"scenePath": "scratch.cv", "comp": map[string]any{"id": "compNode#1", "name": "scratch", "fps": 30, "startFrame": 0, "endFrame": 120}})
		}
	})
	return &n
}

func runSeamCheck(t *testing.T, args ...string) (seamCheckOut, int) {
	t.Helper()
	a := &app{json: true}
	code := a.dispatch(append([]string{"check", "--timeout", "60s"}, args...))
	var out seamCheckOut
	if a.op != nil && len(a.op.Data) > 0 {
		if err := json.Unmarshal(a.op.Data, &out); err != nil {
			t.Fatal(err)
		}
	}
	return out, code
}

func seamStoryboard(t *testing.T, cut bool) {
	seamStoryboardShots(t, cut, false)
}

// seamStoryboardShots writes the four shots; with calm, s4 ends at beat 7 and a last shot s5
// (marked as a cut) starts at frame 105, where the picture does not change.
func seamStoryboardShots(t *testing.T, cut, calm bool) {
	t.Helper()
	t.Chdir(t.TempDir())
	sb := &board.Board{BPM: 120, Seconds: 4, Width: 1920, Height: 1080, Shots: []board.Shot{
		{ID: "s1", Beats: [2]float64{0, 2}, What: "a", Why: "w"},
		{ID: "s2", Beats: [2]float64{2, 4}, What: "b", Why: "w"},
		{ID: "s3", Beats: [2]float64{4, 6}, What: "c", Why: "w"},
		{ID: "s4", Beats: [2]float64{6, 8}, What: "d", Why: "w", Cut: cut},
	}}
	if calm {
		sb.Shots[3].Beats[1] = 7
		sb.Shots = append(sb.Shots, board.Shot{ID: "s5", Beats: [2]float64{7, 8}, What: "e", Why: "w", Cut: true})
	}
	if err := sb.Save("storyboard.json"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFlagsThePopAndNotTheCrossFade(t *testing.T) {
	renders := seamFixture(t)
	seamStoryboard(t, true)
	out, code := runSeamCheck(t)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var seams []finding
	for _, f := range out.Findings {
		if f.Kind == "seam" {
			seams = append(seams, f)
		}
	}
	if len(seams) != 1 || seams[0].Name != "s2" || seams[0].Severity != "warning" {
		t.Fatalf("want one seam finding for s2, got %+v", out.Findings)
	}
	if !strings.Contains(seams[0].Detail, "frame 30 (start of shot s2)") || !strings.Contains(seams[0].Detail, "100 %") {
		t.Errorf("detail %q", seams[0].Detail)
	}
	if ev, _ := seams[0].Evidence.(map[string]any); ev["before"] == "" || ev["after"] == "" || ev["changed"] != 1.0 || ev["changedBefore"] != 0.0 {
		t.Errorf("evidence %+v", seams[0].Evidence)
	}
	if len(seams[0].Guide) != 1 || !strings.Contains(seams[0].Guide[0], "Check both sides of every boundary") {
		t.Errorf("guide %v", seams[0].Guide)
	}
	// The cut shot is intended: listed, not counted.
	if len(out.Intended) != 1 || out.Intended[0].Name != "s4" || out.Intended[0].Severity == "warning" {
		t.Errorf("intended %+v", out.Intended)
	}
	if len(out.Seams) != 3 || out.Seams[1].Flagged || out.Seams[1].Name != "s3" || out.Seams[2].Flagged || !out.Seams[2].Intended {
		t.Errorf("seams %+v", out.Seams)
	}
	if out.Clean || *renders != 1 {
		t.Errorf("clean=%v renders=%d", out.Clean, *renders)
	}
}

func TestCheckAllowCutsMakesAPopIntended(t *testing.T) {
	seamFixture(t)
	seamStoryboard(t, false)
	out, code := runSeamCheck(t, "--allow-cuts", "30,90")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, f := range out.Findings {
		if f.Kind == "seam" {
			t.Errorf("an intended cut was flagged: %+v", f)
		}
	}
	if len(out.Intended) != 2 || out.Intended[0].Name != "s2" || out.Intended[1].Name != "s4" {
		t.Errorf("intended %+v", out.Intended)
	}
	// A high threshold flags nothing; an invalid limit is a usage error.
	if _, code = runSeamCheck(t, "--seam-threshold", "2"); code != exitError {
		t.Errorf("threshold 2: exit %d", code)
	}
	if _, code = runSeamCheck(t, "--max-seams", "0"); code != exitError {
		t.Errorf("max-seams 0: exit %d", code)
	}
}

func TestCheckQuickAndLimitsSkipSeams(t *testing.T) {
	renders := seamFixture(t)
	seamStoryboard(t, false)
	out, code := runSeamCheck(t, "--quick")
	if code != 0 || *renders != 0 {
		t.Fatalf("quick: exit %d, %d render jobs", code, *renders)
	}
	found := false
	for _, s := range out.Skipped {
		found = found || (s["inspection"] == "seams" && s["reason"] == "quick mode")
	}
	if !found || len(out.Seams) != 0 {
		t.Errorf("quick skipped %+v", out.Skipped)
	}
	// Three boundaries, two allowed: the rest is skipped and counted.
	out, _ = runSeamCheck(t, "--max-seams", "2")
	found = false
	for _, s := range out.Skipped {
		found = found || (s["inspection"] == "seams" && s["count"] == 1.0)
	}
	if !found || len(out.Seams) != 2 {
		t.Errorf("limit skipped %+v, seams %d", out.Skipped, len(out.Seams))
	}
}

func TestCheckListsEveryMarkedCutEvenWhenCalm(t *testing.T) {
	seamFixture(t)
	seamStoryboardShots(t, false, true)
	out, code := runSeamCheck(t)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var calm *finding
	for i, f := range out.Intended {
		if f.Name == "s5" {
			calm = &out.Intended[i]
		}
	}
	if calm == nil || !strings.Contains(calm.Detail, "measured and is calm") {
		t.Fatalf("a marked cut with no change must still be listed: %+v", out.Intended)
	}
	for _, f := range out.Findings {
		if f.Kind == "seam" && f.Name == "s5" {
			t.Errorf("a marked cut was flagged: %+v", f)
		}
	}
}

func TestCheckWithCompIgnoresTheStoryboard(t *testing.T) {
	seamFixture(t)
	seamStoryboard(t, false)
	// The storyboard has shots at 30, 60 and 90. For another comp, the layers decide.
	out, code := runSeamCheck(t, "--comp", "Pre-comp")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(out.Seams) != 1 || out.Seams[0].Frame != 45 || out.Seams[0].Name != "Pre" || out.Seams[0].Shot {
		t.Fatalf("seams %+v", out.Seams)
	}
}
