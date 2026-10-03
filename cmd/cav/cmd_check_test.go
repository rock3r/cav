package main

import (
	"encoding/json"
	"github.com/rock3r/cav/internal/bridge"
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
