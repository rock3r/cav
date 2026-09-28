package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
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
  offframe  text or large shapes that never come inside the frame
  clipped   text cut by the frame edge
  edge      text closer than 3 % of the frame height to an edge
  overflow  text that spills out of, or crowds, the button, pill or field it sits on
  hidden    text partly covered by an opaque shape drawn above it
  blank     blank frames at the start or the end (a long empty tail)
  keys      keyframes after the end of the comp (they never play)
Each finding says what to change. Exit code 0 even with findings; use --json to read them.
Keys, oscillators, noise and particles count as motion. Motion on a small layer (under 6 %
of the frame height) counts only when at least three small layers move at the same time.
Text and shapes are checked where they rest (the middle of their visible time); faint
layers (opacity under 50) are skipped.`
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
var W = res.x / 2, H = res.y / 2;
var ids = api.getCompLayers(false);
var segs = [], texts = [], late = [], shapes = [];
var driverTypes = { oscillator: 1, noise: 1, spring: 1, wave: 1, particleShape: 2, forgeDynamicsShape: 2 };
// setFrame re-evaluates the whole scene, so every lookup is batched by frame: at[f] holds the
// functions to run while the playhead is on frame f.
var at = {};
function when(f, fn) { f = Math.round(f); (at[f] = at[f] || []).push(fn) }
function flush() {
  Object.keys(at).map(Number).sort(function (x, y) { return x - y }).forEach(function (f) {
    api.setFrame(f);
    at[f].forEach(function (fn) { try { fn() } catch (e) {} });
  });
  at = {};
}
function visibleRange(id) {
  var a = start, b = end, p = id;
  while (p && p !== comp) {
    try { a = Math.max(a, api.getInFrame(p)); b = Math.min(b, api.getOutFrame(p)) } catch (e) {}
    try { if (api.get(p, 'hidden')) return null } catch (e) {}
    p = api.getParent(p);
  }
  return a <= b ? [a, b] : null;
}
// Motion on a layer smaller than 6 % of the frame height (a small dot) is hard to notice.
function small(id) {
  try { var b = api.getBoundingBox(id, true); return Math.max(b.width, b.height) < 0.06 * res.y } catch (e) { return false }
}
var transformKeys = {};
function movesItself(id) {
  var p = id;
  while (p && p !== comp) { if (transformKeys[p]) return true; p = api.getParent(p) }
  return false;
}
ids.forEach(function (id) {
  var type = api.getLayerType(id);
  var range = visibleRange(id);
  if (driverTypes[type] === 2 && range) segs.push([range[0], range[1], 0]);
  if (driverTypes[type] === 1) {
    var outs = [];
    try { outs = api.getOutConnectedAttributes(id) || [] } catch (e) {}
    outs.forEach(function (attr) {
      var targets = [];
      try { targets = api.getOutConnections(id, attr) || [] } catch (e) {}
      targets.forEach(function (t) {
        var layer = String(t).split('.')[0], r = visibleRange(layer);
        if (!r) return;
        if (/^(position|scale|rotation)/.test(String(t).split('.')[1] || '')) transformKeys[layer] = 1;
        when(r[0], function () { segs.push([r[0], r[1], small(layer) ? 1 : 0]) });
      });
    });
  }
  var anim = [];
  try { anim = api.getAnimatedAttributes(id) } catch (e) {}
  anim.forEach(function (attr) {
    if (/^(position|scale|rotation)/.test(attr)) transformKeys[id] = 1;
    var times = [];
    try { times = api.getKeyframeTimes(id, attr) } catch (e) {}
    times.sort(function (x, y) { return x - y });
    for (var i = 0; i < times.length; i++) {
      if (times[i] > end) late.push({ id: id, name: api.getNiceName(id), attr: attr, frame: times[i] });
    }
    if (!range) return;
    for (var j = 0; j + 1 < times.length; j++) {
      (function (t0, t1) {
        var q = {};
        when(t0, function () { q.v0 = JSON.stringify(api.get(id, attr)); q.tiny = small(id) });
        when(t1, function () {
          if (q.v0 !== undefined && q.v0 !== JSON.stringify(api.get(id, attr)))
            segs.push([Math.max(t0, range[0]), Math.min(t1, range[1]), q.tiny ? 1 : 0]);
        });
      })(times[j], times[j + 1]);
    }
  });
  if (range && (type === 'textShape' || type === 'basicShape')) shapes.push({ id: id, type: type, range: range, looks: {} });
  // A word built from one-letter layers (cav.glyphs) is checked as one text.
  if (range && type === 'group') {
    var kids = [], word = '';
    try { kids = api.getChildren(id) || [] } catch (e) {}
    var letters = kids.filter(function (k) {
      if (api.getLayerType(k) !== 'textShape') return false;
      var tx = api.get(k, 'text'); tx = String(tx && tx.text !== undefined ? tx.text : tx);
      return tx.length === 1;
    });
    if (letters.length >= 2 && letters.length === kids.length) {
      letters.slice().reverse().forEach(function (k) { var tx = api.get(k, 'text'); word += String(tx && tx.text !== undefined ? tx.text : tx) });
      shapes.push({ id: id, type: 'word', word: word, range: range, looks: {} });
    }
  }
});
// Drawing order: cav tree lists layers top first, so a depth-first walk numbers them from the
// top; a smaller number is drawn above a larger one.
var order = {}, counter = 0;
(function walk(list) {
  list.forEach(function (id) { order[id] = counter++; var k = []; try { k = api.getChildren(id) || [] } catch (e) {} walk(k) });
})(api.getCompLayers(true));
function isAncestor(a, b) { var p = b; while (p && p !== comp) { if (p === a) return true; p = api.getParent(p) } return false }
function opaque(id) {
  try { if (!api.hasFill(id)) return false } catch (e) { return false }
  try { var c = api.get(id, 'material.materialColor'); if (c && c.a !== undefined && c.a < 240) return false } catch (e) {}
  return true;
}
// Sample the whole comp at up to 30 frames (and 2 frames later, to see what is moving fast).
// A layer "rests" on a sample where it is visible (opacity over 50 through its parents) and
// moves less than 0.3 % of the frame height in 2 frames; a slow drift still counts as resting.
var n = Math.max(1, Math.min(30, Math.floor((end - start) / 6)));
var samples = [];
for (var i = 0; i < n; i++) samples.push(Math.round(start + (end - start) * (i + 0.5) / n));
function look(s) {
  var op = 1, sc = 1, p = s.id;
  while (p && p !== comp) {
    try { op *= api.get(p, 'opacity') / 100 } catch (e) {}
    try { sc *= Math.abs(api.get(p, 'scale').y) } catch (e) {}
    p = api.getParent(p);
  }
  if (s.type === 'word') {
    // A word is as visible as its most visible letter (typed or cascading letters start hidden).
    var most = 0;
    (api.getChildren(s.id) || []).forEach(function (k) { try { most = Math.max(most, api.get(k, 'opacity') / 100) } catch (e) {} });
    op *= most;
  }
  var b = null;
  try { b = api.getBoundingBox(s.id, true) } catch (e) {}
  return { op: op, sc: sc, b: b };
}
samples.forEach(function (f) {
  var g = Math.min(f + 2, end);
  shapes.forEach(function (s) {
    if (f < s.range[0] || f > s.range[1]) return;
    when(f, function () { s.looks[f] = look(s) });
    when(g, function () { s.looks[f + ':'] = look(s) });
  });
});
flush();
var off = [], clipped = [], tight = [], overflow = [], hidden = [];
var margin = 0.03 * res.y;
shapes.forEach(function (s) {
  var rest = [], seen = false, inside = false;
  samples.forEach(function (f) {
    var x = s.looks[f], y = s.looks[f + ':'];
    if (!x || !x.b || x.op < 0.5 || x.sc === 0 || x.b.width * x.b.height === 0) return;
    seen = true;
    var b = x.b;
    if (!(b.right < -W || b.left > W || b.top < -H || b.bottom > H)) inside = true;
    if (!y || !y.b) return;
    var d = Math.abs(b.left - y.b.left) + Math.abs(b.right - y.b.right) + Math.abs(b.top - y.b.top) + Math.abs(b.bottom - y.b.bottom);
    if (d < 0.003 * res.y) rest.push({ f: f, b: b, sc: x.sc });
  });
  // Off frame: visible but never inside the frame, on a layer whose transform is not animated
  // (a wipe band that parks outside after crossing the frame is fine).
  if (seen && !inside && !movesItself(s.id)) {
    var big = s.type === 'textShape' || rest.some(function (r) { return r.b.width * r.b.height > 0.005 * res.x * res.y });
    if (big) off.push({ id: s.id, name: api.getNiceName(s.id), frame: rest.length ? rest[0].f : s.range[0] });
    return;
  }
  if ((s.type !== 'textShape' && s.type !== 'word') || !rest.length) return;
  var t = s.type === 'word' ? s.word : api.get(s.id, 'text'), size = 0;
  if (s.type === 'textShape') {
    rest.forEach(function (r) { size = Math.max(size, api.get(s.id, 'fontSize') * r.sc) });
    texts.push({ id: s.id, name: api.getNiceName(s.id), size: size, text: String(t && t.text !== undefined ? t.text : t).slice(0, 40), range: s.range });
  }
  // Text cut by the frame edge, or touching it, while it rests and should be readable. Words are
  // skipped here: each of their letters is checked on its own.
  for (var k = 0; k < rest.length && s.type !== 'word'; k++) {
    var b = rest[k].b, f = rest[k].f;
    if (b.right < -W || b.left > W || b.top < -H || b.bottom > H) continue;
    var cut = b.left < -W - 1 || b.right > W + 1 || b.top > H + 1 || b.bottom < -H - 1;
    var near = b.left < -W + margin || b.right > W - margin || b.top > H - margin || b.bottom < -H + margin;
    if (cut) { clipped.push({ id: s.id, name: api.getNiceName(s.id), frame: f }); break }
    if (near) { tight.push({ id: s.id, name: api.getNiceName(s.id), frame: f }); break }
  }
  var str = String(t && t.text !== undefined ? t.text : t);
  // Text partly hidden behind an opaque shape drawn above it (a button left out of its dialog's
  // group ends up under the dialog). Content dimmed by a translucent overlay (a modal backdrop)
  // is background on purpose, so it is skipped.
  if (str.length > 1) {
    for (var k3 = 0; k3 < rest.length; k3++) {
      var hb = rest[k3].b, hf = rest[k3].f, tarea = hb.width * hb.height, cover = null, dimmed = false;
      shapes.forEach(function (o) {
        if (o.type !== 'basicShape' || o.id === s.id || !(order[o.id] < order[s.id]) || isAncestor(o.id, s.id)) return;
        var ol = o.looks[hf], ol2 = o.looks[hf + ':'];
        if (!ol || !ol.b || ol.op < 0.05 || ol.sc === 0) return;
        // A panel sweeping across during a transition covers text only for a moment.
        if (!ol2 || !ol2.b || Math.abs(ol.b.left - ol2.b.left) + Math.abs(ol.b.right - ol2.b.right) + Math.abs(ol.b.top - ol2.b.top) + Math.abs(ol.b.bottom - ol2.b.bottom) >= 0.003 * res.y) return;
        var ob = ol.b, ow = Math.min(hb.right, ob.right) - Math.max(hb.left, ob.left), oh = Math.min(hb.top, ob.top) - Math.max(hb.bottom, ob.bottom);
        if (ow <= 0 || oh <= 0) return;
        var part = ow * oh / tarea;
        if (ol.op < 0.95 || !opaque(o.id)) { if (part > 0.9 && ob.width * ob.height > 0.25 * res.x * res.y) dimmed = true; return }
        if (part > 0.1 && part < 0.95) cover = o.id;
      });
      if (cover && !dimmed) { hidden.push({ id: s.id, name: api.getNiceName(s.id), frame: hf, by: api.getNiceName(cover) }); break }
    }
  }
  // Text that spills out of the small shape it sits on (a button, pill, field or badge).
  for (var k2 = 0; k2 < rest.length; k2++) {
    if (str.length < 2) break;
    var tb = rest[k2].b, tf = rest[k2].f, cx = (tb.left + tb.right) / 2, cy = (tb.top + tb.bottom) / 2, box = null, boxOrder = Infinity, boxMoving = false;
    shapes.forEach(function (o) {
      // A container is drawn below the text and has a fill (an outline ring is not a box).
      if (o.type !== 'basicShape' || !(order[o.id] > order[s.id]) || !opaque(o.id)) return;
      var ol = o.looks[tf], ol2 = o.looks[tf + ':'];
      if (!ol || !ol.b || ol.op < 0.5 || ol.sc === 0) return;
      var moving = !ol2 || !ol2.b || Math.abs(ol.b.left - ol2.b.left) + Math.abs(ol.b.right - ol2.b.right) + Math.abs(ol.b.top - ol2.b.top) + Math.abs(ol.b.bottom - ol2.b.bottom) >= 0.003 * res.y;
      var ob = ol.b, area = ob.width * ob.height;
      if (area > 0.15 * res.x * res.y || area < tb.width * tb.height * 0.8) return;
      if (cx < ob.left || cx > ob.right || cy < ob.bottom || cy > ob.top) return;
      // A container is at least as tall as the text and already covers at least half of it; text that
      // only crosses an unrelated shape (a progress bar under a dialog) does not count.
      var ow = Math.min(tb.right, ob.right) - Math.max(tb.left, ob.left), oh = Math.min(tb.top, ob.top) - Math.max(tb.bottom, ob.bottom);
      if (ob.height < tb.height || ow <= 0 || oh <= 0 || ow * oh < 0.5 * tb.width * tb.height) return;
      // The container is the nearest such shape below the text, not the smallest: a label on a
      // dialog sits on the dialog even when a smaller card under the dialog also contains it.
      if (!box || order[o.id] < boxOrder) { box = ob; boxOrder = order[o.id]; boxMoving = moving }
    });
    // The container is still sliding or scaling in: judge the text on a later frame.
    if (!box || boxMoving) continue;
    if (tb.left < box.left - 2 || tb.right > box.right + 2 || tb.top > box.top + 2 || tb.bottom < box.bottom - 2) {
      overflow.push({ id: s.id, name: api.getNiceName(s.id), frame: tf });
      break;
    }
    // Cramped: side padding under half the text height on a wide shape (a pill or button whose
    // text touches its ends). Round or square shapes such as dials are left alone.
    if (box.width >= 1.8 * box.height && Math.min(tb.left - box.left, box.right - tb.right) < 0.5 * tb.height) {
      overflow.push({ id: s.id, name: api.getNiceName(s.id), frame: tf, cramped: true });
      break;
    }
  }
});
api.setFrame(start);
return { start: start, end: end, fps: api.get(comp, 'fps'), height: res.y, segs: segs, texts: texts, late: late.slice(0, 20), off: off.slice(0, 20), clipped: clipped.slice(0, 20), tight: tight.slice(0, 20), overflow: overflow.slice(0, 20), hidden: hidden.slice(0, 20), layers: ids.length };`

