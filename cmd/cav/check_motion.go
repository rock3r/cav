package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/rock3r/cav/internal/board"
)

// motionAttr is what structure.js read from the keys of one animated attribute. Times are
// frames, sorted. A nil value is a key that holds no plain number (a colour, a text).
// Span is set instead when there were too many keys to read: the attribute counts as moving
// from its first to its last key.
type motionAttr struct {
	Attr   string     `json:"attr"`
	Times  []float64  `json:"times"`
	Values []*float64 `json:"values"`
	Interp []int      `json:"interp"`
	Span   []float64  `json:"span"`
	// Eases holds, for each key, the ease of the segment that starts at it (nil for a plain
	// one). It is set only when at least one key has an ease.
	Eases []*keyEase `json:"eases,omitempty"`
}

// keyEase is what a key says about the segment that starts at it. Magic is Cavalry's Magic
// Easing number (Custom is 34, and Custom is then the range structure.js measured). Right and
// Left are the Bezier handles as offsets from the key, in frames and in value units.
type keyEase struct {
	Magic  int         `json:"magic,omitempty"`
	Custom *customEase `json:"custom,omitempty"`
	Right  []float64   `json:"right,omitempty"`
	Left   []float64   `json:"left,omitempty"`
}

// customEase is the range a Custom ease formula reaches for x from 0 to 1. Error is set when
// the formula was not evaluated.
type customEase struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Mono  bool    `json:"mono"`
	Error string  `json:"error,omitempty"`
}

// motionInfo says whether the motion metadata of every layer was read.
type motionInfo struct {
	Complete bool `json:"complete"`
}

// frameSpan is the frames [From, To).
type frameSpan struct{ From, To float64 }

// interpStep is the data.interpolation value of a step key: the value jumps at the next key.
const interpStep = 2

// driverType matches layers that change over time without keys: oscillators, noise, waves,
// sound, counters and scripts. They draw nothing; they move the layers they are connected to.
var driverType = regexp.MustCompile(`^(oscillator|noise|wave|sound|spring|looper|localTime|timelineCounter|frame|secondsToFrames|javaScript.*)$`)

// nonDrawingType matches layers that draw nothing themselves: filters, shaders, deformers,
// materials and their behaviours, and cameras. Their in and out frames do not put anything on
// screen, so they are not counted as shown. The names are Cavalry's layer type ids.
var nonDrawingType = regexp.MustCompile(`(Filter|Shader|Deformer|Material|Behaviou?r)$|^(materialBehaviour|behaviour|materialSampler|shaderArray)|[cC]amera`)

// mergeSpans sorts spans and joins those that touch or overlap.
func mergeSpans(in []frameSpan) []frameSpan {
	var s []frameSpan
	for _, x := range in {
		if x.To > x.From {
			s = append(s, x)
		}
	}
	sort.Slice(s, func(i, j int) bool { return s[i].From < s[j].From })
	var out []frameSpan
	for _, x := range s {
		if n := len(out); n > 0 && x.From <= out[n-1].To {
			out[n-1].To = math.Max(out[n-1].To, x.To)
			continue
		}
		out = append(out, x)
	}
	return out
}

// clipSpans keeps the parts of in that lie inside the window.
func clipSpans(in []frameSpan, window frameSpan) []frameSpan {
	var out []frameSpan
	for _, x := range in {
		if f, t := math.Max(x.From, window.From), math.Min(x.To, window.To); t > f {
			out = append(out, frameSpan{f, t})
		}
	}
	return out
}

// subtractSpans returns the parts of whole that no span of cut covers. Both are merged.
func subtractSpans(whole, cut []frameSpan) []frameSpan {
	var out []frameSpan
	for _, w := range whole {
		from := w.From
		for _, c := range cut {
			if c.To <= from || c.From >= w.To {
				continue
			}
			if c.From > from {
				out = append(out, frameSpan{from, c.From})
			}
			from = math.Max(from, c.To)
		}
		if from < w.To {
			out = append(out, frameSpan{from, w.To})
		}
	}
	return out
}

func totalSpans(s []frameSpan) float64 {
	var n float64
	for _, x := range s {
		n += x.To - x.From
	}
	return n
}

