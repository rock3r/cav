package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"

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
	for _, e := range d.Edges {
		outEdges[e.From] = true
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
	hiddenAbove := func(id string) bool {
		for depth := 0; depth < 64; depth++ {
			l, ok := byID[id]
			if !ok {
				return false
			}
			if l.Hidden != nil && *l.Hidden {
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
		hidden := l.Hidden != nil && *l.Hidden
		empty := (l.Type == "null" || l.Type == "group") && !children[id] && !outEdges[id]
		driver := driverType.MatchString(l.Type) && !(isSimulation(l.Type) && l.Type != "spring")
		counts[id] = !(hidden && !children[id]) && !driver && !empty
		if !hiddenAbove(id) && !hidden && !children[id] && !driver && !empty && v.To > v.From {
			shown = append(shown, v)
		}
		var own []frameSpan
		switch {
		case l.Expression, driver, l.Type == "compositionReference", isSimulation(l.Type):
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
	for round := 0; round < 8; round++ {
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
