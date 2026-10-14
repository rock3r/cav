package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/review"
)

// planOut is the part of `cav plan --json` that the tests read.
type planOut struct {
	OK   bool `json:"ok"`
	Flow struct {
		Source string `json:"source"`
		Path   string `json:"path"`
	} `json:"flow"`
	Stages []struct {
		N      int    `json:"n"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail"`
		Review *struct {
			Target          string   `json:"target"`
			Approved        bool     `json:"approved"`
			ApprovalUnknown bool     `json:"approvalUnknown"`
			Redo            []string `json:"redo"`
			Cleared         string   `json:"cleared"`
		} `json:"review"`
	} `json:"stages"`
	Next planNext `json:"next"`
}

// runPlan runs cav plan --json in the current folder.
func runPlan(t *testing.T) planOut {
	t.Helper()
	code, out := cav(t, true, "plan")
	if code != 0 {
		t.Fatalf("cav plan exited %d: %s", code, out)
	}
	var p planOut
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return p
}

func (p planOut) status(t *testing.T, name string) string {
	t.Helper()
	for _, s := range p.Stages {
		if s.Name == name {
			return s.Status
		}
	}
	t.Fatalf("no stage %s", name)
	return ""
}

// planDir makes an empty project folder and works in it. The cav home folder is empty.
func planDir(t *testing.T) string {
	t.Helper()
	t.Setenv("CAV_HOME", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func writePlanFile(t *testing.T, rel, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rel, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const storyboardFlow = "## storyboard\noutputs: storyboard.json, board/\nreview: yes\nuse: cav board init\n"

// inStoryboardProject works in a folder with a storyboard of two shots and a flow that has
// only that stage.
func inStoryboardProject(t *testing.T) string {
	t.Helper()
	path := stillsProject(t)
	t.Chdir(filepath.Dir(path))
	writePlanFile(t, "flow.md", storyboardFlow)
	return "storyboard.json"
}

func TestPlanEmptyFolder(t *testing.T) {
	planDir(t)
	p := runPlan(t)
	if p.Flow.Source != "standard" || len(p.Stages) != 5 {
		t.Fatalf("flow = %+v, %d stages", p.Flow, len(p.Stages))
	}
	for _, s := range p.Stages {
		if s.Status != "todo" {
			t.Errorf("stage %s is %s, want todo", s.Name, s.Status)
		}
	}
	if p.Next.Stage != "brief" || p.Next.Status != "todo" || !strings.Contains(p.Next.Text, "cav brief init") {
		t.Errorf("next = %+v", p.Next)
	}
	code, out := cav(t, false, "plan")
	if code != 0 || !strings.Contains(out, "standard (built in)") || !strings.Contains(out, "next") {
		t.Errorf("human output (exit %d): %s", code, out)
	}
}

func TestPlanApprovedStoryboardIsDone(t *testing.T) {
	target := inStoryboardProject(t)
	approveAll(t, target)
	p := runPlan(t)
	s := p.Stages[0]
	if s.Status != "done" || s.Detail != "2 shots, approved" || s.Review == nil || !s.Review.Approved {
		t.Errorf("stage = %+v", s)
	}
	if p.Next.Text != "all stages are done" || p.Next.Stage != "" {
		t.Errorf("next = %+v", p.Next)
	}
}

func TestPlanOutputsWithoutApprovalAreNotDone(t *testing.T) {
	inStoryboardProject(t)
	p := runPlan(t)
	if p.Stages[0].Status != "review" {
		t.Fatalf("status = %s", p.Stages[0].Status)
	}
	if p.Next.Text != "ask the user to approve: cav review "+shellArg("storyboard.json") || p.Next.Command != "cav review "+shellArg("storyboard.json") {
		t.Errorf("next = %+v", p.Next)
	}
}

func TestPlanRedoItem(t *testing.T) {
	target := inStoryboardProject(t)
	sha := itemSHA(t, target, "s1")
	_, err := review.Open(target).Update(func(d *review.Doc) error {
		d.Kind = "storyboard"
		d.Comments = append(d.Comments, review.Comment{ID: "c_01", Item: "s1", ItemSHA: sha, Verdict: "redo", Status: "open", CreatedAt: time.Now()})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p := runPlan(t)
	if p.Stages[0].Status != "review" || !reflect.DeepEqual(p.Stages[0].Review.Redo, []string{"s1"}) {
		t.Errorf("stage = %+v", p.Stages[0])
	}
	if p.Next.Text != "fix s1, then: cav review "+shellArg("storyboard.json") {
		t.Errorf("next = %q", p.Next.Text)
	}
	if !strings.Contains(p.Stages[0].Detail, "s1 marked redo") {
		t.Errorf("detail = %q", p.Stages[0].Detail)
	}
}

func TestPlanApprovalRemovedByAnEdit(t *testing.T) {
	target := inStoryboardProject(t)
	approveAll(t, target)
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(b), "tagline", "new tagline", 1)
	if edited == string(b) {
		t.Fatal("the test did not edit the storyboard")
	}
	writePlanFile(t, target, edited)
	reviewFile := review.Open(target).DocPath
	before, _ := os.ReadFile(reviewFile)
	p := runPlan(t)
	if p.Stages[0].Status != "review" || p.Stages[0].Review.Cleared != "s2 changed" {
		t.Errorf("stage = %+v", p.Stages[0])
	}
	if !strings.HasPrefix(p.Next.Text, "the approval was removed (s2 changed); ") {
		t.Errorf("next = %q", p.Next.Text)
	}
	after, _ := os.ReadFile(reviewFile)
	if string(before) != string(after) {
		t.Error("cav plan changed the review file")
	}
}

func TestPlanSkippedStage(t *testing.T) {
	planDir(t)
	writePlanFile(t, filepath.Join("renders", "final.mp4"), "video")
	writePlanFile(t, filepath.Join("scenes", "a.cv"), "scene")
	p := runPlan(t)
	if got := p.status(t, "storyboard"); got != "skipped" {
		t.Errorf("storyboard is %s, want skipped", got)
	}
	if got := p.status(t, "build"); got != "done" {
		t.Errorf("build is %s, want done", got)
	}
	for _, name := range []string{"brief", "mood board", "music and animatic"} {
		if got := p.status(t, name); got != "skipped" {
			t.Errorf("%s is %s, want skipped", name, got)
		}
	}
	if p.Next.Text != "all stages are done" {
		t.Errorf("next = %+v", p.Next)
	}
}

func TestPlanStageWithoutOutputsIsUnknown(t *testing.T) {
	planDir(t)
	writePlanFile(t, "flow.md", "## polish\nAsk the user.\nuse: ask what is left\n")
	p := runPlan(t)
	if p.Stages[0].Status != "unknown" {
		t.Fatalf("status = %s", p.Stages[0].Status)
	}
	if p.Next.Status != "unknown" || !strings.Contains(p.Next.Text, "cav cannot check this stage") || p.Next.Text == "all stages are done" {
		t.Errorf("next = %+v", p.Next)
	}
}

func TestPlanUnknownComesAfterWorkToDo(t *testing.T) {
	planDir(t)
	writePlanFile(t, "flow.md", "## polish\nuse: ask\n\n## draft\noutputs: draft.txt\n")
	p := runPlan(t)
	if p.Next.Stage != "draft" || p.Next.Status != "todo" {
		t.Errorf("next = %+v", p.Next)
	}
}

func TestPlanReviewOfAPatternNeedsAFlowFix(t *testing.T) {
	planDir(t)
	writePlanFile(t, "flow.md", "## styles\noutputs: style/*.png\nreview: yes\n")
	writePlanFile(t, filepath.Join("style", "a.png"), "x")
	p := runPlan(t)
	if p.Stages[0].Status != "check" || !strings.Contains(p.Stages[0].Detail, "is a pattern") {
		t.Fatalf("stage = %+v", p.Stages[0])
	}
	if !strings.Contains(p.Next.Text, "fix the flow file") || !strings.Contains(p.Next.Text, "flow.md") {
		t.Errorf("next = %q", p.Next.Text)
	}
}

func TestPlanTargetThatDoesNotLoad(t *testing.T) {
	target := inStoryboardProject(t)
	approveAll(t, target)
	writePlanFile(t, target, "{ not json")
	p := runPlan(t)
	s := p.Stages[0]
	if s.Status != "check" || s.Review == nil || !s.Review.ApprovalUnknown || s.Review.Approved {
		t.Fatalf("stage = %+v", s)
	}
	if !strings.Contains(p.Next.Text, "cav review export "+shellArg("storyboard.json")) {
		t.Errorf("next = %q", p.Next.Text)
	}
}

func TestPlanNotesOnAStageWithoutReview(t *testing.T) {
	planDir(t)
	writePlanFile(t, "flow.md", "## build\noutputs: renders/final.mp4\nreview: no\n")
	writePlanFile(t, filepath.Join("renders", "final.mp4"), "video")
	_, err := review.Open(filepath.Join("renders", "final.mp4")).Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments, review.Comment{ID: "c_01", Text: "logo is late", Status: "open", CreatedAt: time.Now()})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p := runPlan(t)
	if p.Stages[0].Status != "notes" || p.Next.Text != "read the notes: cav review export "+shellArg("renders/final.mp4") {
		t.Errorf("stage = %+v, next = %+v", p.Stages[0], p.Next)
	}
}

func TestPlanFlowFileThatDoesNotParse(t *testing.T) {
	planDir(t)
	writePlanFile(t, "flow.md", "## a\nreview: maybe\n")
	code, _ := cav(t, true, "plan")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if _, err := os.Stat("flow.md"); err != nil {
		t.Error(err)
	}
}

func TestPlanJSONShape(t *testing.T) {
	planDir(t)
	_, out := cav(t, true, "plan")
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	stage := raw["stages"].([]any)[0].(map[string]any)
	for _, k := range []string{"n", "name", "status", "detail", "use", "outputs", "missing", "review"} {
		if _, ok := stage[k]; !ok {
			t.Errorf("stage has no %q", k)
		}
	}
	if _, ok := stage["missing"].([]any); !ok {
		t.Errorf("missing = %#v, want a list", stage["missing"])
	}
	o := stage["outputs"].([]any)[0].(map[string]any)
	if _, ok := o["matches"].([]any); !ok || o["exists"] != false {
		t.Errorf("output = %#v", o)
	}
	next := raw["next"].(map[string]any)
	for _, k := range []string{"stage", "status", "text", "command"} {
		if _, ok := next[k]; !ok {
			t.Errorf("next has no %q", k)
		}
	}
}

// snapshot lists every path in dir with its size and time.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[p] = info.ModTime().String() + " " + info.Mode().String() + " " + strconv.FormatInt(info.Size(), 10)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPlanNeverWrites(t *testing.T) {
	target := inStoryboardProject(t)
	approveAll(t, target)
	// An edit after the approval is the case in which cav review export would write.
	b, _ := os.ReadFile(target)
	writePlanFile(t, target, strings.Replace(string(b), "logo", "big logo", 1))
	writePlanFile(t, filepath.Join("renders", "final.mp4"), "video")
	dir, _ := os.Getwd()
	home := os.Getenv("CAV_HOME")
	before, beforeHome := snapshot(t, dir), snapshot(t, home)
	time.Sleep(20 * time.Millisecond)
	for _, json := range []bool{false, true} {
		if code, _ := cav(t, json, "plan"); code != 0 {
			t.Fatalf("exit %d", code)
		}
	}
	if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("the project changed:\n%v\n%v", before, after)
	}
	if after := snapshot(t, home); !reflect.DeepEqual(beforeHome, after) {
		t.Errorf("the cav home folder changed")
	}
}

func TestPlanQuotesReviewTargets(t *testing.T) {
	path := stillsProject(t)
	dir := filepath.Dir(path)
	if err := os.Rename(path, filepath.Join(dir, "my board.json")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	writePlanFile(t, "flow.md", "## storyboard\noutputs: my board.json, board/\nreview: yes\n")
	p := runPlan(t)
	if p.Stages[0].Status != "review" {
		t.Fatalf("stage = %+v", p.Stages[0])
	}
	if p.Next.Command != "cav review "+shellArg("my board.json") || !strings.HasSuffix(p.Next.Text, "cav review "+shellArg("my board.json")) {
		t.Errorf("next = %+v", p.Next)
	}
	writePlanFile(t, "my board.json", `{"schema":"cav-board/1","shots":"x"}`)
	p = runPlan(t)
	if p.Next.Command != "cav review export "+shellArg("my board.json") {
		t.Errorf("next = %+v", p.Next)
	}
}

func TestPlanUsesTheProjectRootFromAFolder(t *testing.T) {
	path := stillsProject(t)
	root := filepath.Dir(path)
	t.Chdir(root)
	if err := os.MkdirAll(".cav", 0o755); err != nil {
		t.Fatal(err)
	}
	writePlanFile(t, "flow.md", storyboardFlow)
	sub := filepath.Join(root, "work", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	p := runPlan(t)
	if p.Flow.Source != "project" || len(p.Stages) != 1 {
		t.Fatalf("flow = %+v, %d stages", p.Flow, len(p.Stages))
	}
	s := p.Stages[0]
	if s.Status != "review" || s.Review == nil || s.Review.Target != "storyboard.json" {
		t.Errorf("stage = %+v", s)
	}
	// The command runs from the working folder, so its path is relative to that folder.
	want := "cav review " + shellArg("../../storyboard.json")
	if p.Next.Command != want {
		t.Errorf("command = %q, want %q", p.Next.Command, want)
	}

	approveAll(t, filepath.Join(root, "storyboard.json"))
	s = runPlan(t).Stages[0]
	if s.Status != "done" || !s.Review.Approved {
		t.Errorf("after approval: stage = %+v", s)
	}
}
