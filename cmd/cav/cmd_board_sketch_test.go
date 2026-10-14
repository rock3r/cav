package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
)

const testSketch = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080">
<rect x="100" y="100" width="400" height="300" fill="#f5f7fa"/>
<circle cx="900" cy="500" r="80" fill="#ff4d2e"/>
</svg>`

// sketchProject is a project folder with a three-shot storyboard, no image keys and no
// Hugging Face cache, so the default image order ends at sketch.
func sketchProject(t *testing.T) (dir, path string) {
	t.Helper()
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	t.Setenv("HF_HUB_CACHE", t.TempDir())
	dir = t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir(".cav", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("board", 0o755); err != nil {
		t.Fatal(err)
	}
	sb := &board.Board{BPM: 120, Seconds: 12, Width: 1920, Height: 1080,
		Style: board.Style{Palette: []string{"#0b0d12", "#f5f7fa", "#ff4d2e"}},
		Shots: []board.Shot{
			{ID: "s1", Beats: [2]float64{0, 4}, What: "the logo lands"},
			{ID: "s2", Beats: [2]float64{4, 8}, What: "the tagline slides in"},
			{ID: "s3", Beats: [2]float64{8, 12}, What: "the logo holds"},
		}}
	path = filepath.Join(dir, "storyboard.json")
	if err := sb.Save(path); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func writeSVG(t *testing.T, id, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join("board", id+".svg"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// closedBridge points cav at a port where nothing listens, as when Cavalry is not open.
func closedBridge(t *testing.T) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	l.Close()
	t.Setenv("CAV_BRIDGE_HOST", "127.0.0.1")
	t.Setenv("CAV_BRIDGE_PORT", port)
	t.Setenv("CAV_SPOOL", "")
	t.Setenv("CAV_RELAYED", "1")
}

// fakeSketchCavalry answers the jobs of a sketch run and records their order.
type fakeSketchCavalry struct {
	mu           sync.Mutex
	state        map[string]any
	jobs         []string
	svgs         map[string]string // the normalized SVG that each render job was given
	failure      string            // a render job fails with this message
	refuseScene  bool              // the scratch scene setup finds the scene changed
	failTempComp bool              // the temporary comp setup returns no comp id
	codes        map[string]string // the code of the last job of each kind
}

func (f *fakeSketchCavalry) kinds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.jobs...)
}

func newFakeSketchCavalry(t *testing.T, state map[string]any) *fakeSketchCavalry {
	t.Helper()
	f := &fakeSketchCavalry{state: state, svgs: map[string]string{}, codes: map[string]string{}}
	fixtureBridge(t, func(req bridge.Request) {
		raw, err := os.ReadFile(req.File)
		if err != nil {
			t.Error(err)
			return
		}
		code := string(raw)
		f.mu.Lock()
		defer f.mu.Unlock()
		var kind string
		var value any = true
		switch {
		case strings.Contains(code, "var sample = "):
			kind = "render"
			line := strings.SplitN(strings.TrimPrefix(code, "var sample = "), ";\n", 2)[0]
			var s struct {
				SVG, Out, Comp string
				Scale          int
			}
			if err := json.Unmarshal([]byte(line), &s); err != nil {
				t.Errorf("render sample %q: %v", line, err)
				return
			}
			svg, _ := os.ReadFile(filepath.FromSlash(s.SVG))
			f.svgs[filepath.Base(s.SVG)] = string(svg)
			if f.failure != "" {
				f.jobs = append(f.jobs, kind)
				fixtureFailure(t, req.ID, f.failure)
				return
			}
			var b bytes.Buffer
			if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1920, 1080))); err != nil {
				t.Error(err)
				return
			}
			if err := os.WriteFile(filepath.FromSlash(s.Out)+".png", b.Bytes(), 0o644); err != nil {
				t.Error(err)
				return
			}
			value = map[string]any{"png": s.Out + ".png", "layers": 3}
		case strings.Contains(code, "getCavalryVersion"):
			kind, value = "state", f.state
		case strings.Contains(code, "createComp('cav.sketch')"):
			kind, value = "temp-comp", "comp#8"
			if f.failTempComp {
				f.jobs = append(f.jobs, kind)
				fixtureFailure(t, req.ID, "timed out")
				return
			}
		case strings.Contains(code, "api.newScene()"):
			kind, value = "scratch-scene", "comp#7"
			if f.refuseScene {
				f.jobs = append(f.jobs, kind)
				fixtureFailure(t, req.ID, sketchRefusedPrefix+"/work/intro.cv")
				return
			}
		case strings.Contains(code, "api.openScene("):
			kind = "reopen"
		case strings.Contains(code, "opened scene changed"):
			kind, value = "reopened-check", map[string]any{"unsaved": false, "path": f.state["path"]}
		case strings.Contains(code, "var doomed"):
			kind = "temp-comp-restore"
		default:
			t.Errorf("unexpected job:\n%s", code)
			return
		}
		f.jobs = append(f.jobs, kind)
		f.codes[kind] = code
		fixtureResult(t, req.ID, value)
	})
	return f
}

func fixtureFailure(t *testing.T, id, msg string) {
	t.Helper()
	b, err := json.Marshal(bridge.Result{Type: "result", ID: id, OK: false, Error: &bridge.ScriptError{Message: msg}, BridgeVersion: "1.0.1", Protocol: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.JobsDir(), id+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// framesJSON runs cav board frames with JSON output and returns what it printed.
func framesJSON(t *testing.T, args ...string) (map[string]any, string, error) {
	t.Helper()
	var err error
	out, errOut := captureStreams(t, func() { err = boardFrames(&app{json: true}, args) })
	var got map[string]any
	if err == nil {
		if e := json.Unmarshal([]byte(out), &got); e != nil {
			t.Fatalf("not JSON: %q (%v)", out, e)
		}
	}
	return got, errOut, err
}

func loadBoard(t *testing.T, path string) *board.Board {
	t.Helper()
	sb, err := board.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return sb
}

func sourcesOf(sb *board.Board) string {
	var s []string
	for _, x := range sb.Shots {
		s = append(s, x.ID+"="+x.Source)
	}
	return strings.Join(s, " ")
}

func savedScene() map[string]any {
	return map[string]any{"unsaved": false, "path": "/work/intro.cv", "layers": 4, "comp": "comp#1", "frame": 30, "version": "2.8.0"}
}

func TestSketchRunWithoutSVGsDrawsCardsAndAsksForSketches(t *testing.T) {
	_, path := sketchProject(t)
	closedBridge(t) // Cavalry is never asked: there is nothing to sketch
	got, _, err := framesJSON(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if got["service"] != "greybox" {
		t.Errorf("service = %v", got["service"])
	}
	need, _ := got["needSketch"].([]any)
	if len(need) != 3 || !strings.Contains(got["sketchNote"].(string), "write board/s1.svg, board/s2.svg, board/s3.svg for sketch frames") {
		t.Errorf("needSketch %v, note %v", got["needSketch"], got["sketchNote"])
	}
	if s := sourcesOf(loadBoard(t, path)); s != "s1=greybox s2=greybox s3=greybox" {
		t.Errorf("sources: %s", s)
	}
}

func TestSketchRunRefusesBeforeTouchingCavalry(t *testing.T) {
	_, path := sketchProject(t)
	f := newFakeSketchCavalry(t, savedScene())
	writeSVG(t, "s1", testSketch)

	_, _, err := framesJSON(t, "--service", "sketch", path)
	if err == nil || !strings.Contains(err.Error(), "s2, s3: write board/s2.svg, board/s3.svg first") {
		t.Fatalf("missing SVGs: %v", err)
	}
	writeSVG(t, "s2", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"><script>x()</script></svg>`)
	writeSVG(t, "s3", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 500 500"/>`)
	for _, args := range [][]string{{path}, {"--service", "sketch", path}} {
		_, _, err = framesJSON(t, args...)
		if err == nil || !strings.Contains(err.Error(), "board/s2.svg: line 1: <script> is not allowed") ||
			!strings.Contains(err.Error(), "board/s3.svg: line 1: the canvas is 1:1 but the storyboard is 16:9") {
			t.Fatalf("args %v: want both bad SVGs listed, got %v", args, err)
		}
	}
	if jobs := f.kinds(); len(jobs) != 0 {
		t.Errorf("Cavalry was asked: %v", jobs)
	}
	if sb := loadBoard(t, path); sb.Shots[0].Frame != "" {
		t.Errorf("a refused run made a frame: %+v", sb.Shots[0])
	}
	if fileExists("board/s1.png") {
		t.Error("a refused run wrote a PNG")
	}
}

