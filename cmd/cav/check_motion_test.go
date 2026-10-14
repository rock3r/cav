package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/bridge"
)

func fp(v float64) *float64 { return &v }

// keyed is an attribute with keys at the given frames and values; interp may be nil.
func keyed(times []float64, values []float64, interp ...int) motionAttr {
	m := motionAttr{Attr: "position.x", Times: times}
	for _, v := range values {
		m.Values = append(m.Values, fp(v))
	}
	m.Interp = interp
	return m
}

func boolp(b bool) *bool { return &b }

// chainLayers are n null layers n0..n(n-1) with children, to pass liveness along.
func chainLayers(n int) []structureLayer {
	var out []structureLayer
	for i := 0; i < n; i++ {
		out = append(out, structureLayer{ID: fmt.Sprintf("n%d", i), Type: "null"})
	}
	return out
}

// chainEdges connect osc to n0, n0 to n1, ... and the last one to end. They are listed last
// to first, so a single pass over the edges cannot reach the end.
func chainEdges(n int) []structureEdge {
	prev := "osc"
	var out []structureEdge
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("n%d", i)
		out = append(out, structureEdge{From: prev, FromAttr: "output", To: id, ToAttr: "position.x"})
		prev = id
	}
	out = append(out, structureEdge{From: prev, FromAttr: "output", To: "end", ToAttr: "position.x"})
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func withEdges(d structureData, e ...structureEdge) structureData {
	d.Edges = e
	return d
}

// motionScene is a 10 s comp at 30 fps with a layer at the root.
func motionScene(layers ...structureLayer) structureData {
	for i := range layers {
		layers[i].Comp = "comp"
		if layers[i].Parent == "" {
			layers[i].Parent = "comp"
		}
		if layers[i].Boundaries == nil {
			layers[i].Boundaries = []int{0, 300}
		}
		if layers[i].Type == "" {
			layers[i].Type = "basicShape"
		}
	}
	return structureData{Start: 0, End: 299, FPS: 30, Comp: "comp", Layers: layers, Motion: &motionInfo{Complete: true}}
}

// A move in the first second, a hold of three seconds, then another move.
func gapKeys() motionAttr {
	return keyed([]float64{0, 30, 120, 150}, []float64{0, 100, 100, 200})
}

