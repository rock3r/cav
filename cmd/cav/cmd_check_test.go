package main

import (
	"encoding/json"
	"github.com/rock3r/cav/internal/bridge"
	"reflect"
	"testing"
)

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

func TestPerformanceFindings(t *testing.T) {
	twenty, hundred := 20.0, 100.0
	d := structureData{Layers: []structureLayer{{ID: "js#1", Name: "driver", Type: "javaScript"}, {ID: "dup#1", Name: "stars", Type: "duplicator", Copies: &hundred}, {ID: "dup#2", Name: "outer", Type: "duplicator", Copies: &twenty}, {ID: "sim#1", Name: "physics", Type: "forgeDynamicsShape"}}, Edges: []structureEdge{{From: "js#1", FromAttr: "output", To: "dup#1", ToAttr: "shapePosition.x"}, {From: "dup#1", FromAttr: "id", To: "dup#2", ToAttr: "shapes"}}}
	for i := 0; i < 32; i++ {
		d.Edges = append(d.Edges, structureEdge{From: "sim#1", To: "dup#1", ToAttr: "irrelevant"})
	}
	got := performanceFindings(d)
	kinds := map[string]finding{}
	for _, f := range got {
		if _, exists := kinds[f.Kind]; !exists {
			kinds[f.Kind] = f
		}
		if f.Attribution != "structural-risk-not-measured" || f.Severity != "warning" || f.Evidence == nil {
			t.Fatalf("unsupported attribution: %+v", f)
		}
	}
	for _, k := range []string{"perf-copy-javascript", "perf-nested-duplication", "perf-fanout", "perf-simulation"} {
		if _, ok := kinds[k]; !ok {
			t.Fatalf("missing %s: %v", k, got)
		}
	}
	if f := kinds["perf-copy-javascript"]; f.Layer != "dup#1" || f.Name != "stars" || *f.EstimatedCopies != 100 {
		t.Fatal(f)
	}
	if n := *kinds["perf-nested-duplication"].EstimatedCopies; n != 2000 {
		t.Fatal(n)
	}
	d.Layers[1].Copies = nil
	for _, f := range performanceFindings(d) {
		if f.Kind == "perf-nested-duplication" && f.EstimatedCopies != nil {
			t.Fatal("fabricated count")
		}
	}
}
func TestCheckPartialMetadataIsNotClean(t *testing.T) {
	fixtureBridge(t, func(req bridge.Request) {
		fixtureResult(t, req.ID, map[string]any{"start": 0, "end": 60, "fps": 30, "layers": []any{}, "edges": []any{}, "failures": []any{map[string]any{"inspection": "connections", "error": "unavailable"}}, "skipped": []any{}})
	})
	a := &app{json: true}
	if code := a.dispatch([]string{"check", "--quick", "--timeout", "1s"}); code != 0 {
		t.Fatal(code)
	}
	var data map[string]any
	if e := json.Unmarshal(a.op.Data, &data); e != nil {
		t.Fatal(e)
	}
	if data["clean"] != false || data["complete"] != false || len(data["failures"].([]any)) != 1 {
		t.Fatalf("partial reported clean: %v", data)
	}
}

func TestJavaScriptDuplicatedSourceAndGlobalTransform(t *testing.T) {
	d := structureData{Layers: []structureLayer{{ID: "js", Type: "javaScript"}, {ID: "text", Type: "textShape", Parent: "group"}, {ID: "group", Type: "group"}, {ID: "dup", Type: "duplicator"}}, Edges: []structureEdge{{From: "js", To: "text", ToAttr: "text"}, {From: "group", To: "dup", ToAttr: "shapes"}}}
	found := performanceFindings(d)
	if len(found) != 1 || found[0].Kind != "perf-copy-javascript" {
		t.Fatal(found)
	}
	d.Edges = []structureEdge{{From: "js", To: "dup", ToAttr: "position.x"}}
	if f := performanceFindings(d); len(f) != 0 {
		t.Fatalf("global transform mislabeled per-copy: %v", f)
	}
}

func TestSkippedSimulationVisualsDoNotReportEmpty(t *testing.T) {
	posts := 0
	fixtureBridge(t, func(req bridge.Request) {
		posts++
		fixtureResult(t, req.ID, map[string]any{"start": 0, "end": 60, "fps": 30, "totalLayers": 1, "layers": []any{map[string]any{"id": "sim#1", "name": "particles", "type": "particleShape"}}, "edges": []any{}})
	})
	a := &app{json: true}
	if code := a.dispatch([]string{"check", "--timeout", "1s"}); code != 0 {
		t.Fatal(code)
	}
	var data struct {
		Findings []finding        `json:"findings"`
		Skipped  []map[string]any `json:"skipped"`
	}
	if err := json.Unmarshal(a.op.Data, &data); err != nil {
		t.Fatal(err)
	}
	for _, f := range data.Findings {
		if f.Kind == "empty" {
			t.Fatal("uninspected visual result reported an empty comp")
		}
	}
	if posts != 1 || len(data.Skipped) == 0 {
		t.Fatalf("simulation visuals not skipped: posts=%d data=%+v", posts, data)
	}
}

