package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	register(command{
		name:    "scene",
		args:    "new|open|save|info [options]",
		summary: "Create, open, save or describe the scene and its active comp.",
		run:     cmdScene,
	})
	longHelp["scene"] = `
cav scene info
    Scene path, unsaved changes, active comp settings and layer count.
cav scene new [--width 1920 --height 1080 --fps 60 --seconds 10 | --frames 600] [--bg #101014] [--force]
    Start an empty scene and set up its comp. It refuses when the open scene has
    unsaved changes, so it never throws away work. --force skips that check.
cav scene comp [--width W --height H --fps F --seconds S | --frames N] [--bg #hex] [--motion-blur]
    Change the active comp's settings without touching its layers.
cav scene open <file.cv> [--force]
cav scene save [<file.cv>]
    Without a path, saves in place (the scene must already have a path).`
	register(command{
		name:    "tree",
		args:    "[--depth N] [--frame F]",
		summary: "Print the layer tree of the active comp (ids, types, names, in/out).",
		run:     cmdTree,
	})
	register(command{
		name:    "layer",
		args:    "<layerId>... [--attrs] [--frame F]",
		summary: "Describe layers: parent, children, bounding box, animated attributes and keys.",
		run:     cmdLayer,
	})
}

func cmdScene(a *app, args []string) error {
	if len(args) == 0 {
		return usageErr("usage: cav scene new|open|save|info|comp")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "info":
		return sceneInfo(a)
	case "new", "comp":
		return sceneNew(a, sub, rest)
	case "open":
		fs := flag.NewFlagSet("scene open", flag.ContinueOnError)
		force := fs.Bool("force", false, "open even if the current scene has unsaved changes")
		pos, err := parseFlags(fs, rest)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return usageErr("usage: cav scene open <file.cv>")
		}
		p, _ := filepath.Abs(pos[0])
		if err := guardUnsaved(a, *force); err != nil {
			return err
		}
		if err := a.jsCall(fmt.Sprintf("api.openScene(%s, true); return true", jsString(p)), 5*time.Minute, nil); err != nil {
			return err
		}
		return sceneInfo(a)
	case "save":
		code := `if (!api.getSceneFilePath()) throw new Error('the scene has never been saved: pass a path, e.g. cav scene save scenes/intro.cv'); return api.saveScene()`
		var p string
		if len(rest) > 0 {
			p, _ = filepath.Abs(rest[0])
			if !strings.HasSuffix(p, ".cv") {
				p += ".cv"
			}
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			code = fmt.Sprintf("return api.saveSceneAs(%s)", jsString(p))
		}
		var ok bool
		if err := a.jsCall(code, 5*time.Minute, &ok); err != nil {
			return err
		}
		if !ok {
			return fail(exitError, "Cavalry did not save the scene", "check that the folder exists and is writable")
		}
		return sceneInfo(a)
	}
	return usageErr("unknown scene command %q", sub)
}

func guardUnsaved(a *app, force bool) error {
	if force {
		return nil
	}
	var st struct {
		Unsaved bool   `json:"unsaved"`
		Path    string `json:"path"`
		Layers  int    `json:"layers"`
	}
	if err := a.jsCall(`return {unsaved: api.sceneHasUnsavedChanges(), path: api.getSceneFilePath(), layers: api.getAllSceneLayers().filter(function (id) { return api.getLayerType(id) !== 'compNode' }).length}`, time.Minute, &st); err != nil {
		return err
	}
	// An untitled scene with no layers holds no work, even if Cavalry flags it as changed.
	if st.Unsaved && (st.Path != "" || st.Layers > 0) {
		name := st.Path
		if name == "" {
			name = "the untitled scene"
		}
		return fail(exitError, "unsaved changes in "+name,
			"save them first (cav scene save <file.cv>), or pass --force to discard them")
	}
	return nil
}