type checkData struct {
	Start  int          `json:"start"`
	End    int          `json:"end"`
	FPS    float64      `json:"fps"`
	Height float64      `json:"height"`
	Segs   [][3]float64 `json:"segs"` // start, end, 1 when the layer is small
	Texts  []struct {
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
	Tight []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Frame int    `json:"frame"`
	} `json:"tight"`
	Overflow []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Frame   int    `json:"frame"`
		Cramped bool   `json:"cramped"`
	} `json:"overflow"`
	Hidden []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Frame int    `json:"frame"`
		By    string `json:"by"`
	} `json:"hidden"`
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
	// Still stretches: frames where no large layer and fewer than three small layers change.
	if d.Layers > 0 {
		gaps := stillGaps(d.Segs, d.Start, d.End)
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
					Detail: fmt.Sprintf("nothing visible moves from frame %d to %d (%.1f s); motion on one small layer does not count", int(g[0]), int(g[1]), secs),
					Fix:    "move something larger during the hold (a slow drift or scale breathe on the main text or shapes), start the next section earlier, or shorten the comp"})
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
	seenTight := map[string]bool{}
	for _, c := range d.Tight {
		if seenTight[c.ID] || seenClip[c.ID] {
			continue
		}
		seenTight[c.ID] = true
		out = append(out, finding{Kind: "edge", Layer: c.ID, Name: c.Name,
			Detail: fmt.Sprintf("text touches the frame edge at frame %d (less than 3 %% margin)", c.Frame),
			Fix:    "leave a margin of at least 5 % of the frame around text; move it in or make it smaller"})
	}
	for _, o := range d.Overflow {
		detail := fmt.Sprintf("text spills out of the shape behind it at frame %d", o.Frame)
		if o.Cramped {
			detail = fmt.Sprintf("text nearly touches the ends of the shape behind it at frame %d (side padding under half the text height)", o.Frame)
		}
		out = append(out, finding{Kind: "overflow", Layer: o.ID, Name: o.Name, Detail: detail,
			Fix: "make the shape wider than the text by at least one text height, or make the text smaller"})
	}
	for _, h := range d.Hidden {
		out = append(out, finding{Kind: "hidden", Layer: h.ID, Name: h.Name,
			Detail: fmt.Sprintf("text is partly hidden behind %q, which is drawn above it, at frame %d", h.By, h.Frame),
			Fix:    "put the text (and the button or card it belongs to) inside that shape's group with parent:, or move it clear; check the stacking with cav tree"})
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

// stillGaps returns the frame ranges where no large layer changes and fewer than three small
// layers change at once.
func stillGaps(segs [][3]float64, start, end int) [][2]float64 {
	n := end - start // frame steps; a key segment [a, b) covers the steps from a to b
	if n <= 0 {
		return nil
	}
	big := make([]bool, n)
	smallCount := make([]int, n)
	for _, s := range segs {
		a, b := int(s[0])-start, int(s[1])-start
		for f := max(a, 0); f < min(b, n); f++ {
			if s[2] == 0 {
				big[f] = true
			} else {
				smallCount[f]++
			}
		}
	}
	var gaps [][2]float64
	runStart := -1
	for f := 0; f <= n; f++ {
		moving := f == n || big[f] || smallCount[f] >= 3
		if !moving && runStart < 0 {
			runStart = f
		}
		if moving && runStart >= 0 {
			gaps = append(gaps, [2]float64{float64(start + runStart), float64(start + f)})
			runStart = -1
		}
	}
	return gaps
}