func TestIncompleteLayerCoverageProfilesConsecutiveFrames(t *testing.T) {
	for _, tc := range []struct {
		name   string
		total  *int
		layers []structureLayer
		want   []int
	}{
		{"complete static", intPointer(1), []structureLayer{{Type: "basicShape"}}, []int{10, 20, 30}},
		{"truncated before simulation", intPointer(2), []structureLayer{{Type: "basicShape"}}, []int{10, 11, 12}},
		{"failed type", intPointer(1), []structureLayer{{Type: "unknown"}}, []int{10, 11, 12}},
		{"missing total", nil, []structureLayer{{Type: "basicShape"}}, []int{10, 11, 12}},
		{"known simulation", intPointer(1), []structureLayer{{Type: "particleShape"}}, []int{10, 11, 12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := profileFrames(structureData{Start: 10, End: 30, TotalLayers: tc.total, Layers: tc.layers}, 3)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("frames=%v want=%v", got, tc.want)
			}
		})
	}
}
func intPointer(n int) *int { return &n }

func TestWholeCompStillnessRequiresCompleteMotionCoverage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failures  []map[string]any
		skipped   []map[string]any
		wantStill bool
	}{
		{name: "complete", wantStill: true},
		{name: "frame budget", skipped: []map[string]any{{"inspection": "visual-frame", "frame": 20, "reason": "sample or time budget"}}},
		{name: "layer budget", skipped: []map[string]any{{"inspection": "visual-layers", "reason": "layer limit", "count": 1}}},
		{name: "failed evaluation", failures: []map[string]any{{"inspection": "visual-frame", "frame": 20, "error": "bounds unavailable"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			fixtureBridge(t, func(req bridge.Request) {
				posts++
				if posts == 1 {
					fixtureResult(t, req.ID, map[string]any{"comp": "original", "scenePath": "scratch.cv", "start": 0, "end": 120, "fps": 30, "totalLayers": 1, "layers": []any{map[string]any{"id": "shape", "type": "basicShape"}}})
					return
				}
				// Later motion is missing because inspection was skipped/failed. Its absence
				// must not become an affirmative claim that the rest of the comp is still.
				fixtureResult(t, req.ID, map[string]any{"start": 0, "end": 120, "fps": 30, "height": 1080, "layers": 1, "segs": [][3]float64{{0, 10, 0}}, "failures": tc.failures, "skipped": tc.skipped})
			})
			a := &app{json: true}
			if code := a.dispatch([]string{"check", "--timeout", "2s"}); code != 0 {
				t.Fatal(code)
			}
			var data struct {
				Findings []finding        `json:"findings"`
				Skipped  []map[string]any `json:"skipped"`
			}
			if err := json.Unmarshal(a.op.Data, &data); err != nil {
				t.Fatal(err)
			}
			still := false
			for _, f := range data.Findings {
				still = still || f.Kind == "still"
			}
			if still != tc.wantStill {
				t.Fatalf("still=%v want=%v: %+v", still, tc.wantStill, data)
			}
			if !tc.wantStill {
				found := false
				for _, s := range data.Skipped {
					found = found || s["inspection"] == "stillness"
				}
				if !found {
					t.Fatal("incomplete stillness coverage was not reported")
				}
			}
		})
	}
}

func TestProfileCoverageAndFailuresRemainSeparate(t *testing.T) {
	for _, tc := range []struct {
		name              string
		samples           []profileResult
		failures, skipped []map[string]any
	}{
		{name: "budget after sample", samples: []profileResult{{Frame: 0}}, skipped: []map[string]any{{"inspection": "profile-sample", "reason": "time budget", "remaining": float64(2)}}},
		{name: "budget before first sample", skipped: []map[string]any{{"inspection": "profile-sample", "reason": "time budget", "remaining": float64(3)}}},
		{name: "restore failure without samples", failures: []map[string]any{{"inspection": "playhead-restore", "error": "unavailable"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			fixtureBridge(t, func(req bridge.Request) {
				posts++
				if posts == 1 {
					fixtureResult(t, req.ID, map[string]any{"comp": "scratch", "scenePath": "scratch.cv", "start": 0, "end": 30, "fps": 30, "totalLayers": 1, "layers": []structureLayer{{ID: "sim", Type: "particleShape"}}})
					return
				}
				fixtureResult(t, req.ID, map[string]any{"profile": tc.samples, "failures": tc.failures, "skipped": tc.skipped, "restored": len(tc.failures) == 0, "restoreMs": 1})
			})
			a := &app{json: true}
			if code := a.dispatch([]string{"check", "--profile", "--timeout", "1s"}); code != exitOK {
				t.Fatal(code)
			}
			var data struct {
				Failures, Skipped []map[string]any
				Profile           []profileResult
				Complete, Clean   bool
			}
			if err := json.Unmarshal(a.op.Data, &data); err != nil {
				t.Fatal(err)
			}
			if posts != 2 || len(data.Profile) != len(tc.samples) || len(data.Failures) != len(tc.failures) || data.Complete || data.Clean {
				t.Fatalf("incorrect profile classification: %+v", data)
			}
			for _, item := range tc.skipped {
				found := false
				for _, got := range data.Skipped {
					found = found || reflect.DeepEqual(got, item)
				}
				if !found {
					t.Fatalf("missing skipped coverage %v: %+v", item, data)
				}
			}
			for _, sample := range data.Profile {
				if len(sample.Failures) != 0 {
					t.Fatalf("coverage or global failure assigned to a sample: %+v", sample)
				}
			}
		})
	}
}
