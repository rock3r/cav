package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/guide"
)

// Every link from a finding kind must name exactly one guide section, so editing a
// heading or a trap row cannot silently drop it from cav check.
func TestCheckGuideLinksResolve(t *testing.T) {
	ix, err := guide.Load(assets.Guide, "guide", guideTopics)
	if err != nil {
		t.Fatal(err)
	}
	for kind, refs := range checkGuide {
		for _, ref := range refs {
			if got := guideRef(ix, ref); len(got) != 1 {
				t.Errorf("%s: %v names %d sections, want 1", kind, ref, len(got))
			}
		}
	}
}

func TestGuideForFindingsAddsEachSectionOnce(t *testing.T) {
	out := []finding{
		{Kind: "font", Layer: "a"},
		{Kind: "font", Layer: "b"},
		{Kind: "offframe", Layer: "c"},
		{Kind: "text", Layer: "d"},
	}
	hits := guideForFindings(out)
	if len(hits) != 3 {
		t.Fatalf("got %d sections, want the font trap and two offframe traps: %+v", len(hits), hits)
	}
	if !strings.Contains(hits[0].Text, "cavalry.fontExists") {
		t.Errorf("first section is not the font trap: %q", hits[0].Text)
	}
	if len(out[0].Guide) != 1 || out[0].Guide[0] != "traps › Look and render › Missing font" || len(out[1].Guide) != 1 {
		t.Errorf("font findings link %v and %v", out[0].Guide, out[1].Guide)
	}
	if len(out[2].Guide) != 2 || out[3].Guide != nil {
		t.Errorf("offframe links %v, text links %v", out[2].Guide, out[3].Guide)
	}
	total := 0
	for _, h := range guideForFindings([]finding{{Kind: "perf-simulation"}, {Kind: "perf-copy-javascript"}, {Kind: "perf-sampled-keys"}, {Kind: "font"}, {Kind: "hidden"}}) {
		if len(h.Text) > checkGuideHitBytes {
			t.Errorf("%v: %d bytes", h.Path, len(h.Text))
		}
		total += len(h.Text)
	}
	if total > checkGuideTotalBytes {
		t.Errorf("guide sections add %d bytes, cap %d", total, checkGuideTotalBytes)
	}
}

func TestCheckPrintsTheTrapForAFinding(t *testing.T) {
	fixtureBridge(t, func(req bridge.Request) {
		fixtureResult(t, req.ID, map[string]any{"start": 0, "end": 60, "fps": 30, "edges": []any{}, "failures": []any{}, "skipped": []any{},
			"layers": []any{map[string]any{"id": "textShape#1", "name": "title", "type": "textShape", "missingFont": map[string]any{"family": "Nope Sans", "style": "Bold"}}}})
	})
	a := &app{json: true}
	if code := a.dispatch([]string{"check", "--quick", "--timeout", "1s"}); code != 0 {
		t.Fatal(code)
	}
	var data struct {
		Findings []finding   `json:"findings"`
		Guide    []guide.Hit `json:"guide"`
	}
	if e := json.Unmarshal(a.op.Data, &data); e != nil {
		t.Fatal(e)
	}
	if len(data.Findings) == 0 || data.Findings[0].Kind != "font" || len(data.Findings[0].Guide) != 1 {
		t.Fatalf("font finding without a guide link: %+v", data.Findings)
	}
	if len(data.Guide) != 1 || !strings.Contains(data.Guide[0].Text, "Missing font") {
		t.Fatalf("check did not include the font trap: %+v", data.Guide)
	}
}
