package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rock3r/cav/internal/flow"
	"github.com/rock3r/cav/internal/review"
)

func init() {
	register(command{
		name:    "plan",
		args:    "[--json]   (cav help plan)",
		summary: "Show the project's progress through its flow, and the next step. It never blocks a command.",
		run:     cmdPlan,
	})
	longHelp["plan"] = `
cav plan reads the flow in effect (see cav guide flow) and checks every stage against the
files in the current folder. It shows where the project stands and what to do next. It only
reads: it writes no file, calls no paid service, and blocks no other command.

A stage has one of these states:

  done      its outputs exist and, if it has a review, the review file holds an approval
  review    the outputs exist, but the user has not approved them (or items are marked redo)
  check     cav cannot tell: the review target does not load, or the flow file asks for a
            review that cannot hold an approval. The next step says how to fix it.
  notes     the outputs exist and a review has open notes
  todo      an output is missing
  skipped   no output exists, but a later stage already has some
  unknown   the stage has no outputs: line, so cav cannot check it. It is never done.

The next step is the first stage in the flow that is review, check, notes or todo. Stages
without an outputs: line come after those.

Outputs: a plain path is a file; a path that ends in / is a folder with at least one file
inside; * matches inside one folder.

Use --json for the same facts as data. Exit code 0, except when the flow file does not parse.
`
}

type planOutput struct {
	Path    string   `json:"path"`
	Exists  bool     `json:"exists"`
	Matches []string `json:"matches"`
}

type planReview struct {
	Target          string   `json:"target"`
	ReviewFile      string   `json:"reviewFile"`
	Kind            string   `json:"kind"`
	Approved        bool     `json:"approved"`
	ApprovalUnknown bool     `json:"approvalUnknown"`
	Reason          string   `json:"reason"`
	Items           int      `json:"items"`
	Redo            []string `json:"redo"`
	OpenNotes       []string `json:"openNotes"`
	Cleared         string   `json:"cleared"`
}

type planStage struct {
	N       int          `json:"n"`
	Name    string       `json:"name"`
	Status  string       `json:"status"`
	Detail  string       `json:"detail"`
	Use     string       `json:"use"`
	Outputs []planOutput `json:"outputs"`
	Missing []string     `json:"missing"`
	Review  *planReview  `json:"review"`

	// text and command are the next step when this stage is the next one.
	text, command string
}

type planNext struct {
	Stage   string `json:"stage"`
	Status  string `json:"status"`
	Text    string `json:"text"`
	Command string `json:"command"`
}

func cmdPlan(a *app, args []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageErr("usage: cav plan [--json]")
	}
	root := flowRoot()
	f, err := flow.Load(root)
	if err != nil {
		path, source := flow.Find(root)
		hint := "fix the flow file"
		if path != "" {
			hint = "fix " + path + ", or move it away to use the next flow in line: "
			if source == flow.SourceProject {
				hint += "~/.cav/flow.md, or the standard flow"
			} else {
				hint += "the standard flow"
			}
		}
		return fail(exitError, err.Error(), hint)
	}
	stages, err := planStages(root, f)
	if err != nil {
		return err
	}
	next := planNextStep(stages)
	flowInfo := map[string]any{"source": f.Source, "path": f.Path}
	a.emit(map[string]any{"flow": flowInfo, "stages": stages, "next": next}, func() {
		width := 15
		for _, s := range stages {
			width = max(width, len(s.Name))
		}
		fmt.Printf("%-9s   %s\n", "flow", f.Label())
		for _, s := range stages {
			fmt.Printf("%-9s %d %-*s %s\n", s.Status, s.N, width, s.Name, s.Detail)
		}
		fmt.Println()
		fmt.Printf("%-9s   %s\n", "next", next.Text)
	})
	return nil
}

// planStages checks every stage of the flow against the files in root. It reads files and
// writes none. The paths it reports are relative to root.
func planStages(root string, f *flow.Flow) ([]planStage, error) {
	out := make([]planStage, len(f.Stages))
	for i, s := range f.Stages {
		ps := planStage{N: i + 1, Name: s.Name, Use: s.Use, Outputs: []planOutput{}, Missing: []string{}}
		for _, o := range s.Outputs {
			matches, ok, err := flow.Check(root, o)
			if err != nil {
				return nil, fail(exitError, fmt.Sprintf("stage %s: cannot look for %s: %v", s.Name, o, err), "")
			}
			if matches == nil {
				matches = []string{}
			}
			ps.Outputs = append(ps.Outputs, planOutput{Path: o, Exists: ok, Matches: matches})
			if !ok {
				ps.Missing = append(ps.Missing, o)
			}
		}
		planState(&ps, s, root)
		out[i] = ps
	}
	// A stage with no output at all, while a later stage has one, was left out on purpose.
	for i := range out {
		if out[i].Status != "todo" || len(out[i].Missing) < len(out[i].Outputs) {
			continue
		}
		for _, later := range out[i+1:] {
			if anyOutput(later) {
				out[i].Status, out[i].Detail = "skipped", "no outputs, but a later stage has some"
				break
			}
		}
	}
	for i := range out {
		planText(&out[i], f, root)
	}
	return out, nil
}

func anyOutput(s planStage) bool {
	for _, o := range s.Outputs {
		if o.Exists {
			return true
		}
	}
	return false
}

