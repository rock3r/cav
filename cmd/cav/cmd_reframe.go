package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rock3r/cav/assets"
)

// parseAspect reads a target shape such as 9:16 or 9x16. Only the shapes reframe supports are
// accepted: 9:16 (vertical video) and 1:1 (square).
func parseAspect(s string) (w, h int, err error) {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(s)), func(r rune) bool { return r == ':' || r == 'x' })
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("--to must look like 9:16 or 1:1, got %q", s)
	}
	w, e1 := strconv.Atoi(parts[0])
	h, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil {
		return 0, 0, fmt.Errorf("--to must look like 9:16 or 1:1, got %q", s)
	}
	if (w == 9 && h == 16) || (w == 1 && h == 1) {
		return w, h, nil
	}
	return 0, 0, fmt.Errorf("--to supports 9:16 and 1:1 only, got %d:%d", w, h)
}

type reframeFinding struct {
	Kind   string `json:"kind"`
	Layer  string `json:"layer,omitempty"`
	Name   string `json:"name,omitempty"`
	Frame  *int   `json:"frame,omitempty"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

type reframeComp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

type reframeResult struct {
	Source reframeComp `json:"source"`
	Target reframeComp `json:"target"`
	Key    struct {
		Mode   string `json:"mode"`
		Layers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"layers"`
	} `json:"key"`
	Scale   float64    `json:"scale"`
	Offset  [2]float64 `json:"offset"`
	Cover   float64    `json:"cover"`
	Wrapped []struct {
		Name string `json:"name"`
	} `json:"wrapped"`
	Backgrounds []struct {
		Name string `json:"name"`
	} `json:"backgrounds"`
	Findings         []reframeFinding `json:"findings"`
	CoverageComplete bool             `json:"coverageComplete"`
	Skipped          int              `json:"skipped"`
	Warnings         []string         `json:"warnings"`
	DryRun           bool             `json:"dryRun"`
}

// reframeOpts is the input of the native job (assets/reframe/reframe.js).
type reframeOpts struct {
	Source   string   `json:"source"`
	To       [2]int   `json:"to"`
	Name     string   `json:"name"`
	Key      []string `json:"key"`
	Scale    float64  `json:"scale"`
	Samples  int      `json:"samples"`
	MinText  float64  `json:"minText"`
	DryRun   bool     `json:"dryRun"`
	BudgetMS int64    `json:"budgetMs"`
}

func sceneReframe(a *app, args []string) error {
	fs := flag.NewFlagSet("scene reframe", flag.ContinueOnError)
	to := fs.String("to", "", "target shape: 9:16 or 1:1")
	comp := fs.String("comp", "", "source composition ID or unique name (default: the active one)")
	name := fs.String("name", "", "name of the new comp (default: <source> 9x16 or 1x1)")
	key := fs.String("key", "", "comma-separated layer names or IDs to fit into the middle half (default: the rig group)")
	scale := fs.Float64("scale", 0, "scale of the content (default: the largest that fits, never above 1)")
	samples := fs.Int("samples", 12, "frames to measure (1..24)")
	minText := fs.Float64("min-text", 28, "smallest readable text size in the new comp")
	dry := fs.Bool("dry-run", false, "print the plan and the predicted findings; change nothing")
	timeout := fs.Duration("timeout", 2*time.Minute, "maximum wait for Cavalry")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageErr("usage: cav scene reframe --to 9:16|1:1 [options]")
	}
	if *to == "" {
		return usageErr("--to is required: cav scene reframe --to 9:16 (or 1:1)")
	}
	w, h, err := parseAspect(*to)
	if err != nil {
		return usageErr("%s", err)
	}
	scaleSet := false
	fs.Visit(func(f *flag.Flag) { scaleSet = scaleSet || f.Name == "scale" })
	if *samples < 1 || *samples > 24 || *minText <= 0 || *timeout <= 0 || (scaleSet && *scale <= 0) {
		return usageErr("invalid reframe limits: --samples 1..24, --scale and --min-text above 0")
	}
	in, err := json.Marshal(reframeOpts{Source: *comp, To: [2]int{w, h}, Name: *name, Key: splitList(*key), Scale: *scale,
		Samples: *samples, MinText: *minText, DryRun: *dry, BudgetMS: (*timeout / 2).Milliseconds()})
	if err != nil {
		return err
	}
	// One job measures the original and builds the copy, so nothing changes in between.
	o, err := a.execJS("var opts = "+string(in)+";\n"+assets.Reframe, execOpts{helpers: true, timeout: *timeout, source: "cav scene reframe"})
	if err != nil {
		return err
	}
	if !o.result.OK {
		return scriptErr(o)
	}
	var r reframeResult
	if err := json.Unmarshal(o.result.Value, &r); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(o.result.Value, &raw); err != nil {
		return err
	}
	a.emit(raw, func() { printReframe(&r) })
	return nil
}

func printReframe(r *reframeResult) {
	verb := "made"
	if r.DryRun {
		verb = "would make"
	}
	fmt.Printf("%s %dx%d: %s %q at %dx%d", r.Source.Name, r.Source.W, r.Source.H, verb, r.Target.Name, r.Target.W, r.Target.H)
	if !r.DryRun {
		fmt.Print(", now the active comp")
	}
	fmt.Println()
	var keys []string
	for _, k := range r.Key.Layers {
		keys = append(keys, k.Name)
	}
	fmt.Printf("fit: %s into the middle half, scale %.3f, offset (%.0f, %.0f)\n", strings.Join(keys, ", "), r.Scale, r.Offset[0], r.Offset[1])
	if len(r.Backgrounds) > 0 {
		var bg []string
		for _, b := range r.Backgrounds {
			bg = append(bg, b.Name)
		}
		fmt.Printf("backgrounds: %s, scaled x%.2f to cover the frame\n", strings.Join(bg, ", "), r.Cover)
	}
	if len(r.Wrapped) > 0 {
		var ws []string
		for _, w := range r.Wrapped {
			ws = append(ws, w.Name)
		}
		fmt.Printf("keyed layers kept as they are, inside a fit group: %s\n", strings.Join(ws, ", "))
	}
	for _, f := range r.Findings {
		where := f.Name
		if where == "" {
			where = "note"
		}
		fmt.Printf("- %s: %s: %s\n", f.Kind, where, f.Detail)
	}
	if !r.CoverageComplete {
		fmt.Fprintf(os.Stderr, "note: not every layer or frame was measured (%d layers skipped), so the findings may be incomplete\n", r.Skipped)
	}
	for _, w := range r.Warnings {
		fmt.Fprintln(os.Stderr, "note: "+w)
	}
	if r.DryRun {
		fmt.Println("\nNothing was changed. Run the same command without --dry-run to make the copy.")
		return
	}
	fmt.Printf("\nNext:\n  cav sheet --comp %q            look at it\n  cav check --comp %q            confirm the findings\n", r.Target.Name, r.Target.Name)
	fmt.Println("  Move titles and captions into the free bands above and below the middle half (cav.safeArea()),")
	fmt.Println("  raise small text, retime if needed, then cav scene save.")
	fmt.Println("The original comp was not changed.")
}