// attrLive lists the frames over which the keys of one attribute change its value. Equal
// values on both sides of a pair are a hold. Unknown values count as moving. A step pair
// changes the value only at its second key.
func attrLive(m motionAttr) []frameSpan {
	if len(m.Span) == 2 {
		return []frameSpan{{m.Span[0], m.Span[1]}}
	}
	var out []frameSpan
	for i := 0; i+1 < len(m.Times) && i+1 < len(m.Values); i++ {
		a, b := m.Values[i], m.Values[i+1]
		if a != nil && b != nil && math.Abs(*a-*b) < 1e-9 {
			continue
		}
		if i < len(m.Interp) && m.Interp[i] == interpStep {
			out = append(out, frameSpan{m.Times[i+1], m.Times[i+1] + 1})
			continue
		}
		out = append(out, frameSpan{m.Times[i], m.Times[i+1]})
	}
	return out
}

// deadTimeFindings reports the stretches of the active comp, longer than maxDead seconds,
// where something is on screen and no layer has moving keys, an expression or a live driver.
// It reads only metadata, so it works with --quick and with pre-comps (a pre-comp counts as
// moving, as it is not opened). A stretch that lies inside a storyboard shot marked "hold"
// is returned as intended instead. The result is nil when the motion metadata is missing,
// and skipped says why when it is incomplete.
func deadTimeFindings(d structureData, sb *board.Board, maxDead float64) (flagged, intended []finding, spans []frameSpan, skipped []map[string]any) {
	if d.Motion == nil || d.FPS <= 0 {
		return
	}
	if !d.Motion.Complete || (d.CoverageComplete != nil && !*d.CoverageComplete) {
		skipped = append(skipped, map[string]any{"inspection": "dead-time", "reason": "incomplete motion metadata"})
		return
	}
	whole := frameSpan{float64(d.Start), float64(d.End + 1)}
	byID := map[string]structureLayer{}
	children := map[string]bool{}
	for _, l := range d.Layers {
		if l.Comp != d.Comp {
			continue
		}
		byID[l.ID] = l
		children[l.Parent] = true
	}
	outEdges := map[string]bool{}
	opacityDriven := map[string]bool{}
	for _, e := range d.Edges {
		outEdges[e.From] = true
		if e.ToAttr == "opacity" {
			opacityDriven[e.To] = true
		}
	}

	// The frames a layer is shown: its own in and out frames, inside those of its parents.
	visCache := map[string]frameSpan{}
	var visible func(id string, depth int) frameSpan
	visible = func(id string, depth int) frameSpan {
		if v, ok := visCache[id]; ok {
			return v
		}
		l, ok := byID[id]
		if !ok || depth > 64 {
			return whole
		}
		v := whole
		if len(l.Boundaries) >= 2 {
			v = frameSpan{math.Max(v.From, float64(l.Boundaries[0])), math.Min(v.To, float64(l.Boundaries[1]))}
		}
		if l.Parent != "" {
			p := visible(l.Parent, depth+1)
			v = frameSpan{math.Max(v.From, p.From), math.Min(v.To, p.To)}
		}
		visCache[id] = v
		return v
	}
	// A layer is off when it is hidden, or when its opacity is 0 and nothing changes the opacity:
	// no keys, no expression and no connection. What is below an off layer is not on screen.
	off := func(l structureLayer) bool {
		if l.Hidden != nil && *l.Hidden {
			return true
		}
		if l.Opacity == nil || *l.Opacity != 0 {
			return false
		}
		for _, m := range l.Motion {
			if m.Attr == "opacity" {
				return false
			}
		}
		for _, x := range l.Expressions {
			if x == "opacity" {
				return false
			}
		}
		return !opacityDriven[l.ID]
	}
	offAbove := func(id string) bool {
		for depth := 0; depth < 64; depth++ {
			l, ok := byID[id]
			if !ok {
				return false
			}
			if off(l) {
				return true
			}
			id = l.Parent
		}
		return false
	}

	live := map[string][]frameSpan{}
	var shown []frameSpan
	counts := map[string]bool{} // layers whose own motion counts as something moving
	for id, l := range byID {
		v := visible(id, 0)
		empty := ((l.Type == "null" || l.Type == "group") && !children[id] && !outEdges[id])
		driver := driverType.MatchString(l.Type) && !(isSimulation(l.Type) && l.Type != "spring")
		// A layer under an off layer shows nothing and moves nothing on screen. An off driver
		// still drives the visible layers it is connected to.
		onScreen := !offAbove(id)
		counts[id] = onScreen && !driver && !empty
		if onScreen && !children[id] && !driver && !empty && !nonDrawingType.MatchString(l.Type) && v.To > v.From {
			shown = append(shown, v)
		}
		var own []frameSpan
		switch {
		case len(l.Expressions) > 0, driver, l.Type == "compositionReference", isSimulation(l.Type):
			own = []frameSpan{v}
		default:
			for _, m := range l.Motion {
				own = append(own, clipSpans(attrLive(m), v)...)
			}
		}
		live[id] = mergeSpans(own)
	}
	// Whatever a live layer is connected to moves while it does: a layer driven by an
	// oscillator, or a layer a keyed layer feeds.
	// Repeat until nothing changes; a chain cannot be longer than the number of layers.
	for round := 0; round <= len(byID); round++ {
		changed := false
		for _, e := range d.Edges {
			src, ok := live[e.From]
			if _, target := byID[e.To]; !ok || !target || len(src) == 0 {
				continue
			}
			before := totalSpans(live[e.To])
			live[e.To] = mergeSpans(append(live[e.To], clipSpans(src, visible(e.To, 0))...))
			changed = changed || totalSpans(live[e.To]) > before
		}
		if !changed {
			break
		}
	}
	var moving []frameSpan
	for id, s := range live {
		if counts[id] {
			moving = append(moving, s...)
		}
	}
	dead := subtractSpans(mergeSpans(shown), mergeSpans(moving))

	var shots []board.ShotSpan
	if sb != nil {
		shots = sb.Spans(d.FPS, d.Start)
	}
	for _, s := range dead {
		secs := (s.To - s.From) / d.FPS
		if secs <= maxDead {
			continue
		}
		spans = append(spans, s)
		f := deadTimeFinding(s, secs)
		var holds bool
		for _, sh := range shots {
			if s.From >= float64(sh.From) && s.From < float64(sh.To) {
				f.Name = sh.ID
				f.Evidence.(map[string]any)["shot"] = sh.ID
				holds = sh.Hold && s.To <= float64(sh.To)
				break
			}
		}
		if holds {
			f.Severity = "info"
			f.Detail = fmt.Sprintf("nothing moves from frame %d to %d (%.1f s); shot %s is marked as a hold", int(s.From), int(s.To), secs, f.Name)
			f.Fix = "none; the hold is marked on purpose"
			intended = append(intended, f)
			continue
		}
		flagged = append(flagged, f)
	}
	return
}