const sceneInfoJS = `
var comp = api.getActiveComp();
var res = api.get(comp, 'resolution');
var bg = api.get(comp, 'backgroundColor');
function hex(c) { function h(n) { var s = Math.round(n).toString(16); return s.length < 2 ? '0' + s : s } return '#' + h(c.r) + h(c.g) + h(c.b) }
return {
  scenePath: api.getSceneFilePath(),
  unsaved: api.sceneHasUnsavedChanges(),
  cavalryVersion: api.getCavalryVersion(),
  comp: {
    id: comp, name: api.getNiceName(comp),
    width: res.x, height: res.y, fps: api.get(comp, 'fps'),
    startFrame: api.get(comp, 'startFrame'), endFrame: api.get(comp, 'endFrame'),
    background: hex(bg), motionBlur: api.get(comp, 'motionBlur'),
    layers: api.getCompLayers(false).length, topLevelLayers: api.getCompLayers(true).length
  },
  comps: api.getComps().map(function (c) { return {id: c, name: api.getNiceName(c)} }),
  frame: api.getFrame()
};`

type sceneState struct {
	ScenePath      string `json:"scenePath"`
	Unsaved        bool   `json:"unsaved"`
	CavalryVersion string `json:"cavalryVersion"`
	Comp           struct {
		ID             string  `json:"id"`
		Name           string  `json:"name"`
		Width          int     `json:"width"`
		Height         int     `json:"height"`
		FPS            float64 `json:"fps"`
		StartFrame     int     `json:"startFrame"`
		EndFrame       int     `json:"endFrame"`
		Background     string  `json:"background"`
		MotionBlur     bool    `json:"motionBlur"`
		Layers         int     `json:"layers"`
		TopLevelLayers int     `json:"topLevelLayers"`
	} `json:"comp"`
	Comps []map[string]string `json:"comps"`
	Frame int                 `json:"frame"`
}