func TestSketchFramesRenderThroughCavalry(t *testing.T) {
	_, path := sketchProject(t)
	f := newFakeSketchCavalry(t, savedScene())
	writeSVG(t, "s1", testSketch)
	writeSVG(t, "s2", testSketch)

	got, _, err := framesJSON(t, path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"state", "scratch-scene", "render", "render", "reopen", "reopened-check"}
	if jobs := f.kinds(); strings.Join(jobs, ",") != strings.Join(want, ",") {
		t.Fatalf("jobs %v, want %v", jobs, want)
	}
	if !strings.Contains(f.codes["scratch-scene"], "resolution: [1920, 1080], backgroundColor: \"#0b0d12\"") {
		t.Errorf("scratch scene: %s", f.codes["scratch-scene"])
	}
	if !strings.Contains(f.codes["reopen"], `/work/intro.cv`) || !strings.Contains(f.codes["reopened-check"], "frame = 30") ||
		!strings.Contains(f.codes["reopened-check"], `"comp#1"`) {
		t.Errorf("the user's scene, comp and playhead were not restored:\n%s\n%s", f.codes["reopen"], f.codes["reopened-check"])
	}
	// Cavalry imported a normalized copy, with the background first.
	if svg := f.svgs["s1.svg"]; !strings.Contains(svg, `viewBox="0 0 1920 1080"`) || !strings.Contains(svg, `fill="#0b0d12"`) {
		t.Errorf("the imported copy is not normalized: %s", svg)
	}
	sb := loadBoard(t, path)
	if s := sourcesOf(sb); s != "s1=sketch s2=sketch s3=greybox" {
		t.Errorf("sources: %s", s)
	}
	if sb.Shots[0].Frame != "board/s1.png" || !fileExists("board/s1.png") {
		t.Errorf("frame: %+v", sb.Shots[0])
	}
	if b, _ := os.ReadFile("board/s1.svg"); string(b) != testSketch {
		t.Error("the agent's SVG was changed")
	}
	if left, _ := filepath.Glob("board/.cav-sketch*"); len(left) != 0 {
		t.Errorf("the staging folder was not removed: %v", left)
	}
	if need, _ := got["needSketch"].([]any); len(need) != 1 || need[0] != "s3" {
		t.Errorf("needSketch %v", got["needSketch"])
	}
	m, err := library.Load(library.Root())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range m.Assets {
		if e.Path == "board/s1.png" {
			found = true
			if e.Source != "sketch" || e.Title != "storyboard s1 (sketch)" || e.Generated == nil || e.Generated.Provider != "sketch" ||
				e.Generated.Model != "Cavalry 2.8.0" || len(e.Generated.Refs) != 1 || e.Generated.Refs[0] != "board/s1.svg" || e.Generated.Prompt != "" {
				t.Errorf("manifest entry: %+v %+v", e, e.Generated)
			}
		}
	}
	if !found {
		t.Errorf("no manifest entry for board/s1.png: %+v", m.Assets)
	}

	// Nothing changed: nothing is drawn and Cavalry is not asked.
	f.mu.Lock()
	f.jobs = nil
	f.mu.Unlock()
	if _, _, err := framesJSON(t, path); err != nil {
		t.Fatal(err)
	}
	if jobs := f.kinds(); len(jobs) != 0 {
		t.Errorf("an unchanged run asked Cavalry: %v", jobs)
	}

	// Editing one SVG draws only that shot again.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes("board/s1.svg", later, later); err != nil {
		t.Fatal(err)
	}
	if _, _, err := framesJSON(t, path); err != nil {
		t.Fatal(err)
	}
	want = []string{"state", "scratch-scene", "render", "reopen", "reopened-check"}
	if jobs := f.kinds(); strings.Join(jobs, ",") != strings.Join(want, ",") {
		t.Errorf("after an edit: jobs %v, want %v", jobs, want)
	}
}