func TestDeadTimeFindings(t *testing.T) {
	tail := keyed([]float64{150, 299}, []float64{200, 300}) // moves to the very end
	tests := []struct {
		name     string
		d        structureData
		shots    []board.Shot
		flagged  [][2]int // from, to
		intended int
		skipped  bool
	}{
		{"three seconds without a move is flagged",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}),
			nil, [][2]int{{30, 120}}, 0, false},
		{"a gap of 1.9 s is not flagged",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{keyed([]float64{0, 30, 87, 117, 299}, []float64{0, 1, 1, 2, 3})}}),
			nil, nil, 0, false},
		{"equal values on both keys are a hold, not motion",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{keyed([]float64{0, 100, 200, 299}, []float64{5, 5, 5, 5})}}),
			nil, [][2]int{{0, 300}}, 0, false},
		{"an oscillator connected to a layer fills the gap",
			withEdges(motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}, structureLayer{ID: "osc", Type: "oscillator"}, structureLayer{ID: "b"}),
				structureEdge{From: "osc", FromAttr: "output", To: "b", ToAttr: "position.y"}),
			nil, nil, 0, false},
		{"an oscillator that drives nothing visible does not hide the gap",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}, structureLayer{ID: "osc", Type: "oscillator"}),
			nil, [][2]int{{30, 120}}, 0, false},
		{"a hidden layer with keys does not count",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}, structureLayer{ID: "h", Hidden: boolp(true), Motion: []motionAttr{keyed([]float64{0, 299}, []float64{0, 1})}}),
			nil, [][2]int{{30, 120}}, 0, false},
		{"a hidden parent and its subtree do not count as moving",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}},
				structureLayer{ID: "p", Type: "null", Hidden: boolp(true), Motion: []motionAttr{keyed([]float64{0, 299}, []float64{0, 1})}},
				structureLayer{ID: "c", Parent: "p", Type: "basicShape", Motion: []motionAttr{keyed([]float64{0, 299}, []float64{0, 1})}}),
			nil, [][2]int{{30, 120}}, 0, false},
		{"a child under a hidden ancestor does not count",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}},
				structureLayer{ID: "g", Type: "group", Hidden: boolp(true)},
				structureLayer{ID: "c", Parent: "g", Motion: []motionAttr{keyed([]float64{0, 299}, []float64{0, 1})}}),
			nil, [][2]int{{30, 120}}, 0, false},
		{"a hidden oscillator still drives a visible layer",
			withEdges(motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}, structureLayer{ID: "osc", Type: "oscillator", Hidden: boolp(true)}, structureLayer{ID: "b"}),
				structureEdge{From: "osc", FromAttr: "output", To: "b", ToAttr: "position.y"}),
			nil, nil, 0, false},
		{"a hidden oscillator driving a hidden layer does not count",
			withEdges(motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}, structureLayer{ID: "osc", Type: "oscillator"}, structureLayer{ID: "b", Hidden: boolp(true)}),
				structureEdge{From: "osc", FromAttr: "output", To: "b", ToAttr: "position.y"}),
			nil, [][2]int{{30, 120}}, 0, false},
		{"a layer at opacity 0 with nothing changing it is not on screen and does not move",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}},
				structureLayer{ID: "z", Opacity: fp(0), Motion: []motionAttr{keyed([]float64{0, 299}, []float64{0, 1})}}),
			nil, [][2]int{{30, 120}}, 0, false},
		{"opacity 0 under a parent at opacity 0 hides the child",
			motionScene(structureLayer{ID: "a", Boundaries: []int{0, 60}, Motion: []motionAttr{keyed([]float64{0, 59}, []float64{0, 1})}},
				structureLayer{ID: "g", Type: "group", Opacity: fp(0)},
				structureLayer{ID: "c", Parent: "g"}),
			nil, nil, 0, false},
		{"opacity 0 with keys on opacity is a fade, so the layer counts",
			motionScene(structureLayer{ID: "z", Opacity: fp(0), Motion: []motionAttr{{Attr: "opacity", Times: []float64{0, 299}, Values: []*float64{fp(0), fp(100)}}}}),
			nil, nil, 0, false},
		{"opacity 0 with an expression on opacity is not off",
			motionScene(structureLayer{ID: "z", Opacity: fp(0), Expressions: []string{"opacity"}}),
			nil, nil, 0, false},
		{"opacity 0 with a connection into opacity is not off",
			withEdges(motionScene(structureLayer{ID: "osc", Type: "oscillator"}, structureLayer{ID: "z", Opacity: fp(0)}),
				structureEdge{From: "osc", FromAttr: "output", To: "z", ToAttr: "opacity"}),
			nil, nil, 0, false},
		{"a long chain of drivers reaches its target",
			withEdges(motionScene(append([]structureLayer{{ID: "osc", Type: "oscillator"}}, append(chainLayers(30), structureLayer{ID: "end"})...)...),
				chainEdges(30)...),
			nil, nil, 0, false},
		{"a pre-comp is moving",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}, structureLayer{ID: "ref", Type: "compositionReference"}),
			nil, nil, 0, false},
		{"a layer with an expression is moving",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}, structureLayer{ID: "e", Expressions: []string{"scale.x"}}),
			nil, nil, 0, false},
		{"a simulation is moving",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}, structureLayer{ID: "p", Type: "particleShape"}),
			nil, nil, 0, false},
		{"a step key counts only at the second key",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{keyed([]float64{0, 100, 200}, []float64{0, 1, 2}, interpStep, interpStep)}}),
			nil, [][2]int{{0, 100}, {101, 200}, {201, 300}}, 0, false},
		{"keys with unknown values count as moving",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{{Attr: "color", Times: []float64{0, 299}, Values: []*float64{nil, nil}}}}),
			nil, nil, 0, false},
		{"too many keys count from the first to the last",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{{Attr: "position.x", Span: []float64{10, 299}}}}),
			nil, nil, 0, false},
		{"a hold shot over the gap makes it intended",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}),
			[]board.Shot{{ID: "s1", Beats: [2]float64{0, 2}}, {ID: "s2", Beats: [2]float64{2, 8}, Hold: true}, {ID: "s3", Beats: [2]float64{8, 20}}},
			nil, 1, false},
		{"a shot that is not a hold leaves the gap flagged",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}),
			[]board.Shot{{ID: "s1", Beats: [2]float64{0, 2}}, {ID: "s2", Beats: [2]float64{2, 8}}, {ID: "s3", Beats: [2]float64{8, 20}}},
			[][2]int{{30, 120}}, 0, false},
		{"a hold shot that covers only part of the gap leaves it flagged",
			motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), tail}}),
			[]board.Shot{{ID: "s1", Beats: [2]float64{0, 2}}, {ID: "s2", Beats: [2]float64{2, 5}, Hold: true}, {ID: "s3", Beats: [2]float64{5, 20}}},
			[][2]int{{30, 120}}, 0, false},
		{"a span with nothing on screen is not reported",
			motionScene(structureLayer{ID: "a", Boundaries: []int{0, 60}, Motion: []motionAttr{keyed([]float64{0, 59}, []float64{0, 1})}}),
			nil, nil, 0, false},
		{"incomplete motion metadata reports nothing and says so",
			func() structureData {
				d := motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys()}})
				d.Motion.Complete = false
				return d
			}(), nil, nil, 0, true},
		{"missing motion metadata reports nothing and skips nothing",
			func() structureData {
				d := motionScene(structureLayer{ID: "a"})
				d.Motion = nil
				return d
			}(), nil, nil, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sb *board.Board
			if tc.shots != nil {
				sb = &board.Board{BPM: 120, FPS: 30, Shots: tc.shots}
			}
			flagged, intended, _, skipped := deadTimeFindings(tc.d, sb, 2)
			var got [][2]int
			for _, f := range flagged {
				e := f.Evidence.(map[string]any)
				got = append(got, [2]int{e["from"].(int), e["to"].(int)})
				if f.Kind != "dead-time" || f.Severity != "warning" {
					t.Errorf("finding %+v", f)
				}
			}
			if len(got) != len(tc.flagged) {
				t.Fatalf("flagged %v, want %v", got, tc.flagged)
			}
			for i := range got {
				if got[i] != tc.flagged[i] {
					t.Errorf("flagged %v, want %v", got, tc.flagged)
				}
			}
			if len(intended) != tc.intended {
				t.Errorf("intended %+v, want %d", intended, tc.intended)
			}
			if (len(skipped) > 0) != tc.skipped {
				t.Errorf("skipped %+v, want %v", skipped, tc.skipped)
			}
		})
	}
}

