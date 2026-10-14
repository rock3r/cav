package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/config"
)

// seamBoundary is a frame where one section hands over to the next.
type seamBoundary struct {
	Frame    int
	Name     string // the shot ID, or the name of the layer that starts here
	Shot     bool   // Name is a shot, not a layer
	Intended bool   // a hard cut on purpose: reported, never flagged
}

// seamCheck is what cav check measured at one boundary. The three numbers are the changed
// fraction of the picture between frames f-2 and f-1, f-1 and f, and f and f+1.
type seamCheck struct {
	Frame     int     `json:"frame"`
	Name      string  `json:"name"`
	Shot      bool    `json:"shot,omitempty"`
	Intended  bool    `json:"intended,omitempty"`
	Flagged   bool    `json:"flagged"`
	Before    float64 `json:"changedBefore"`
	At        float64 `json:"changed"`
	After     float64 `json:"changedAfter"`
	Box       *[4]int `json:"box,omitempty"`
	BeforePNG string  `json:"before"`
	AfterPNG  string  `json:"after"`
}

// seamSpike says a boundary pops: the picture changes by at least threshold in one frame,
// and by at least twice as much as in the frame pairs on either side. A pan or a cross-fade
// changes about as much in all three, so it is not a spike.
func seamSpike(before, at, after, threshold float64) bool {
	return at >= threshold && at >= 2*max(before, after)
}

// boundaryFrames lists the section boundaries of the active comp, sorted, once each. The
// storyboard shots after the first give them; without a storyboard (or with a single shot),
// the in-frames of top-level groups and pre-comps do. A frame in allowed, or a shot marked
// "cut", is an intended cut. Frames outside (start, end] are dropped.
func boundaryFrames(d structureData, sb *board.Board, allowed map[int]bool) []seamBoundary {
	var out []seamBoundary
	seen := map[int]bool{}
	add := func(f int, name string, shot, cut bool) {
		if f <= d.Start || f > d.End || seen[f] {
			return
		}
		seen[f] = true
		out = append(out, seamBoundary{Frame: f, Name: name, Shot: shot, Intended: cut || allowed[f]})
	}
	if sb != nil && len(sb.Shots) > 1 {
		for _, s := range sb.StartFrames(d.FPS, d.Start) {
			add(s.Frame, s.ID, true, s.Cut)
		}
	} else {
		for _, l := range d.Layers {
			topLevel := l.Parent == "" || l.Parent == d.Comp
			if l.Comp != d.Comp || !topLevel || (l.Type != "group" && l.Type != "compositionReference") || len(l.Boundaries) == 0 {
				continue
			}
			add(l.Boundaries[0], l.Name, false, false)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Frame < out[j].Frame })
	return out
}

// seamFrames are the frames to render for the boundaries: two before each and one after,
// inside the comp.
func seamFrames(d structureData, bs []seamBoundary) []int {
	var frames []int
	for _, b := range bs {
		for _, f := range []int{b.Frame - 2, b.Frame - 1, b.Frame, b.Frame + 1} {
			if f >= d.Start && f <= d.End {
				frames = append(frames, f)
			}
		}
	}
	return sortedFrames(frames)
}

// Rendering needs about this long per frame, plus a fixed part for the scene queries.
const (
	seamFrameCost = 500 * time.Millisecond
	seamBaseCost  = 5 * time.Second
)

// seamsPass renders the frames around each boundary and compares them. It returns the
// flagged boundaries as findings, the intended ones as findings that are only reported,
// every measurement, and the inspections it skipped or that failed.
func (a *app) seamsPass(d structureData, sb *board.Board, allowed map[int]bool, maxSeams int, threshold float64) (flagged, intended []finding, checks []seamCheck, skipped, failures []map[string]any, err error) {
	bs := boundaryFrames(d, sb, allowed)
	if len(bs) == 0 {
		return
	}
	if needsChronological(d) {
		skipped = append(skipped, map[string]any{"inspection": "seams", "reason": "simulation or incomplete coverage; run cav seams --video renders/final.mp4"})
		return
	}
	if len(bs) > maxSeams {
		skipped = append(skipped, map[string]any{"inspection": "seams", "reason": "boundary limit; raise --max-seams", "count": len(bs) - maxSeams})
		bs = bs[:maxSeams]
	}
	samples := seamFrames(d, bs)
	if deadline, ok := a.ctx.Deadline(); ok && time.Until(deadline) < seamBaseCost+time.Duration(len(samples))*seamFrameCost {
		skipped = append(skipped, map[string]any{"inspection": "seams", "reason": "time budget; raise --timeout or lower --max-seams", "count": len(bs)})
		return
	}
	dir := filepath.Join(config.Home(), "operations", "seams-"+a.op.ID)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	paths, err := a.renderSeamFrames(d, samples, 25, dir, 10000)
	if err != nil {
		return
	}
	measure := func(x, y int) (seamResult, error) {
		if x < d.Start || y > d.End {
			return seamResult{}, nil
		}
		return compareSeam(a.ctx, paths[x], paths[y], 8)
	}
	for _, b := range bs {
		var before, at, after seamResult
		if before, err = measure(b.Frame-2, b.Frame-1); err != nil {
			return
		}
		if at, err = measure(b.Frame-1, b.Frame); err != nil {
			return
		}
		if after, err = measure(b.Frame, b.Frame+1); err != nil {
			return
		}
		c := seamCheck{Frame: b.Frame, Name: b.Name, Shot: b.Shot, Intended: b.Intended,
			Before: before.ChangedFraction, At: at.ChangedFraction, After: after.ChangedFraction,
			Box: at.Box, BeforePNG: paths[b.Frame-1], AfterPNG: paths[b.Frame]}
		c.Flagged = !b.Intended && seamSpike(c.Before, c.At, c.After, threshold)
		checks = append(checks, c)
		switch {
		case c.Flagged:
			flagged = append(flagged, seamFinding(c, false))
		case b.Intended && c.At >= threshold:
			intended = append(intended, seamFinding(c, true))
		}
	}
	return
}

// seamFinding describes one boundary. A flagged one is a warning; an intended one is
// only reported.
func seamFinding(c seamCheck, intended bool) finding {
	where := "start of layer " + c.Name
	if c.Shot {
		where = "start of shot " + c.Name
	}
	around := max(c.Before, c.After)
	f := finding{Kind: "seam", Name: c.Name, Severity: "warning",
		Evidence: map[string]any{
			"frame": c.Frame, "changedBefore": c.Before, "changed": c.At, "changedAfter": c.After,
			"box": c.Box, "before": c.BeforePNG, "after": c.AfterPNG,
		}}
	if intended {
		f.Severity = "info"
		f.Detail = fmt.Sprintf("frame %d (%s) is an intended cut: %.0f %% of the picture changes in one frame", c.Frame, where, 100*c.At)
		f.Fix = "none; the cut is marked on purpose"
		return f
	}
	rel := "and nothing changes in the frames around it"
	if around > 0 {
		rel = fmt.Sprintf("%.1fx more than the frames around it", c.At/around)
	}
	f.Detail = fmt.Sprintf("frame %d (%s): %.0f %% of the picture changes in one frame, %s (pop-in, flash or jump)", c.Frame, where, 100*c.At, rel)
	f.Fix = "look at Before/After; fix the element that pops (stagger it in, cross-fade, match position/weight across the hand-off); if this is a hard cut on purpose, set \"cut\": true on the shot in storyboard.json"
	return f
}