func TestSketchReplacesGreyboxCardWhenSVGAppears(t *testing.T) {
	_, path := sketchProject(t)
	f := newFakeSketchCavalry(t, savedScene())
	if _, _, err := framesJSON(t, path); err != nil { // cards for all three
		t.Fatal(err)
	}
	if jobs := f.kinds(); len(jobs) != 0 {
		t.Fatalf("Cavalry was asked: %v", jobs)
	}
	writeSVG(t, "s2", testSketch)
	if _, _, err := framesJSON(t, path); err != nil {
		t.Fatal(err)
	}
	if s := sourcesOf(loadBoard(t, path)); s != "s1=greybox s2=sketch s3=greybox" {
		t.Errorf("sources: %s", s)
	}
}

func TestSketchRefusesUnsavedWork(t *testing.T) {
	_, path := sketchProject(t)
	state := savedScene()
	state["unsaved"] = true
	f := newFakeSketchCavalry(t, state)
	writeSVG(t, "s1", testSketch)
	_, _, err := framesJSON(t, "--only", "s1", path)
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "unsaved changes in /work/intro.cv") || !strings.Contains(ce.hint, "cav scene save") ||
		strings.Contains(ce.hint, "--force") {
		t.Fatalf("got %v", err)
	}
	if jobs := f.kinds(); strings.Join(jobs, ",") != "state" {
		t.Errorf("jobs %v: only the read-only state job may run", jobs)
	}
	if loadBoard(t, path).Shots[0].Frame != "" {
		t.Error("a refused run made a frame")
	}
}

