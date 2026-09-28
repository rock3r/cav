package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rock3r/cavalry-skill/internal/bridge"
)

func init() {
	register(command{
		name:    "check",
		args:    "[--min-text 28] [--max-still 1.5]",
		summary: "Find common problems before you render: still stretches, tiny text, off-frame or empty frames.",
		run:     cmdSceneCheck,
	})
	longHelp["check"] = `
cav check looks at the active comp and reports problems that viewers notice:
  still     stretches longer than --max-still seconds where nothing is animated
  text      text smaller than --min-text px (scaled to a 1080 px tall frame)
  offframe  text or large shapes completely outside the frame on a key frame
  clipped   text cut by the frame edge
  blank     blank frames at the start or the end (a long empty tail)
  keys      keyframes after the end of the comp (they never play)
Each finding says what to change. Exit code 0 even with findings; use --json to read them.
Oscillators, noise and other connected behaviours count as motion.`
}

type finding struct {
	Kind   string `json:"kind"`
	Layer  string `json:"layer,omitempty"`
	Name   string `json:"name,omitempty"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

const checkJS = `
var comp = api.getActiveComp();
var start = api.get(comp, 'startFrame'), end = api.get(comp, 'endFrame');
var res = api.get(comp, 'resolution');
var ids = api.getCompLayers(false);
var segs = [], drivers = [], texts = [], late = [], shapes = [];
var driverTypes = { oscillator: 1, noise: 1, stagger: 0, spring: 1, wave: 1, pathfinder: 0, particleShape: 1, forgeDynamicsShape: 1 };
function visibleRange(id) {
  var a = start, b = end, p = id;
  while (p && p !== comp) {
    try { a = Math.max(a, api.getInFrame(p)); b = Math.min(b, api.getOutFrame(p)) } catch (e) {}
    try { if (api.get(p, 'hidden')) return null } catch (e) {}
    p = api.getParent(p);
  }
  return a <= b ? [a, b] : null;
}
ids.forEach(function (id) {
  var type = api.getLayerType(id);
  if (driverTypes[type]) {
    var outs = [];
    try { outs = api.getOutConnectedAttributes(id) || [] } catch (e) {}
    if (outs.length || type === 'particleShape' || type === 'forgeDynamicsShape') drivers.push(type);
  }
  var range = visibleRange(id);
  var anim = [];
  try { anim = api.getAnimatedAttributes(id) } catch (e) {}
  anim.forEach(function (attr) {
    var times = [];
    try { times = api.getKeyframeTimes(id, attr) } catch (e) {}
    times.sort(function (x, y) { return x - y });
    for (var i = 0; i < times.length; i++) {
      if (times[i] > end) late.push({ id: id, name: api.getNiceName(id), attr: attr, frame: times[i] });
    }
    if (!range) return;
    for (var j = 0; j + 1 < times.length; j++) {
      var t0 = times[j], t1 = times[j + 1];
      var v0, v1;
      try { api.setFrame(t0); v0 = JSON.stringify(api.get(id, attr)); api.setFrame(t1); v1 = JSON.stringify(api.get(id, attr)) } catch (e) { continue }
      if (v0 !== v1) segs.push([Math.max(t0, range[0]), Math.min(t1, range[1])]);
    }
  });
  if (!range) return;
  if (type === 'textShape') {
    var mid = Math.round((range[0] + range[1]) / 2);
    api.setFrame(mid);
    var sc = 1, p = id;
    while (p && p !== comp) { try { var s = api.get(p, 'scale'); sc *= Math.abs(s.y) } catch (e) {} p = api.getParent(p) }
    var size = api.get(id, 'fontSize') * sc;
    var t = api.get(id, 'text');
    texts.push({ id: id, name: api.getNiceName(id), size: size, text: String(t && t.text !== undefined ? t.text : t).slice(0, 40), range: range });
  }
  if (type === 'textShape' || type === 'basicShape' || type === 'group') {
    shapes.push({ id: id, type: type, range: range });
  }
});
var off = [], clipped = [];
var W = res.x / 2, H = res.y / 2;
var probe = [end, Math.round((start + end) / 2)];
shapes.forEach(function (s) {
  if (s.type === 'group') return;
  probe.forEach(function (f) {
    if (f < s.range[0] || f > s.range[1]) return;
    api.setFrame(f);
    var b;
    try { b = api.getBoundingBox(s.id, true) } catch (e) { return }
    if (!b || b.width * b.height === 0) return;
    var big = s.type === 'textShape' || b.width * b.height > 0.005 * res.x * res.y;
    var outside = b.right < -W || b.left > W || b.top < -H || b.bottom > H;
    try { if (api.get(s.id, 'opacity') === 0) return } catch (e) {}
    if (big && outside) off.push({ id: s.id, name: api.getNiceName(s.id), frame: f });
    // Text cut by the frame edge (partly outside) while it should be readable.
    var cut = !outside && (b.left < -W - 1 || b.right > W + 1 || b.top > H + 1 || b.bottom < -H - 1);
    if (s.type === 'textShape' && cut) clipped.push({ id: s.id, name: api.getNiceName(s.id), frame: f });
  });
});
api.setFrame(start);
return { start: start, end: end, fps: api.get(comp, 'fps'), height: res.y, segs: segs, drivers: drivers, texts: texts, late: late.slice(0, 20), off: off.slice(0, 20), clipped: clipped.slice(0, 20), layers: ids.length };`

type checkData struct {
	Start   int          `json:"start"`
	End     int          `json:"end"`
	FPS     float64      `json:"fps"`
	Height  float64      `json:"height"`
	Segs    [][2]float64 `json:"segs"`
	Drivers []string     `json:"drivers"`
	Texts   []struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Size  float64 `json:"size"`
		Text  string  `json:"text"`
		Range [2]int  `json:"range"`
	} `json:"texts"`
	Late []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Attr  string `json:"attr"`
		Frame int    `json:"frame"`
	} `json:"late"`
	Off []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Frame int    `json:"frame"`
	} `json:"off"`
	Clipped []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Frame int    `json:"frame"`
	} `json:"clipped"`
	Layers int `json:"layers"`
}

func cmdSceneCheck(a *app, args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	minText := fs.Float64("min-text", 28, "smallest readable text size at 1080 px frame height")
	maxStill := fs.Float64("max-still", 1.5, "longest still stretch in seconds")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	var d checkData
	if err := a.jsCall(checkJS, 5*time.Minute, &d); err != nil {
		return err
	}
	var out []finding
	if d.Layers == 0 {
		out = append(out, finding{Kind: "empty", Detail: "the active comp has no layers", Fix: "build the scene first"})
	}
	// Still stretches: frames not covered by any changing key segment.
	if len(d.Drivers) == 0 && d.Layers > 0 {
		sort.Slice(d.Segs, func(i, j int) bool { return d.Segs[i][0] < d.Segs[j][0] })
		cursor := float64(d.Start)
		var gaps [][2]float64
		for _, s := range d.Segs {
			if s[0] > cursor {
				gaps = append(gaps, [2]float64{cursor, s[0]})
			}
			if s[1] > cursor {
				cursor = s[1]
			}
		}
		if float64(d.End) > cursor {
			gaps = append(gaps, [2]float64{cursor, float64(d.End)})
		}
		stillFrames := 0.0
		for _, g := range gaps {
			stillFrames += g[1] - g[0]
		}
		if total := float64(d.End - d.Start); total > 0 && stillFrames/total > 0.5 {
			out = append(out, finding{Kind: "still",
				Detail: fmt.Sprintf("nothing moves in %.0f %% of the piece (many short holds add up)", 100*stillFrames/total),
				Fix:    "give holds a little life (cav.oscillate / cav.wiggle on position or scale, a slow drift, staggered accents), and keep holds to what the text needs"})
		}
		for _, g := range gaps {
			secs := (g[1] - g[0]) / d.FPS
			if secs > *maxStill {
				out = append(out, finding{Kind: "still",
					Detail: fmt.Sprintf("nothing moves from frame %d to %d (%.1f s)", int(g[0]), int(g[1]), secs),
					Fix:    "add secondary motion (a slow drift, scale breathe, cav.oscillate or cav.wiggle), start the next section earlier, or shorten the comp"})
			}
		}
	}
	scale := 1080 / d.Height
	for _, t := range d.Texts {
		if px := t.Size * scale; px < *minText {
			out = append(out, finding{Kind: "text", Layer: t.ID, Name: t.Name,
				Detail: fmt.Sprintf("%q is %.0f px (at 1080p); below %.0f px it is hard to read", t.Text, px, *minText),
				Fix:    fmt.Sprintf("raise its size to at least %.0f px, or cut it", *minText)})
		}
	}
	seenOff := map[string]bool{}
	for _, o := range d.Off {
		if seenOff[o.ID] {
			continue
		}
		seenOff[o.ID] = true
		out = append(out, finding{Kind: "offframe", Layer: o.ID, Name: o.Name,
			Detail: fmt.Sprintf("completely outside the frame at frame %d", o.Frame),
			Fix:    "check its position and its parents' positions (+y is up, (0,0) is the centre)"})
	}
	seenClip := map[string]bool{}
	for _, c := range d.Clipped {
		if seenClip[c.ID] {
			continue
		}
		seenClip[c.ID] = true
		out = append(out, finding{Kind: "clipped", Layer: c.ID, Name: c.Name,
			Detail: fmt.Sprintf("text is cut by the frame edge at frame %d", c.Frame),
			Fix:    "move it inside the frame (keep text within the middle 90 %), or make it smaller"})
	}
	for _, l := range d.Late {
		out = append(out, finding{Kind: "keys", Layer: l.ID, Name: l.Name,
			Detail: fmt.Sprintf("key on %s at frame %d is after the comp end (%d)", l.Attr, l.Frame, d.End),
			Fix:    "move the key inside the comp, or lengthen the comp with cav scene comp --frames"})
	}
	// Blank frames at the start and the end, from quick 10 % renders.
	if d.Layers > 0 {
		if f, err := a.blankRun(d.Start, d.End, d.FPS); err == nil {
			out = append(out, f...)
		}
	}
	a.emit(map[string]any{"findings": out, "count": len(out)}, func() {
		if len(out) == 0 {
			fmt.Println("no problems found")
			return
		}
		for _, f := range out {
			who := ""
			if f.Layer != "" {
				who = fmt.Sprintf(" %s %q", f.Layer, f.Name)
			}
			fmt.Printf("%-8s%s: %s\n         fix: %s\n", f.Kind, who, f.Detail, f.Fix)
		}
	})
	return nil
}

// blankRun reports long runs of blank frames at the start or the end of the comp.
func (a *app) blankRun(start, end int, fps float64) ([]finding, error) {
	step := int(fps / 4)
	if step < 1 {
		step = 1
	}
	var frames []int
	for f := end; f > end-int(3*fps) && f > start; f -= step {
		frames = append(frames, f)
	}
	for f := start; f < start+int(2*fps) && f < end; f += step {
		frames = append(frames, f)
	}
	dir := filepath.Join(os.TempDir(), "cav-check-"+bridge.NewID())
	defer os.RemoveAll(dir)
	paths, err := a.renderFrames(frames, 10, dir)
	if err != nil {
		return nil, err
	}
	blank := map[int]bool{}
	for i, p := range paths {
		blank[frames[i]] = isBlank(p)
	}
	var out []finding
	tail := 0
	for f := end; f > end-int(3*fps) && f > start; f -= step {
		if !blank[f] {
			break
		}
		tail = end - f + step
	}
	if secs := float64(tail) / fps; secs >= 1.0 {
		out = append(out, finding{Kind: "blank", Detail: fmt.Sprintf("the last %.1f s look empty", secs),
			Fix: "if the brief does not ask for an exit to empty, end on a finished frame (hold the lockup); if it does, shorten the comp or end the exit closer to the last frame. Do not lengthen holds the brief sized"})
	}
	head := 0
	for f := start; f < start+int(2*fps) && f < end; f += step {
		if !blank[f] {
			break
		}
		head = f - start + step
	}
	if secs := float64(head) / fps; secs >= 0.75 {
		out = append(out, finding{Kind: "blank", Detail: fmt.Sprintf("the first %.1f s look empty", secs),
			Fix: "bring the first element in within about half a second"})
	}
	return out, nil
}

// isBlank is true when the frame is (nearly) one flat colour.
func isBlank(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return false
	}
	b := img.Bounds()
	var n, sum, sum2 float64
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			l := (0.3*float64(r) + 0.59*float64(g) + 0.11*float64(bl)) / 257
			n++
			sum += l
			sum2 += l * l
		}
	}
	if n == 0 {
		return true
	}
	mean := sum / n
	return sum2/n-mean*mean < 16 // standard deviation under 4 levels
}