func deadTimeFinding(s frameSpan, secs float64) finding {
	return finding{Kind: "dead-time", Severity: "warning",
		Evidence: map[string]any{"from": int(s.From), "to": int(s.To), "seconds": math.Round(secs*10) / 10},
		Detail:   fmt.Sprintf("no layer has moving keys or a live driver from frame %d to %d (%.1f s)", int(s.From), int(s.To), secs),
		Fix:      "keep something alive during the hold (cav.oscillate / cav.wiggle on the hero, a slow drift or scale breathe, a ticking value), shorten the hold, or mark the shot \"hold\": true in storyboard.json if the stillness is intended"}
}

// insideSpan says a still gap lies fully inside one dead-time span, which already reports it.
func insideSpan(gap [2]float64, spans []frameSpan) bool {
	for _, s := range spans {
		if gap[0] >= s.From && gap[1] <= s.To {
			return true
		}
	}
	return false
}

// magicEases names Cavalry's Magic Easing numbers (data.easing) that overshoot or bounce. The
// other ones (1-3 and 10-12) are monotonic; 34 is Custom.
var magicEases = map[int]string{
	22: "SpringIn", 23: "SpringOut", 24: "SpringInSpringOut",
	25: "SmallSpringIn", 26: "SmallSpringOut", 27: "SmallSpringInSmallSpringOut",
	28: "AnticipateIn", 29: "OvershootOut", 30: "AnticipateInOvershootOut",
	31: "BounceIn", 32: "BounceOut", 33: "BounceInBounceOut",
}