func TestSketchRefusesAnUntitledSceneWithLayers(t *testing.T) {
	_, path := sketchProject(t)
	state := savedScene()
	state["path"], state["layers"], state["unsaved"] = "", 2, false
	f := newFakeSketchCavalry(t, state)
	writeSVG(t, "s1", testSketch)
	_, _, err := framesJSON(t, "--only", "s1", path)
	if err == nil || !strings.Contains(err.Error(), "unsaved changes in the untitled scene") {
		t.Fatalf("got %v", err)
	}
	if jobs := f.kinds(); len(jobs) != 1 {
		t.Errorf("jobs %v", jobs)
	}
}

func TestSketchFailedRenderStillReopensTheScene(t *testing.T) {
	_, path := sketchProject(t)
	f := newFakeSketchCavalry(t, savedScene())
	f.failure = "Cavalry imported nothing"
	writeSVG(t, "s1", testSketch)
	_, _, err := framesJSON(t, "--only", "s1", path)
	if err == nil || !strings.Contains(err.Error(), "s1: ") || !strings.Contains(err.Error(), "Cavalry imported nothing") {
		t.Fatalf("got %v", err)
	}
	want := []string{"state", "scratch-scene", "render", "reopen", "reopened-check"}
	if jobs := f.kinds(); strings.Join(jobs, ",") != strings.Join(want, ",") {
		t.Errorf("jobs %v, want %v", jobs, want)
	}
	if loadBoard(t, path).Shots[0].Frame != "" {
		t.Error("a failed render left a frame")
	}
}

func TestSketchInAnUntitledEmptySceneUsesATemporaryComp(t *testing.T) {
	_, path := sketchProject(t)
	state := savedScene()
	state["path"], state["layers"] = "", 0
	state["unsaved"] = true // Cavalry flags a new scene as changed; it holds no work
	f := newFakeSketchCavalry(t, state)
	writeSVG(t, "s1", testSketch)
	if _, _, err := framesJSON(t, "--only", "s1", path); err != nil {
		t.Fatal(err)
	}
	want := []string{"state", "temp-comp", "render", "temp-comp-restore"}
	if jobs := f.kinds(); strings.Join(jobs, ",") != strings.Join(want, ",") {
		t.Fatalf("jobs %v, want %v", jobs, want)
	}
	if code := f.codes["temp-comp-restore"]; !strings.Contains(code, `"comp#1"`) || !strings.Contains(code, `"comp#8"`) || !strings.Contains(code, "frame = 30") {
		t.Errorf("restore: %s", code)
	}
}

func TestSketchWhenCavalryIsNotOpen(t *testing.T) {
	_, path := sketchProject(t)
	closedBridge(t)
	writeSVG(t, "s1", testSketch)
	_, errOut, err := framesJSON(t, "--only", "s1", path)
	if err != nil {
		t.Fatalf("the default order falls back to a card: %v", err)
	}
	_ = errOut
	if s := sourcesOf(loadBoard(t, path)); !strings.HasPrefix(s, "s1=greybox") {
		t.Errorf("sources: %s", s)
	}
	// An explicit --service sketch has no fallback.
	if _, _, err := framesJSON(t, "--service", "sketch", "--only", "s1", "--force", path); err == nil {
		t.Error("--service sketch drew without Cavalry")
	}
	if s := sourcesOf(loadBoard(t, path)); !strings.HasPrefix(s, "s1=greybox") {
		t.Errorf("a failed run changed the frame: %s", s)
	}
}