func getScene(a *app) (*sceneState, error) {
	var st sceneState
	if err := a.jsCall(sceneInfoJS, time.Minute, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func sceneInfo(a *app) error {
	st, err := getScene(a)
	if err != nil {
		return err
	}
	a.emit(map[string]any{"scene": st}, func() {
		p := st.ScenePath
		if p == "" {
			p = "(untitled)"
		}
		if st.Unsaved {
			p += "  [unsaved changes]"
		}
		c := st.Comp
		frames := c.EndFrame - c.StartFrame + 1
		fmt.Printf("scene   %s\ncomp    %s (%s)  %dx%d  %g fps  frames %d-%d (%d frames, %.2f s)\n",
			p, c.Name, c.ID, c.Width, c.Height, c.FPS, c.StartFrame, c.EndFrame, frames, float64(frames)/c.FPS)
		fmt.Printf("        background %s  motion blur %v  layers %d (%d top level)\n", c.Background, c.MotionBlur, c.Layers, c.TopLevelLayers)
		fmt.Printf("cavalry %s  playhead %d\n", st.CavalryVersion, st.Frame)
	})
	return nil
}

func sceneNew(a *app, sub string, args []string) error {
	fs := flag.NewFlagSet("scene "+sub, flag.ContinueOnError)
	w := fs.Int("width", 0, "comp width")
	h := fs.Int("height", 0, "comp height")
	fps := fs.Float64("fps", 0, "frames per second")
	secs := fs.Float64("seconds", 0, "duration in seconds")
	frames := fs.Int("frames", 0, "duration in frames")
	bg := fs.String("bg", "", "background colour, #rrggbb")
	mb := fs.Bool("motion-blur", false, "turn on comp motion blur")
	force := fs.Bool("force", false, "discard unsaved changes")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if sub == "new" {
		if *w == 0 {
			*w = 1920
		}
		if *h == 0 {
			*h = 1080
		}
		if *fps == 0 {
			*fps = 60
		}
		if *secs == 0 && *frames == 0 {
			*secs = 10
		}
		if *bg == "" {
			*bg = "#000000"
		}
		if err := guardUnsaved(a, *force); err != nil {
			return err
		}
	}
	var js strings.Builder
	if sub == "new" {
		js.WriteString("api.newScene();\n")
	}
	js.WriteString("var comp = api.getActiveComp(); var d = {};\n")
	if *w > 0 && *h > 0 {
		fmt.Fprintf(&js, "d.resolution = [%d, %d];\n", *w, *h)
	}
	if *fps > 0 {
		fmt.Fprintf(&js, "d.fps = %g;\n", *fps)
	}
	if *bg != "" {
		fmt.Fprintf(&js, "d.backgroundColor = %s;\n", jsString(*bg))
	}
	if *mb {
		js.WriteString("d.motionBlur = true; d.motionBlurSamples = 8; d.shutterAngle = 180;\n")
	}
	js.WriteString("api.set(comp, d);\n")
	if *frames > 0 || *secs > 0 {
		fmt.Fprintf(&js, "var fps = api.get(comp, 'fps'); var n = %d > 0 ? %d : Math.round(%g * fps);\n", *frames, *frames, *secs)
		js.WriteString("api.set(comp, {startFrame: 0, endFrame: n - 1, playbackStart: 0, playbackEnd: n - 1});\n")
	}
	js.WriteString("api.setFrame(0); return true;")
	if err := a.jsCall(js.String(), time.Minute, nil); err != nil {
		return err
	}
	return sceneInfo(a)
}

const treeJS = `
var maxDepth = %d, frame = %d;
if (frame >= 0) api.setFrame(frame);
function node(id, depth) {
  var n = { id: id, name: api.getNiceName(id), type: api.getLayerType(id) };
  try { var i = api.getInFrame(id), o = api.getOutFrame(id); n.in = i; n.out = o } catch (e) {}
  try { if (!api.isVisible(id)) n.hidden = true } catch (e) {}
  try { var op = api.get(id, 'opacity'); if (op !== 100) n.opacity = Math.round(op * 10) / 10 } catch (e) {}
  try { var anim = api.getAnimatedAttributes(id); if (anim.length) n.animated = anim.length } catch (e) {}
  var kids = [];
  try { kids = api.getChildren(id) } catch (e) {}
  if (kids.length) {
    if (depth < maxDepth) n.children = kids.map(function (k) { return node(k, depth + 1) });
    else n.childCount = kids.length;
  }
  return n;
}
return api.getCompLayers(true).map(function (id) { return node(id, 1) });`

type treeNode struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	In         *int       `json:"in,omitempty"`
	Out        *int       `json:"out,omitempty"`
	Hidden     bool       `json:"hidden,omitempty"`
	Opacity    *float64   `json:"opacity,omitempty"`
	Animated   int        `json:"animated,omitempty"`
	Children   []treeNode `json:"children,omitempty"`
	ChildCount int        `json:"childCount,omitempty"`
}

func cmdTree(a *app, args []string) error {
	fs := flag.NewFlagSet("tree", flag.ContinueOnError)
	depth := fs.Int("depth", 6, "maximum depth")
	frame := fs.Int("frame", -1, "move the playhead here first")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	var nodes []treeNode
	if err := a.jsCall(fmt.Sprintf(treeJS, *depth, *frame), 2*time.Minute, &nodes); err != nil {
		return err
	}
	count := 0
	var walk func([]treeNode)
	walk = func(ns []treeNode) {
		for _, n := range ns {
			count++
			walk(n.Children)
		}
	}
	walk(nodes)
	a.emit(map[string]any{"layers": nodes, "count": count}, func() {
		var pr func([]treeNode, string)
		pr = func(ns []treeNode, indent string) {
			for _, n := range ns {
				extra := ""
				if n.In != nil && n.Out != nil {
					extra += fmt.Sprintf(" [%d-%d]", *n.In, *n.Out)
				}
				if n.Hidden {
					extra += " hidden"
				}
				if n.Opacity != nil {
					extra += fmt.Sprintf(" opacity %g", *n.Opacity)
				}
				if n.Animated > 0 {
					extra += fmt.Sprintf(" %d animated", n.Animated)
				}
				if n.ChildCount > 0 {
					extra += fmt.Sprintf(" (+%d children)", n.ChildCount)
				}
				fmt.Printf("%s%s  %q  %s%s\n", indent, n.ID, n.Name, n.Type, extra)
				pr(n.Children, indent+"  ")
			}
		}
		if len(nodes) == 0 {
			fmt.Println("(the active comp has no layers)")
		}
		pr(nodes, "")
	})
	return nil
}

const layerJS = `
var ids = %s, withAttrs = %v, frame = %d;
if (frame >= 0) api.setFrame(frame);
function val(v) { if (v && typeof v === 'object') { try { return JSON.parse(JSON.stringify(v)) } catch (e) { return String(v) } } return v }
return ids.map(function (id) {
  if (!api.layerExists(id)) return { id: id, error: 'no such layer' };
  var n = { id: id, name: api.getNiceName(id), type: api.getLayerType(id), parent: api.getParent(id) || null, children: api.getChildren(id) };
  try { n.in = api.getInFrame(id); n.out = api.getOutFrame(id) } catch (e) {}
  try { var b = api.getBoundingBox(id, true); n.bbox = { left: b.left, right: b.right, top: b.top, bottom: b.bottom, width: b.width, height: b.height, centre: b.centre } } catch (e) {}
  n.transform = {};
  ['position', 'rotation', 'scale', 'pivot', 'opacity'].forEach(function (a) { try { n.transform[a] = val(api.get(id, a)) } catch (e) {} });
  n.animated = {};
  var anim = [];
  try { anim = api.getAnimatedAttributes(id) } catch (e) {}
  anim.forEach(function (a) {
    var times = [];
    try { times = api.getKeyframeTimes(id, a) } catch (e) {}
    var keys = times.map(function (t) {
      var k = { frame: t };
      try { var cur = api.getFrame(); api.setFrame(t); k.value = val(api.get(id, a)); api.setFrame(cur) } catch (e) {}
      return k;
    });
    n.animated[a] = keys;
  });
  try { var inputs = api.getInConnectedAttributes(id); if (inputs && inputs.length) n.connectedInputs = inputs } catch (e) {}
  if (withAttrs) {
    n.attrs = {};
    api.getAttributes(id).forEach(function (a) { try { n.attrs[a] = val(api.get(id, a)) } catch (e) {} });
  }
  return n;
});`

func cmdLayer(a *app, args []string) error {
	fs := flag.NewFlagSet("layer", flag.ContinueOnError)
	attrs := fs.Bool("attrs", false, "list every attribute and its current value")
	frame := fs.Int("frame", -1, "move the playhead here first")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageErr("usage: cav layer <layerId>... (get ids from `cav tree`)")
	}
	ids := "["
	for i, p := range pos {
		if i > 0 {
			ids += ","
		}
		ids += jsString(p)
	}
	ids += "]"
	var layers []map[string]any
	if err := a.jsCall(fmt.Sprintf(layerJS, ids, *attrs, *frame), 2*time.Minute, &layers); err != nil {
		return err
	}
	a.emit(map[string]any{"layers": layers}, func() {
		for _, l := range layers {
			fmt.Println(prettyAny(l))
		}
	})
	return nil
}

func init() {
	register(command{
		name:    "status",
		args:    "",
		summary: "Show the bridge, the open scene and the active comp.",
		run: func(a *app, args []string) error {
			if c := bridgeCheck(a); !c.OK {
				return fail(exitUnavailable, "bridge: "+c.Detail, c.Fix)
			}
			return sceneInfo(a)
		},
	})
}
