package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/imagegen"
)

// sketchStagePrefix starts the name of the folder, inside the board folder, where cav keeps the
// sketch copies it imports into Cavalry and the PNGs Cavalry renders. Each run makes its own
// folder with a unique name and removes only that one, so a folder that was already there stays.
const sketchStagePrefix = ".cav-sketch-"

// sketchUnavailable means cav could not reach Cavalry at all, so nothing was changed.
type sketchUnavailable struct{ err error }

func (e *sketchUnavailable) Error() string { return e.err.Error() }
func (e *sketchUnavailable) Unwrap() error { return e.err }

type sketchMode int

const (
	// sketchScene: the open scene is saved and clean. cav draws in a scratch scene and
	// opens the saved scene again afterwards.
	sketchScene sketchMode = iota
	// sketchComp: the open scene is untitled and empty. cav draws in a temporary
	// composition and deletes it afterwards.
	sketchComp
)

// sketchSession is one run of drawing sketches in Cavalry. It never touches work that is not
// saved: startSketch refuses when the open scene has any.
type sketchSession struct {
	a       *app
	mode    sketchMode
	stage   string // absolute folder for the normalized SVGs and staged PNGs
	w, h    int
	bg      string
	version string

	prevPath    string // the scene to open again (sketchScene)
	prevComp    string
	prevFrame   int
	compsBefore []string // the comps that existed before the temporary one
	comp        string   // the comp the sketches are drawn in
	started     bool     // something in Cavalry was changed, so close must restore it
	closed      bool
}

const (
	sketchStateTimeout  = time.Minute
	sketchSetupTimeout  = time.Minute
	sketchRenderTimeout = 2 * time.Minute
	sketchReopenTimeout = 10 * time.Minute
)

const sketchStateJS = `var w = ` + sceneWorkJS + `;
w.comp = api.getActiveComp(); w.frame = api.getFrame(); w.version = api.getCavalryVersion(); w.comps = api.getComps();
return w;`

// sketchSetup builds the scratch scene or the temporary comp. %s: resolution and colour.
// It first checks again that the open scene is still the saved one it read, because the person
// may have changed it since: if not, it throws sketchRefusedPrefix and touches nothing.
const sketchNewSceneJS = `
var expectedPath = %s;
if (String(api.getSceneFilePath()).replace(/\\/g, '/') !== expectedPath || api.sceneHasUnsavedChanges())
  throw new Error('` + sketchRefusedPrefix + `' + api.getSceneFilePath());
api.newScene();
var comp = api.getActiveComp();
api.set(comp, {resolution: [%d, %d], backgroundColor: %s});
api.setFrame(api.get(comp, 'startFrame'));
return comp;`

// sketchRefusedPrefix starts the error that the scratch scene setup throws when the open scene
// has changed since cav read it.
const sketchRefusedPrefix = "cav-sketch-refused: "

const sketchTempCompJS = `
var prev = api.getActiveComp();
var comp = api.createComp('cav.sketch');
try {
  api.setActiveComp(comp);
  api.set(comp, {resolution: [%d, %d], backgroundColor: %s});
  api.setFrame(api.get(comp, 'startFrame'));
} catch (e) {
  api.setActiveComp(prev);
  api.deleteLayer(comp);
  throw e;
}
return comp;`

const sketchTempCompRestoreJS = `
var prev = %s, frame = %d, tmp = %s, before = %s;
if (prev !== api.getActiveComp() && api.getComps().indexOf(prev) >= 0) api.setActiveComp(prev);
if (api.getFrame() !== frame) api.setFrame(frame);
// With no id for the temporary comp the setup job's outcome is unknown: delete only a comp that
// was not there before this run and is named cav.sketch.
var doomed = tmp ? [tmp] : api.getComps().filter(function (c) { return before.indexOf(c) < 0 && api.getNiceName(c) === 'cav.sketch' });
doomed.forEach(function (c) {
  try { api.deleteLayer(c) } catch (e) { if (api.getComps().indexOf(c) >= 0) throw e }
});
return true;`

// sketchReopenedJS puts the user's composition and playhead back after their scene is open again.
const sketchReopenedJS = `
var prev = %s, frame = %d;
if (prev !== api.getActiveComp() && api.getComps().indexOf(prev) >= 0) api.setActiveComp(prev);
if (api.getFrame() !== frame) api.setFrame(frame);
return {unsaved: api.sceneHasUnsavedChanges(), path: api.getSceneFilePath()};`