func TestSketchNoteWhenTheOrderHasNoSketch(t *testing.T) {
	_, path := sketchProject(t)
	cfg := `{"order":{"image":["greybox","openai"]}}`
	if err := os.WriteFile(services.Path(), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	writeSVG(t, "s1", testSketch)
	got, _, err := framesJSON(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := got["sketchNote"].(string); !strings.Contains(note, "s1.svg in board exist but the image order has no sketch: cav config order image sketch,greybox,openai") {
		t.Errorf("note: %v", got["sketchNote"])
	}
}

func TestFrameCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "storyboard.json")
	if err := (&board.Board{BPM: 120, Seconds: 2, Shots: []board.Shot{{ID: "s1", Beats: [2]float64{0, 4}}}}).Save(path); err != nil {
		t.Fatal(err)
	}
	sb := loadBoard(t, path)
	pngPath, svgPath := filepath.Join(dir, "board", "s1.png"), filepath.Join(dir, "board", "s1.svg")
	if err := os.MkdirAll(filepath.Dir(pngPath), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	set := func(p string, age time.Duration) {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(age), now.Add(age)); err != nil {
			t.Fatal(err)
		}
	}
	shot := func(source string) board.Shot {
		return board.Shot{ID: "s1", Frame: "board/s1.png", Source: source}
	}
	if frameCurrent(sb, shot("sketch"), svgPath, true) {
		t.Error("a frame with no file is current")
	}
	set(pngPath, -time.Minute)
	if !frameCurrent(sb, shot("gemini"), svgPath, true) || !frameCurrent(sb, shot("greybox"), svgPath, true) {
		t.Error("without an SVG every existing frame stands")
	}
	set(svgPath, 0) // newer than the frame
	for _, c := range []struct {
		source    string
		sketching bool
		want      bool
	}{
		{"greybox", true, false}, {"sketch", true, false}, {"gemini", true, true}, {"file", true, true}, {"", true, true},
		{"greybox", false, true}, {"sketch", false, true},
	} {
		if got := frameCurrent(sb, shot(c.source), svgPath, c.sketching); got != c.want {
			t.Errorf("source %q, sketching %v: got %v, want %v", c.source, c.sketching, got, c.want)
		}
	}
	set(svgPath, -2*time.Minute) // older than the frame
	if !frameCurrent(sb, shot("sketch"), svgPath, true) {
		t.Error("a sketch newer than its SVG is not current")
	}
	if frameCurrent(sb, shot("greybox"), svgPath, true) {
		t.Error("a greybox card stands when an SVG exists")
	}
}

// A folder that was already in the board folder, even one with the old fixed name, is not cav's to delete.
func TestSketchRunLeavesAnExistingStagingFolderAlone(t *testing.T) {
	sketchProject(t)
	newFakeSketchCavalry(t, savedScene())
	writeSVG(t, "s1", testSketch)
	if err := os.MkdirAll("board/.cav-sketch", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("board/.cav-sketch/keep.txt", []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("board/.cav-sketch/s1.png", []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := framesJSON(t, "--only", "s1", "storyboard.json"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"board/.cav-sketch/keep.txt", "board/.cav-sketch/s1.png"} {
		if b, err := os.ReadFile(f); err != nil || string(b) != "mine" {
			t.Errorf("%s was changed or removed: %q %v", f, b, err)
		}
	}
	if left, _ := filepath.Glob("board/.cav-sketch-*"); len(left) != 0 {
		t.Errorf("cav's own staging folder was left behind: %v", left)
	}
}

// The scene may change between cav reading it and the setup job. The setup job checks again and
// refuses before it touches the scene; cav then reports the same unsaved-changes refusal and does
// not try to reopen anything.
func TestSketchSetupRefusesWhenTheSceneChangedAfterTheRead(t *testing.T) {
	sketchProject(t)
	f := newFakeSketchCavalry(t, savedScene())
	f.refuseScene = true
	writeSVG(t, "s1", testSketch)
	_, _, err := framesJSON(t, "--only", "s1", "storyboard.json")
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "unsaved changes in /work/intro.cv") || !strings.Contains(ce.hint, "cav scene save") {
		t.Fatalf("got %v", err)
	}
	if got, want := strings.Join(f.kinds(), ","), "state,scratch-scene"; got != want {
		t.Errorf("jobs %s, want %s: a refused setup must not be followed by a reopen", got, want)
	}
	if left, _ := filepath.Glob("board/.cav-sketch*"); len(left) != 0 {
		t.Errorf("staging folder left behind: %v", left)
	}
	code := f.codes["scratch-scene"]
	if code != "" && strings.Index(code, "sceneHasUnsavedChanges") > strings.Index(code, "api.newScene()") {
		t.Error("the check comes after newScene")
	}
}
