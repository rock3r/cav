package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Every native call owns its switch and restoration, even when the CLI times out.
func selectedCompJS(selector, code string) string {
	return `var selector=` + jsString(selector) + `;
var comps=api.getComps(), matches=comps.filter(function(c){return c===selector;});
if(!matches.length)matches=comps.filter(function(c){return api.getNiceName(c)===selector;});
if(matches.length!==1)throw new Error('composition must resolve to one ID: '+selector);
var originalComp=api.getActiveComp(), originalFrame=api.getFrame(), targetFrame;
try {
 api.setActiveComp(matches[0]);targetFrame=api.getFrame();
 return (function(){` + code + `})();
} finally {
 try { if(targetFrame!==undefined)api.setFrame(targetFrame); }
 finally { api.setActiveComp(originalComp);api.setFrame(originalFrame); }
}`
}

func sortedFrames(frames []int) []int {
	out := append([]int{}, frames...)
	sort.Ints(out)
	kept := make([]int, 0, len(out))
	for _, f := range out {
		if len(kept) == 0 || f != kept[len(kept)-1] {
			kept = append(kept, f)
		}
	}
	return kept
}

func init() {
	longHelp["frames"] = `Evaluate every frame from comp start through the last retained frame.
Keep every Nth requested frame with --keep-every (default 1), up to 1000 retained PNGs.
--max-evaluated bounds total evaluations (default 10000; maximum 100000).
Requests are sorted/deduplicated. Unretained frames use one reusable warm-up PNG.
--comp accepts an ID or unique name. Composition and playhead restore in native finally.
The command is an operation: after timeout, resume its ID rather than retrying.`
	register(command{name: "frames", args: "<start-end> [--keep-every N] [--comp ID|name] [--max-evaluated 10000] [-o dir]",
		summary: "Preview simulations chronologically, retaining only selected PNG frames.", run: func(a *app, args []string) error {
			return cmdFrame(a, append(append([]string{}, args...), "--chronological=true"))
		}})
}

func (a *app) renderChronologicalFrames(frames []int, scale int, dir string, st *sceneState) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	sample, err := json.Marshal(map[string]any{
		"frames": frames, "start": st.Comp.StartFrame, "dir": filepath.ToSlash(dir),
		"scale": scale, "comp": st.Comp.ID, "scenePath": st.ScenePath, "requireSaved": a.requireSaved,
	})
	if err != nil {
		return nil, err
	}
	code := "var sample=" + string(sample) + ";\n" + chronologicalFramesJS
	var paths []string
	if err := a.jsCall(code, 5*time.Minute, &paths); err != nil {
		return nil, err
	}
	if len(paths) != len(frames) {
		return nil, fmt.Errorf("preview returned %d/%d images", len(paths), len(frames))
	}
	for i, p := range paths {
		expected := filepath.Join(dir, fmt.Sprintf("f_%d.png", frames[i]))
		if filepath.FromSlash(p) != expected {
			return nil, fmt.Errorf("unexpected preview path")
		}
		if info, err := os.Stat(expected); err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("missing preview PNG")
		}
		paths[i] = expected
	}
	return paths, nil
}

const chronologicalFramesJS = `
if(api.getActiveComp()!==sample.comp||api.getSceneFilePath()!==sample.scenePath)throw new Error('preview scene/comp changed');
if(sample.requireSaved&&api.sceneHasUnsavedChanges())throw new Error('chunk scene has unsaved changes');
var cur=api.getFrame(),out=[],cursor=sample.start;
try{
 sample.frames.forEach(function(f){
  while(cursor<f){api.setFrame(cursor++);api.renderPNGFrame(sample.dir+'/_warmup',sample.scale);}
  api.setFrame(f);var p=sample.dir+'/f_'+f;api.renderPNGFrame(p,sample.scale);
  if(!api.filePathExists(p+'.png'))throw new Error('missing preview frame '+f);
  out.push(p+'.png');cursor=f+1;
 });
}finally{api.setFrame(cur)}
return out;`
