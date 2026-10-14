package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/bridge"
)

func TestParseAspect(t *testing.T) {
	for _, c := range []struct {
		in   string
		w, h int
		ok   bool
	}{
		{"9:16", 9, 16, true}, {"9x16", 9, 16, true}, {" 9:16 ", 9, 16, true}, {"1:1", 1, 1, true}, {"1x1", 1, 1, true},
		{"16:9", 0, 0, false}, {"4:5", 0, 0, false}, {"", 0, 0, false}, {"9", 0, 0, false}, {"a:b", 0, 0, false}, {"9:16:1", 0, 0, false},
	} {
		w, h, err := parseAspect(c.in)
		if (err == nil) != c.ok || w != c.w || h != c.h {
			t.Errorf("parseAspect(%q) = %d, %d, %v", c.in, w, h, err)
		}
	}
}

func TestSceneReframeRefusesBadFlags(t *testing.T) {
	for _, args := range [][]string{
		{"scene", "reframe"},
		{"scene", "reframe", "--to", "16:9"},
		{"scene", "reframe", "--to", "9:16", "--samples", "0"},
		{"scene", "reframe", "--to", "9:16", "--samples", "25"},
		{"scene", "reframe", "--to", "9:16", "--scale", "0"},
		{"scene", "reframe", "--to", "9:16", "--scale", "-1"},
		{"scene", "reframe", "--to", "9:16", "--min-text", "0"},
		{"scene", "reframe", "--to", "9:16", "extra"},
	} {
		a := &app{json: true}
		if code := a.dispatch(args); code == 0 {
			t.Errorf("%v was accepted", args)
		}
	}
}

// reframeFixture is the job result of a 16:9 launch scene made into 9:16.
func reframeFixture(dry bool) map[string]any {
	return map[string]any{
		"source":  map[string]any{"id": "compNode#1", "name": "Launch", "w": 1920, "h": 1080},
		"target":  map[string]any{"id": "compNode#2", "name": "Launch 9x16", "w": 1080, "h": 1920},
		"key":     map[string]any{"mode": "rig", "layers": []any{map[string]any{"id": "group#1", "name": "rig"}}},
		"scale":   0.50625,
		"offset":  []float64{0, 0},
		"cover":   1.7778,
		"wrapped": []any{map[string]any{"id": "group#1", "name": "rig"}},
		"backgrounds": []any{
			map[string]any{"id": "basicShape#9", "name": "bg"},
		},
		"findings": []any{
			map[string]any{"kind": "small-text", "layer": "textShape#4", "name": "title", "detail": "text would be 20 px", "fix": "raise it"},
			map[string]any{"kind": "space", "detail": "the content fills 51 % of the height", "fix": "use the bands"},
		},
		"kept":             []any{map[string]any{"id": "sound#1", "name": "music", "type": "sound"}},
		"coverageComplete": true,
		"skipped":          0,
		"warnings":         []any{},
		"dryRun":           dry,
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestSceneReframeSendsTheJobAndReportsTheResult(t *testing.T) {
	var code string
	fixtureBridge(t, func(req bridge.Request) {
		b, err := os.ReadFile(req.File)
		if err != nil {
			t.Error(err)
		}
		code = string(b)
		fixtureResult(t, req.ID, reframeFixture(false))
	})
	a := &app{json: true}
	c := 0
	out := captureStdout(t, func() {
		c = a.dispatch([]string{"scene", "reframe", "--to", "9x16", "--comp", "Launch", "--name", "Vertical", "--key", "rig, cards", "--scale", "0.8", "--samples", "6", "--min-text", "30", "--timeout", "1m"})
	})
	if c != 0 {
		t.Fatal(c)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if data["scale"] != 0.50625 || data["dryRun"] != false || data["ok"] != true {
		t.Fatalf("json result changed: %v", data)
	}
	head, _, ok := strings.Cut(code, ";\n")
	if !ok || !strings.HasPrefix(head, "var opts = ") {
		t.Fatalf("job does not start with the options: %.80s", code)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(head, "var opts = ")), &opts); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(map[string]any{"source": "Launch", "to": []int{9, 16}, "name": "Vertical", "key": []string{"rig", "cards"}, "scale": 0.8, "samples": 6, "minText": 30.0, "dryRun": false, "budgetMs": 30000})
	got, _ := json.Marshal(opts)
	var w2 map[string]any
	_ = json.Unmarshal(want, &w2)
	want, _ = json.Marshal(w2)
	if string(got) != string(want) {
		t.Fatalf("options = %s, want %s", got, want)
	}
	if !strings.Contains(code, "api.duplicate(") || !strings.Contains(code, "cav.safeArea") {
		t.Fatal("the reframe job source is not part of the posted code")
	}
}

func TestSceneReframeDryRunSendsDryRun(t *testing.T) {
	var code string
	fixtureBridge(t, func(req bridge.Request) {
		b, err := os.ReadFile(req.File)
		if err != nil {
			t.Error(err)
		}
		code = string(b)
		fixtureResult(t, req.ID, reframeFixture(true))
	})
	a := &app{json: true}
	if c := a.dispatch([]string{"scene", "reframe", "--to", "1:1", "--dry-run"}); c != 0 {
		t.Fatal(c)
	}
	if !strings.Contains(code, `"dryRun":true`) || !strings.Contains(code, `"to":[1,1]`) {
		t.Fatalf("options missing: %.200s", code)
	}
}

func TestPrintReframeText(t *testing.T) {
	var r reframeResult
	b, _ := json.Marshal(reframeFixture(false))
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { printReframe(&r) })
	for _, want := range []string{
		`Launch 1920x1080: made "Launch 9x16" at 1080x1920, now the active comp`,
		"scale 0.506", "backgrounds: bg", "inside a fit group: rig",
		"- small-text: title: text would be 20 px", "  fix: raise it", "kept untouched (no position or scale): music", `cav sheet --comp "Launch 9x16"`, `cav check --comp "Launch 9x16"`, "The original comp was not changed.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output lacks %q:\n%s", want, out)
		}
	}
	r.DryRun = true
	out = captureStdout(t, func() { printReframe(&r) })
	if !strings.Contains(out, "would make") || !strings.Contains(out, "Nothing was changed") || strings.Contains(out, "cav sheet") {
		t.Errorf("dry-run text is wrong:\n%s", out)
	}
}
