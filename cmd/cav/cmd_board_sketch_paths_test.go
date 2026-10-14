package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const guardedFrame = "if (api.getFrame() !== frame) api.setFrame(frame)"
const guardedComp = "prev !== api.getActiveComp() &&"

// Cavalry resolves a relative path from its own folder, not the project's, so every path in
// the render job must be absolute even when the storyboard path given to cav is relative.
func TestSketchJobsGetAbsolutePathsAndRestoreOnlyWhatDiffers(t *testing.T) {
	sketchProject(t)
	f := newFakeSketchCavalry(t, savedScene())
	writeSVG(t, "s1", testSketch)
	if _, _, err := framesJSON(t, "--only", "s1", "storyboard.json"); err != nil {
		t.Fatal(err)
	}
	line := strings.SplitN(strings.TrimPrefix(f.codes["render"], "var sample = "), ";\n", 2)[0]
	var s struct{ SVG, Out string }
	if err := json.Unmarshal([]byte(line), &s); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{s.SVG, s.Out} {
		if !filepath.IsAbs(filepath.FromSlash(p)) || strings.Contains(p, "\\") {
			t.Errorf("path %q passed to Cavalry is not an absolute, slash-separated path", p)
		}
	}
	// The restore sets the comp or the playhead only when it differs, so a scene that is
	// already as it was is not touched.
	c := f.codes["reopened-check"]
	if !strings.Contains(c, guardedFrame) || !strings.Contains(c, guardedComp) {
		t.Errorf("the restore job sets the comp or frame without checking:\n%s", c)
	}
}

func TestSketchTempCompRestoreOnlyChangesWhatDiffers(t *testing.T) {
	sketchProject(t)
	state := savedScene()
	state["path"], state["layers"] = "", 0
	f := newFakeSketchCavalry(t, state)
	writeSVG(t, "s1", testSketch)
	if _, _, err := framesJSON(t, "--only", "s1", "storyboard.json"); err != nil {
		t.Fatal(err)
	}
	c := f.codes["temp-comp-restore"]
	if !strings.Contains(c, guardedFrame) || !strings.Contains(c, guardedComp) {
		t.Errorf("restore:\n%s", c)
	}
}
