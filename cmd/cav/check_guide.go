package main

import (
	"strings"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/guide"
)

// checkGuide points cav check finding kinds at the guide sections that explain them: the
// topic, then a prefix of each heading (and of the table row's first cell for a trap).
// cav check prints these sections with its findings, so the agent does not have to search.
var checkGuide = map[string][][]string{
	"offframe": {
		{"traps", "Layers and transforms", "Y points up"},
		{"traps", "Layers and transforms", "api.parent(child, parent)"},
	},
	"hidden":                {{"traps", "Layers and transforms", "api.reorder(a, b)"}},
	"font":                  {{"traps", "Look and render", "Missing font"}},
	"sksl-input-redeclared": {{"traps", "Launch-video observations", "Redeclaring a SkSL input uniform"}},
	"group-filter":          {{"traps", "Launch-video observations", "A filter attached to a group"}},
	"shot-why":              {{"ui-showcase", "2. Story and message"}},
	"perf-sampled-keys":     {{"native", "Dense animation curves"}},
	"perf-keyframe-volume":  {{"native", "Dense animation curves"}},
	"perf-copy-javascript":  {{"native", "Per-copy JavaScript"}},
	"perf-simulation":       {{"native", "Physics and particles"}},
}

// Size limits for the guide sections cav check adds: one section, and all of them.
const (
	checkGuideHitBytes   = 1500
	checkGuideTotalBytes = 4000
)

// guideForFindings sets each finding's Guide to the paths of its sections and returns
// those sections once each, in the order the findings first need them.
func guideForFindings(out []finding) []guide.Hit {
	var ix *guide.Index
	var hits []guide.Hit
	seen := map[string]bool{}
	for i := range out {
		refs := checkGuide[out[i].Kind]
		if len(refs) == 0 {
			continue
		}
		if ix == nil {
			var err error
			if ix, err = guide.Load(assets.Guide, "guide", guideTopics); err != nil {
				return nil
			}
		}
		for _, ref := range refs {
			for _, s := range ix.Find(ref[0], ref[1:]...) {
				path := strings.Join(append([]string{s.Topic}, s.Path...), " › ")
				out[i].Guide = append(out[i].Guide, path)
				if !seen[path] {
					seen[path] = true
					hits = append(hits, guide.Hit{Section: s})
				}
			}
		}
	}
	hits, _ = guide.Cap(hits, checkGuideHitBytes, checkGuideTotalBytes)
	return hits
}