// startSketch reads the open scene and sets up a place to draw. It refuses when the scene has
// unsaved work. A *sketchUnavailable error means Cavalry did not answer and nothing changed.
func startSketch(a *app, sb *board.Board, boardDir string) (*sketchSession, error) {
	o, err := a.execJS(sketchStateJS, execOpts{timeout: sketchStateTimeout, source: "cav board frames"})
	if err != nil {
		if errors.Is(err, bridge.ErrUnavailable) {
			return nil, &sketchUnavailable{err}
		}
		return nil, err
	}
	if !o.result.OK {
		return nil, scriptErr(o)
	}
	var st struct {
		sceneWork
		Comp    string   `json:"comp"`
		Frame   int      `json:"frame"`
		Version string   `json:"version"`
		Comps   []string `json:"comps"`
	}
	if err := json.Unmarshal(o.result.Value, &st); err != nil {
		return nil, err
	}
	// Work that is not saved, or an untitled scene with layers that cav could not open again.
	if st.hasUnsaved() || (st.Path == "" && st.Layers > 0) {
		return nil, fail(exitError, "unsaved changes in "+st.name(),
			"cav renders sketches in a scratch scene and reopens yours afterwards: save it (cav scene save), then run again")
	}
	// Cavalry resolves a relative path from its own folder, so every path it gets is absolute.
	boardAbs, err := filepath.Abs(boardDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(boardAbs, 0o755); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(boardAbs, sketchStagePrefix)
	if err != nil {
		return nil, err
	}
	s := &sketchSession{a: a, w: sb.Width, h: sb.Height, bg: board.SketchBackground(sb.Style.Palette), version: st.Version,
		stage: stage, prevPath: st.Path, prevComp: st.Comp, prevFrame: st.Frame, compsBefore: st.Comps}
	if s.compsBefore == nil {
		s.compsBefore = []string{}
	}
	js := sketchTempCompJS
	s.mode = sketchComp
	if st.Path != "" {
		js, s.mode = sketchNewSceneJS, sketchScene
	}
	code := fmt.Sprintf(js, s.w, s.h, jsString(s.bg))
	if s.mode == sketchScene {
		code = fmt.Sprintf(js, jsString(filepath.ToSlash(st.Path)), s.w, s.h, jsString(s.bg))
	}
	// From here Cavalry may change, so a failure after this point still needs close.
	s.started = true
	o, execErr := a.execJS(code, execOpts{timeout: sketchSetupTimeout, source: "cav board frames"})
	if execErr != nil {
		return s, execErr
	}
	if !o.result.OK {
		if o.result.Error != nil && strings.HasPrefix(o.result.Error.Message, sketchRefusedPrefix) {
			// The scene was not touched: nothing to restore.
			s.started = false
			_ = os.RemoveAll(s.stage)
			return nil, fail(exitError, "unsaved changes in "+st.name(),
				"cav renders sketches in a scratch scene and reopens yours afterwards: save it (cav scene save), then run again")
		}
		return s, scriptErr(o)
	}
	if err := json.Unmarshal(o.result.Value, &s.comp); err != nil {
		return s, err
	}
	return s, nil
}

// render draws the sketch at svgAbs in the scratch comp and returns the PNG.
func (s *sketchSession) render(id, svgAbs string) (*imagegen.Image, error) {
	raw, err := board.ReadSketch(svgAbs)
	if err != nil {
		return nil, err
	}
	// Cavalry imports a copy that cav checked and rewrote, never the file itself.
	norm, err := board.NormalizeSketch(raw, s.w, s.h, s.bg)
	if err != nil {
		return nil, err
	}
	in := filepath.Join(s.stage, id+".svg")
	out := filepath.Join(s.stage, id)
	if err := os.WriteFile(in, norm, 0o644); err != nil {
		return nil, err
	}
	_ = os.Remove(out + ".png")
	sample, err := json.Marshal(map[string]any{"svg": filepath.ToSlash(in), "out": filepath.ToSlash(out), "scale": 100, "comp": s.comp})
	if err != nil {
		return nil, err
	}
	var r struct {
		PNG    string `json:"png"`
		Layers int    `json:"layers"`
	}
	if err := s.a.jsCall("var sample = "+string(sample)+";\n"+assets.BoardSketch, sketchRenderTimeout, &r); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(out + ".png")
	if err != nil {
		return nil, fmt.Errorf("Cavalry did not write the sketch image: %w", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("Cavalry wrote an image that is not a PNG: %w", err)
	}
	if cfg.Width != s.w || cfg.Height != s.h {
		return nil, fmt.Errorf("Cavalry rendered %dx%d, not the storyboard's %dx%d", cfg.Width, cfg.Height, s.w, s.h)
	}
	return &imagegen.Image{Data: data, MIME: "image/png", Provider: "sketch", Model: "Cavalry " + s.version}, nil
}

// restoreHint is what the person does when cav could not give Cavalry back.
func (s *sketchSession) restoreHint() string {
	if s.mode == sketchScene {
		return "Cavalry still has cav's scratch scene open; your scene file was not touched: cav scene open " + s.prevPath
	}
	return "Cavalry may still have cav's temporary comp \"cav.sketch\"; delete it and make your own comp active"
}

// close gives Cavalry back as it was. It is safe to call twice.
func (s *sketchSession) close() error {
	if s == nil || s.closed || !s.started {
		return nil
	}
	s.closed = true
	restoreErr := func(err error) error {
		return &cliError{code: exitError, msg: "could not restore Cavalry after drawing sketches: " + err.Error(), hint: s.restoreHint()}
	}
	switch s.mode {
	case sketchScene:
		// The same two jobs as cav scene open: open, then check what opened.
		if err := s.a.jsCall(fmt.Sprintf("api.openScene(%s, true); return true", jsString(s.prevPath)), sketchReopenTimeout, nil); err != nil {
			return restoreErr(err)
		}
		code := "var expectedScenePath = " + jsString(filepath.ToSlash(s.prevPath)) + ";\n" + openedSceneGuardJS +
			fmt.Sprintf(sketchReopenedJS, jsString(s.prevComp), s.prevFrame)
		var back struct {
			Unsaved bool `json:"unsaved"`
		}
		if err := s.a.jsCall(code, sketchReopenTimeout, &back); err != nil {
			return restoreErr(err)
		}
		if back.Unsaved {
			fmt.Fprintln(os.Stderr, "note: Cavalry reports unsaved changes in "+s.prevPath+" after reopening it")
		}
	default:
		if err := s.a.jsCall(fmt.Sprintf(sketchTempCompRestoreJS, jsString(s.prevComp), s.prevFrame, jsString(s.comp), jsJSON(s.compsBefore)), sketchSetupTimeout, nil); err != nil {
			return restoreErr(err)
		}
	}
	// The copies and staged images are only needed while Cavalry draws.
	_ = os.RemoveAll(s.stage)
	return nil
}

func jsJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
