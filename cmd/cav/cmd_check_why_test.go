package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/bridge"
)

func metadataFixture(t *testing.T) {
	t.Helper()
	fixtureBridge(t, func(req bridge.Request) {
		fixtureResult(t, req.ID, map[string]any{"start": 0, "end": 60, "fps": 30, "totalLayers": 1, "failures": []any{}, "skipped": []any{},
			"layers": []any{map[string]any{"id": "basicShape#1", "name": "box", "type": "basicShape"}}, "edges": []any{}})
	})
}

type checkOut struct {
	Findings []finding        `json:"findings"`
	Clean    bool             `json:"clean"`
	Failures []map[string]any `json:"failures"`
	Board    string           `json:"board"`
}

func runQuickCheck(t *testing.T, extra ...string) (checkOut, int) {
	t.Helper()
	a := &app{json: true}
	code := a.dispatch(append([]string{"check", "--quick", "--timeout", "2s"}, extra...))
	var out checkOut
	if a.op != nil && len(a.op.Data) > 0 {
		if err := json.Unmarshal(a.op.Data, &out); err != nil {
			t.Fatal(err)
		}
	}
	return out, code
}

func TestCheckWarnsAboutShotsWithoutWhy(t *testing.T) {
	metadataFixture(t)
	t.Chdir(t.TempDir())
	sb := &board.Board{BPM: 120, Seconds: 12, Width: 1920, Height: 1080, Shots: []board.Shot{
		{ID: "s1", Beats: [2]float64{0, 4}, What: "the dashboard", Why: "the viewer sees where the numbers live"},
		{ID: "s2", Beats: [2]float64{4, 8}, What: "a filter opens"},
		{ID: "s3", Beats: [2]float64{8, 12}, What: "the export button, which is a very long description that goes past sixty characters", Why: whyPlaceholder},
	}}
	if err := sb.Save("storyboard.json"); err != nil {
		t.Fatal(err)
	}
	out, code := runQuickCheck(t)
	if code != 0 {
		t.Fatal(code)
	}
	var why []finding
	for _, f := range out.Findings {
		if f.Kind == "shot-why" {
			why = append(why, f)
		}
	}
	if len(why) != 2 || why[0].Name != "s2" || why[1].Name != "s3" {
		t.Fatalf("want shot-why for s2 and s3, got %+v", out.Findings)
	}
	for _, f := range why {
		if f.Severity != "warning" || len(f.Guide) == 0 || !strings.Contains(f.Guide[0], "Story and message") {
			t.Errorf("finding without severity or guide link: %+v", f)
		}
	}
	if ev, _ := why[1].Evidence.(map[string]any); len(ev["what"].(string)) != 60 || ev["shot"] != "s3" {
		t.Errorf("evidence %+v", why[1].Evidence)
	}
	if out.Clean || !strings.HasSuffix(out.Board, "storyboard.json") {
		t.Errorf("clean=%v board=%q", out.Clean, out.Board)
	}
}

func TestCheckBoardFlagAndMissingStoryboard(t *testing.T) {
	metadataFixture(t)
	t.Chdir(t.TempDir())
	// No storyboard: nothing added.
	out, code := runQuickCheck(t)
	if code != 0 || len(out.Findings) != 0 || out.Board != "" {
		t.Fatalf("no storyboard must add nothing: %+v code %d", out, code)
	}
	// An explicit file that is missing is a usage error.
	if _, code = runQuickCheck(t, "--board", "nope.json"); code != exitError {
		t.Fatalf("missing --board: exit %d", code)
	}
	// A storyboard that does not load is a failed inspection, not a crash.
	os.WriteFile("bad.json", []byte("{not json"), 0o644)
	out, code = runQuickCheck(t, "--board", "bad.json")
	if code != 0 || len(out.Failures) == 0 || out.Failures[0]["inspection"] != "storyboard" || out.Clean {
		t.Fatalf("bad storyboard: %+v code %d", out, code)
	}
}

func TestShotWhyNeverStopsFramesOrMotion(t *testing.T) {
	// A shot with a real "what" but the "why" placeholder is not refused by the frame check.
	sb := &board.Board{Shots: []board.Shot{{ID: "s1", What: "a logo", Why: whyPlaceholder}, {ID: "s2", What: "a logo"}}}
	if err := placeholderShots(sb, func(board.Shot) bool { return true }); err != nil {
		t.Fatalf("why must not make frames refuse: %v", err)
	}
	if !missingWhy(sb.Shots[0]) || !missingWhy(sb.Shots[1]) || missingWhy(board.Shot{Why: " because "}) {
		t.Error("missingWhy")
	}
}