func TestDeadTimeDetailNamesFramesAndSeconds(t *testing.T) {
	d := motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), keyed([]float64{150, 299}, []float64{200, 300})}})
	flagged, _, spans, _ := deadTimeFindings(d, nil, 2)
	if len(flagged) != 1 || len(spans) != 1 {
		t.Fatalf("%+v", flagged)
	}
	want := "no layer has moving keys or a live driver from frame 30 to 120 (3.0 s)"
	if flagged[0].Detail != want || !strings.Contains(flagged[0].Fix, `"hold": true`) {
		t.Errorf("detail %q fix %q", flagged[0].Detail, flagged[0].Fix)
	}
	if flagged[0].Evidence.(map[string]any)["seconds"] != 3.0 {
		t.Errorf("evidence %+v", flagged[0].Evidence)
	}
}

func TestDeadTimeMaxDeadMovesTheLimit(t *testing.T) {
	d := motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys(), keyed([]float64{150, 299}, []float64{200, 300})}})
	if f, _, _, _ := deadTimeFindings(d, nil, 3.5); len(f) != 0 {
		t.Errorf("3 s gap flagged with --max-dead 3.5: %+v", f)
	}
	if f, _, _, _ := deadTimeFindings(d, nil, 1); len(f) != 1 {
		t.Errorf("3 s gap not flagged with --max-dead 1: %+v", f)
	}
}

func TestDeadTimeDropsAStillGapInsideIt(t *testing.T) {
	spans := []frameSpan{{30, 120}}
	if !insideSpan([2]float64{40, 100}, spans) || !insideSpan([2]float64{30, 120}, spans) {
		t.Error("a gap inside the span should be dropped")
	}
	if insideSpan([2]float64{20, 100}, spans) || insideSpan([2]float64{40, 130}, spans) {
		t.Error("a gap that sticks out should stay")
	}
}

// motionFixture answers the check's metadata job with the scene of motionScene, and any other
// job with an empty result.
func motionFixture(t *testing.T, d structureData) {
	t.Helper()
	fixtureBridge(t, func(req bridge.Request) {
		fixtureResult(t, req.ID, d)
	})
}