const (
	easeCustom = 34
	// A custom ease may leave 0..1 by this much before it counts as an overshoot.
	easeSlack = 0.005
	// A Bezier segment may leave the range of its two key values by this share of the move.
	bezierSlack = 0.005
)

// easeOvershoots says whether the segment from a key with ease e0 to the next key with ease
// e1, going from value v0 to v1, overshoots or bounces, and why. v0 and v1 are nil when a key
// holds no plain number; only a Bezier needs them. Linear, step and the plain Magic Easing
// names never overshoot.
func easeOvershoots(e0, e1 *keyEase, v0, v1 *float64) (bool, string) {
	if e0 == nil {
		return false, ""
	}
	if name, ok := magicEases[e0.Magic]; ok {
		return true, name
	}
	if e0.Magic == easeCustom && e0.Custom != nil && e0.Custom.Error == "" {
		c := e0.Custom
		switch {
		case c.Max > 1+easeSlack:
			return true, fmt.Sprintf("a custom ease that goes to %.0f %% of the move", 100*c.Max)
		case c.Min < -easeSlack:
			return true, fmt.Sprintf("a custom ease that first goes %.0f %% the wrong way", -100*c.Min)
		case !c.Mono:
			return true, "a custom ease that moves back and forth"
		}
		return false, ""
	}
	if e0.Magic == 0 && len(e0.Right) == 2 && e1 != nil && len(e1.Left) == 2 && v0 != nil && v1 != nil {
		return bezierOvershoots(e0.Right[1], e1.Left[1], *v0, *v1)
	}
	return false, ""
}

// bezierOvershoots evaluates the values of the cubic that the handles draw: the first handle
// is the Right handle of the key that starts the segment, the second is the Left handle of
// the key that ends it. Both are offsets from their key, so the value of a handle point is
// the key value plus the offset. The time of the handles does not change the range.
func bezierOvershoots(rightY, leftY, v0, v1 float64) (bool, string) {
	move := math.Abs(v1 - v0)
	if move < 1e-9 {
		return false, ""
	}
	y := [4]float64{v0, v0 + rightY, v1 + leftY, v1}
	lo, hi := math.Min(v0, v1), math.Max(v0, v1)
	worst := 0.0
	for i := 0; i <= 32; i++ {
		t := float64(i) / 32
		u := 1 - t
		v := u*u*u*y[0] + 3*u*u*t*y[1] + 3*u*t*t*y[2] + t*t*t*y[3]
		worst = math.Max(worst, math.Max(lo-v, v-hi))
	}
	if worst > bezierSlack*move {
		return true, fmt.Sprintf("a curve that goes %.0f %% past the end value", 100*worst/move)
	}
	return false, ""
}

// Names in an attribute or layer that mean the value is data.
var (
	dataAttrName  = regexp.MustCompile(`(?i)(^|\.)(value|number|percent(age)?|progress|amount|level)$`)
	dataLayerName = regexp.MustCompile(`(?i)\b(bar|meter|progress|gauge|ring|arc|donut|pie|chart|graph|value|count(er)?|number|percent|stat|score|kpi|level|battery|volume)\b`)
	// Attributes that draw a length. The ring and arc of an ellipse are drawn with the stroke trim.
	lengthAttr = regexp.MustCompile(`^(scale\.[xy]|generator\.dimensions(\.[xy])?|stroke\.trim(Start|End))$`)
	// Attributes that place a layer; they never show a value.
	neverDataAttr = regexp.MustCompile(`^(position|rotation)(\.[xyz])?$|^opacity$`)
)

// dataRole says why an attribute of a layer shows data: "userData" (cav.role), "text" (it
// feeds a text layer), "attr" (the attribute name) or "name" (the layer name and a length
// attribute). It is empty when the attribute is not data. The first rule that applies wins.
func dataRole(l structureLayer, attr string, hasChildren, feedsText bool) string {
	switch l.Role {
	case "data":
		return "userData"
	case "container":
		return ""
	}
	if l.Type == "group" || l.Type == "compositionReference" || hasChildren || neverDataAttr.MatchString(attr) {
		return ""
	}
	switch {
	case feedsText:
		return "text"
	case dataAttrName.MatchString(attr):
		return "attr"
	case lengthAttr.MatchString(attr) && dataLayerName.MatchString(l.Name):
		return "name"
	}
	return ""
}