// planState sets the status and detail of one stage from its outputs and its review.
func planState(ps *planStage, s flow.Stage, root string) {
	switch {
	case len(s.Outputs) == 0:
		ps.Status, ps.Detail = "unknown", "no outputs: line, so cav cannot check it"
		return
	case len(ps.Missing) > 0:
		ps.Status, ps.Detail = "todo", "missing "+strings.Join(ps.Missing, ", ")
		return
	}
	if s.Review {
		target, problem := s.ReviewTargetIn(root)
		if problem != "" {
			ps.Status, ps.Detail = "check", problem
			return
		}
		planReviewed(ps, root, target)
		return
	}
	// A stage without a review still shows notes that someone left on its first output.
	first := s.Outputs[0]
	if !strings.HasSuffix(first, "/") && !strings.Contains(first, "*") {
		if st := review.ReadStatus(filepath.Join(root, filepath.FromSlash(first))); !st.ApprovalUnknown && st.HasReview && len(st.Open) > 0 {
			ps.Review = reviewInfo(root, first, st)
			ps.Status, ps.Detail = "notes", fmt.Sprintf("%s open: %s", count(len(st.Open), "note", "notes"), strings.Join(st.Open, ", "))
			return
		}
	}
	ps.Status, ps.Detail = "done", "outputs found"
}

// planReviewed sets the status of a stage whose first output is a review target.
func planReviewed(ps *planStage, root, target string) {
	st := review.ReadStatus(filepath.Join(root, filepath.FromSlash(target)))
	ps.Review = reviewInfo(root, target, st)
	size := ""
	if st.Items > 0 {
		noun := map[review.Kind][2]string{
			review.KindBrief:      {"field", "fields"},
			review.KindMoodboard:  {"item", "items"},
			review.KindStoryboard: {"shot", "shots"},
		}[st.Kind]
		size = count(st.Items, noun[0], noun[1]) + ", "
	}
	switch {
	case st.ApprovalUnknown:
		ps.Status, ps.Detail = "check", "approval unknown: "+st.Reason
	case st.Approved:
		if size == "" {
			size = target + ", "
		}
		ps.Status, ps.Detail = "done", size+"approved"
	default:
		ps.Status = "review"
		var parts []string
		for _, id := range st.Redo {
			parts = append(parts, id+" marked redo")
		}
		if len(st.Open) > 0 {
			parts = append(parts, count(len(st.Open), "open note", "open notes"))
		}
		switch {
		case st.Cleared != "":
			parts = append(parts, "approval removed ("+st.Cleared+")")
		case len(parts) == 0:
			parts = append(parts, "not approved yet")
		}
		ps.Detail = size + strings.Join(parts, ", ")
	}
}

func reviewInfo(root, target string, st review.Status) *planReview {
	reviewFile := st.ReviewFile
	if rel, err := filepath.Rel(root, reviewFile); reviewFile != "" && err == nil && !strings.HasPrefix(rel, "..") {
		reviewFile = rel
	}
	return &planReview{Target: target, ReviewFile: filepath.ToSlash(reviewFile), Kind: string(st.Kind),
		Approved: st.Approved, ApprovalUnknown: st.ApprovalUnknown, Reason: st.Reason, Items: st.Items,
		Redo: st.Redo, OpenNotes: st.Open, Cleared: st.Cleared}
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// fromCwd turns a path relative to the project root into one relative to the working
// folder, in slash form, for a command the user runs from there. Display and JSON keep the
// root-relative path.
func fromCwd(root, target string) string {
	abs, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(target)))
	if err != nil {
		return target
	}
	wd, err := os.Getwd()
	if err != nil {
		return target
	}
	rel, err := filepath.Rel(wd, abs)
	if err != nil {
		return target
	}
	return filepath.ToSlash(rel)
}

// planText writes the next step for a stage, used when it is the first that needs work.
func planText(ps *planStage, f *flow.Flow, root string) {
	switch ps.Status {
	case "todo":
		switch {
		case ps.Use != "":
			ps.text = fmt.Sprintf("%s: %s (missing %s)", ps.Name, ps.Use, strings.Join(ps.Missing, ", "))
		default:
			ps.text = fmt.Sprintf("%s: make %s", ps.Name, strings.Join(ps.Missing, ", "))
		}
	case "review":
		r := ps.Review
		cmd := "cav review " + shellArg(fromCwd(root, r.Target))
		ps.command = cmd
		if fix := append(append([]string{}, r.Redo...), r.OpenNotes...); len(fix) > 0 {
			ps.text = fmt.Sprintf("fix %s, then: %s", strings.Join(fix, ", "), cmd)
			break
		}
		ps.text = "ask the user to approve: " + cmd
		if r.Cleared != "" {
			ps.text = fmt.Sprintf("the approval was removed (%s); %s", r.Cleared, ps.text)
		}
	case "check":
		if ps.Review != nil {
			ps.command = "cav review export " + shellArg(fromCwd(root, ps.Review.Target))
			ps.text = fmt.Sprintf("fix %s (%s), then: %s", ps.Review.Target, ps.Review.Reason, ps.command)
			break
		}
		where := "your flow file"
		if f.Path != "" {
			where = f.Path
		}
		ps.text = fmt.Sprintf("%s: %s; fix the flow file %s", ps.Name, ps.Detail, where)
	case "notes":
		ps.command = "cav review export " + shellArg(fromCwd(root, ps.Review.Target))
		ps.text = "read the notes: " + ps.command
	case "unknown":
		step := "ask the user"
		if ps.Use != "" {
			step = ps.Use
		}
		ps.text = fmt.Sprintf("%s: cav cannot check this stage (no outputs:); %s", ps.Name, step)
	}
}

// planNextStep picks the first stage that needs work, then the first unknown one.
func planNextStep(stages []planStage) planNext {
	for _, s := range stages {
		switch s.Status {
		case "review", "check", "notes", "todo":
			return planNext{Stage: s.Name, Status: s.Status, Text: s.text, Command: s.command}
		}
	}
	for _, s := range stages {
		if s.Status == "unknown" {
			return planNext{Stage: s.Name, Status: s.Status, Text: s.text, Command: s.command}
		}
	}
	return planNext{Text: "all stages are done"}
}