func runDeadCheck(t *testing.T, args ...string) seamCheckOut {
	t.Helper()
	a := &app{json: true}
	if code := a.dispatch(append([]string{"check", "--quick", "--timeout", "30s"}, args...)); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var out seamCheckOut
	if err := json.Unmarshal(a.op.Data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func deadKinds(fs []finding) (n int) {
	for _, f := range fs {
		if f.Kind == "dead-time" {
			n++
		}
	}
	return
}

func TestQuickCheckReportsDeadTimeFromMetadata(t *testing.T) {
	d := motionScene(structureLayer{ID: "a", Name: "box", Motion: []motionAttr{gapKeys(), keyed([]float64{150, 299}, []float64{200, 300})}})
	total := 1
	cov := true
	d.TotalLayers, d.CoverageComplete, d.ScenePath = &total, &cov, "scratch.cv"
	t.Chdir(t.TempDir())
	motionFixture(t, d)
	out := runDeadCheck(t)
	if deadKinds(out.Findings) != 1 || out.Clean {
		t.Fatalf("findings %+v clean %v", out.Findings, out.Clean)
	}
	if len(out.Findings[0].Guide) == 0 || !strings.Contains(out.Findings[0].Guide[0], "camera holds") {
		t.Errorf("guide %+v", out.Findings[0].Guide)
	}
	out = runDeadCheck(t, "--max-dead", "4")
	if deadKinds(out.Findings) != 0 {
		t.Errorf("--max-dead 4: %+v", out.Findings)
	}
}

func TestQuickCheckMovesAHoldShotToIntended(t *testing.T) {
	d := motionScene(structureLayer{ID: "a", Name: "box", Motion: []motionAttr{gapKeys(), keyed([]float64{150, 299}, []float64{200, 300})}})
	total := 1
	d.TotalLayers, d.ScenePath = &total, "scratch.cv"
	t.Chdir(t.TempDir())
	sb := &board.Board{BPM: 120, FPS: 30, Seconds: 10, Width: 1920, Height: 1080, Shots: []board.Shot{
		{ID: "s1", Beats: [2]float64{0, 2}, What: "a", Why: "w"},
		{ID: "s2", Beats: [2]float64{2, 8}, What: "b", Why: "w", Hold: true},
		{ID: "s3", Beats: [2]float64{8, 20}, What: "c", Why: "w"},
	}}
	if err := sb.Save("storyboard.json"); err != nil {
		t.Fatal(err)
	}
	motionFixture(t, d)
	out := runDeadCheck(t)
	if deadKinds(out.Findings) != 0 || deadKinds(out.Intended) != 1 || out.Intended[0].Name != "s2" {
		t.Fatalf("findings %+v intended %+v", out.Findings, out.Intended)
	}
}

func TestQuickCheckSkipsDeadTimeWhenMotionIsIncomplete(t *testing.T) {
	d := motionScene(structureLayer{ID: "a", Motion: []motionAttr{gapKeys()}})
	d.Motion.Complete = false
	total := 1
	d.TotalLayers, d.ScenePath = &total, "scratch.cv"
	t.Chdir(t.TempDir())
	motionFixture(t, d)
	out := runDeadCheck(t)
	found := false
	for _, s := range out.Skipped {
		found = found || (s["inspection"] == "dead-time" && s["reason"] == "incomplete motion metadata")
	}
	if !found || deadKinds(out.Findings) != 0 || out.Clean {
		t.Fatalf("skipped %+v findings %+v", out.Skipped, out.Findings)
	}
}

func TestFindingHeadOmitsWhatIsEmpty(t *testing.T) {
	for _, tc := range []struct {
		f    finding
		want string
	}{
		{finding{Kind: "dead-time"}, "dead-time"},
		{finding{Kind: "dead-time", Name: "s2"}, `dead-time "s2"`},
		{finding{Kind: "text", Layer: "textShape#1", Name: "Title"}, `text textShape#1 "Title"`},
		{finding{Kind: "offframe", Layer: "basicShape#1"}, "offframe basicShape#1"},
	} {
		if got := findingHead(tc.f); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestCheckRejectsAMaxDeadThatIsNotPositive(t *testing.T) {
	for _, v := range []string{"0", "-1", "NaN"} {
		a := &app{json: true}
		if code := a.dispatch([]string{"check", "--quick", "--max-dead", v}); code == 0 {
			t.Errorf("--max-dead %s accepted", v)
		}
	}
}