// overshootFindings reports data that eases with a spring, a back, a bounce or any other ease
// that leaves the range between its two values (see dataRole for what counts as data). One
// finding per attribute, for the first segment that overshoots. A Custom ease that structure.js
// could not evaluate on a data attribute is listed in skipped. It reads metadata only.
func overshootFindings(d structureData) (flagged []finding, skipped []map[string]any) {
	if d.Motion == nil {
		return
	}
	hasChildren := map[string]bool{}
	for _, l := range d.Layers {
		hasChildren[l.Parent] = true
	}
	types := map[string]string{}
	for _, l := range d.Layers {
		types[l.ID] = l.Type
	}
	feedsText := map[string]bool{}
	for _, e := range d.Edges {
		if (e.ToAttr == "text" || strings.HasPrefix(e.ToAttr, "text.")) && types[e.To] == "textShape" {
			feedsText[e.From] = true
		}
	}
	done := map[string]bool{}
	for _, l := range d.Layers {
		for _, m := range l.Motion {
			if len(m.Eases) == 0 {
				continue
			}
			role := dataRole(l, m.Attr, hasChildren[l.ID], feedsText[l.ID])
			if role == "" {
				continue
			}
			unread := false
			for i := 0; i+1 < len(m.Times) && i < len(m.Eases); i++ {
				var v0, v1 *float64
				if i+1 < len(m.Values) {
					v0, v1 = m.Values[i], m.Values[i+1]
				}
				if v0 != nil && v1 != nil && math.Abs(*v0-*v1) < 1e-9 {
					continue
				}
				if i < len(m.Interp) && m.Interp[i] == interpStep {
					continue
				}
				e := m.Eases[i]
				if e != nil && e.Magic == easeCustom && (e.Custom == nil || e.Custom.Error != "") {
					unread = true
				}
				var e1 *keyEase
				if i+1 < len(m.Eases) {
					e1 = m.Eases[i+1]
				}
				if bad, why := easeOvershoots(e, e1, v0, v1); bad {
					flagged = append(flagged, overshootFinding(l, m.Attr, m.Times[i], why, role))
					done[l.ID+"\x00"+m.Attr] = true
					unread = false
					break
				}
			}
			if unread {
				skipped = append(skipped, map[string]any{"inspection": "ease", "layer": l.ID, "attr": m.Attr, "reason": "custom expression not evaluated"})
			}
		}
	}
	// A spring layer connected to a data attribute overshoots by itself.
	byID := map[string]structureLayer{}
	for _, l := range d.Layers {
		byID[l.ID] = l
	}
	for _, e := range d.Edges {
		l, ok := byID[e.To]
		if !ok || types[e.From] != "spring" || done[e.To+"\x00"+e.ToAttr] {
			continue
		}
		if role := dataRole(l, e.ToAttr, hasChildren[l.ID], feedsText[l.ID]); role != "" {
			done[e.To+"\x00"+e.ToAttr] = true
			flagged = append(flagged, overshootFinding(l, e.ToAttr, float64(d.Start), "a spring layer", role))
		}
	}
	return
}

func overshootFinding(l structureLayer, attr string, frame float64, ease, role string) finding {
	return finding{Kind: "data-overshoot", Layer: l.ID, Name: l.Name, Comp: l.Comp, CompPath: l.CompPath, Severity: "warning",
		Evidence: map[string]any{"attr": attr, "frame": int(frame), "ease": ease, "role": role},
		Detail:   fmt.Sprintf("%s on %s uses %s, which overshoots or bounces, so the data shows a wrong value for a moment", attr, l.Name, ease),
		Fix:      "use an ease that settles without overshoot (outCubic, outQuint, outExpo, inOutCubic) on the value; keep the spring on its card or group; if this layer is not data, cav.role(layer, 'container')"}
}
