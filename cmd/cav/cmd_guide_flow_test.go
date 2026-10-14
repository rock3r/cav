package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/flow"
	"github.com/rock3r/cav/internal/guide"
	"github.com/rock3r/cav/internal/operation"
)

// captureStreams runs f and returns what it wrote to os.Stdout and os.Stderr.
func captureStreams(t *testing.T, f func()) (stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Drain both pipes while f runs: a full pipe would block f (Windows pipes are small).
	read := func(r *os.File) <-chan string {
		c := make(chan string, 1)
		go func() { b, _ := io.ReadAll(r); c <- string(b) }()
		return c
	}
	o, e := read(outR), read(errR)
	os.Stdout, os.Stderr = outW, errW
	f()
	outW.Close()
	errW.Close()
	return <-o, <-e
}

func TestGuideWorkflowIsAWarningAliasForScene(t *testing.T) {
	out, errOut := captureStreams(t, func() {
		if err := cmdGuide(&app{}, []string{"workflow"}); err != nil {
			t.Error(err)
		}
	})
	scene, sceneErr := captureStreams(t, func() {
		if err := cmdGuide(&app{}, []string{"scene"}); err != nil {
			t.Error(err)
		}
	})
	want := "To build a scene in Cavalry, use `cav guide scene`. For the production stages from brief to animatic, use `cav guide flow`.\n"
	if errOut != want {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
	if sceneErr != "" {
		t.Errorf("cav guide scene wrote to stderr: %q", sceneErr)
	}
	if out == "" || out != scene {
		t.Errorf("cav guide workflow did not print the scene guide on stdout (%d bytes, scene %d)", len(out), len(scene))
	}
	if !strings.Contains(out, "## 2. The build loop") {
		t.Error("the printed guide is not the scene guide")
	}
}

func TestGuideWithoutTopicPrintsSceneWithoutWarning(t *testing.T) {
	out, errOut := captureStreams(t, func() { _ = cmdGuide(&app{}, nil) })
	if errOut != "" || !strings.HasPrefix(out, "# Making motion graphics with cav") {
		t.Errorf("stdout has %d bytes, stderr %q", len(out), errOut)
	}
}

func TestGuideSearchDoesNotReturnTheWorkflowAlias(t *testing.T) {
	for _, topic := range guideTopics {
		if topic == "workflow" {
			t.Fatal("workflow is listed as a topic")
		}
	}
	a := &app{op: &operation.Record{}}
	if err := cmdGuide(a, []string{"--search", "workflow", "-n", "50"}); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Results []guide.Hit `json:"results"`
	}
	if err := json.Unmarshal(a.op.Data, &out); err != nil {
		t.Fatal(err)
	}
	for _, h := range out.Results {
		if h.Topic == "workflow" {
			t.Errorf("search returned the alias as a topic: %+v", h.Path)
		}
	}
}

// Every `cav <command>` that the flow guide names must be a real command, and a plain
// second word must be one of its subcommands.
func TestFlowGuideNamesOnlyExistingCommands(t *testing.T) {
	b, err := assets.Guide.ReadFile("guide/flow.md")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]command{}
	for _, c := range commands {
		byName[c.name] = c
	}
	word := regexp.MustCompile(`^[a-z]+$`)
	check := func(where, ref string) {
		t.Helper()
		f := strings.Fields(ref)
		c, ok := byName[f[1]]
		if !ok {
			t.Errorf("%s: %q: cav has no command %q", where, ref, f[1])
			return
		}
		if len(f) > 2 && word.MatchString(f[2]) && !strings.Contains(c.args, f[2]) && !strings.Contains(longHelp[c.name], f[2]) {
			t.Errorf("%s: %q: cav %s has no subcommand %q", where, ref, c.name, f[2])
		}
	}
	span := regexp.MustCompile("`(cav [^`]+)`")
	seen := 0
	for where, text := range map[string]string{"flow.md": string(b), "standard.md": string(assets.StandardFlow)} {
		for _, m := range span.FindAllStringSubmatch(text, -1) {
			if len(strings.Fields(m[1])) >= 2 {
				seen++
				check(where, m[1])
			}
		}
	}
	// use: lines are plain text, so find commands without backticks.
	plain := regexp.MustCompile(`cav [a-z]+( [a-z]+)?`)
	for _, s := range flow.Standard().Stages {
		for _, m := range plain.FindAllString(s.Use, -1) {
			seen++
			check("use: of "+s.Name, m)
		}
	}
	if seen < 30 {
		t.Errorf("found only %d commands in flow.md and standard.md; the parser is broken", seen)
	}
}

// flowEnv isolates a test from the real ~/.cav/flow.md and from the repository folder.
func flowEnv(t *testing.T) (project, home string) {
	t.Helper()
	project, home = t.TempDir(), t.TempDir()
	t.Chdir(project)
	t.Setenv("CAV_HOME", home)
	return project, home
}

func flowGuide(t *testing.T, jsonOut bool) (stdout, stderr string, data map[string]any) {
	t.Helper()
	a := &app{json: jsonOut}
	stdout, stderr = captureStreams(t, func() {
		if err := cmdGuide(a, []string{"flow"}); err != nil {
			t.Error(err)
		}
	})
	if jsonOut {
		if err := json.Unmarshal([]byte(stdout), &data); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, stdout)
		}
	}
	return stdout, stderr, data
}

func TestGuideFlowPrintsTheStandardFlow(t *testing.T) {
	flowEnv(t)
	out, errOut, _ := flowGuide(t, false)
	if errOut != "" {
		t.Errorf("stderr = %q", errOut)
	}
	if !strings.HasPrefix(out, "# Production flow") {
		t.Error("the rules are missing")
	}
	header := "## The flow in effect: standard (built in)\n"
	i := strings.Index(out, header)
	if i < 0 {
		t.Fatalf("no header %q", header)
	}
	if !strings.HasSuffix(out, strings.TrimRight(string(assets.StandardFlow), "\n")+"\n") {
		t.Error("the standard flow is not printed verbatim at the end")
	}
	if strings.Contains(out[i:], "cav reads its stages from") {
		t.Error("the standard flow has no file to name")
	}
}

func TestGuideFlowPrintsTheProjectFlowVerbatim(t *testing.T) {
	project, _ := flowEnv(t)
	body := "# Mine\n\nIntro for the agent.\n\n## style frames\nMake three frames.\noutputs: style/*.png, style/\nreview: no\nuse: cav gen image\n"
	if err := os.WriteFile(filepath.Join(project, "flow.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, _ := flowGuide(t, false)
	if errOut != "" {
		t.Errorf("stderr = %q", errOut)
	}
	if !strings.Contains(out, "## The flow in effect: flow.md in this project\n\ncav reads its stages from flow.md.\n\n"+body) {
		t.Errorf("project flow not printed verbatim under its header:\n%s", out[strings.Index(out, "## The flow in effect"):])
	}
	if strings.Contains(out, "## brief") {
		t.Error("the standard stages are printed as well")
	}
}

func TestGuideFlowFallsBackToTheHomeFlow(t *testing.T) {
	_, home := flowEnv(t)
	if err := os.WriteFile(filepath.Join(home, "flow.md"), []byte("## only\noutputs: a.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, data := flowGuide(t, true)
	text, _ := data["text"].(string)
	if !strings.Contains(text, "## The flow in effect: "+filepath.Join(home, "flow.md")) {
		t.Errorf("header does not name the home file:\n%s", out)
	}
	if got := data["flow"].(map[string]any)["source"]; got != "user" {
		t.Errorf("flow.source = %v", got)
	}
}

func TestGuideFlowBrokenFileStillPrintsWithAWarning(t *testing.T) {
	project, home := flowEnv(t)
	if err := os.WriteFile(filepath.Join(home, "flow.md"), []byte("## fine\noutputs: a.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "## a\noutputs: a.json\nreview: maybe\n"
	if err := os.WriteFile(filepath.Join(project, "flow.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, _ := flowGuide(t, false)
	if !strings.Contains(errOut, "warning:") || !strings.Contains(errOut, "line 3") || !strings.Contains(errOut, "review: must be yes or no") {
		t.Errorf("stderr = %q", errOut)
	}
	if !strings.Contains(out, "## The flow in effect: flow.md in this project\n") || !strings.HasSuffix(out, body) {
		t.Errorf("the broken file is not printed:\n%s", out)
	}
	if strings.Contains(out, "## fine") {
		t.Error("fell back to the home flow")
	}
}

func TestGuideFlowJSONHasTheSource(t *testing.T) {
	flowEnv(t)
	_, _, data := flowGuide(t, true)
	info, ok := data["flow"].(map[string]any)
	if !ok {
		t.Fatalf("no flow in %v", data)
	}
	if info["source"] != "standard" || info["path"] != "" {
		t.Errorf("flow = %v", info)
	}
	if stages, _ := info["stages"].([]any); len(stages) != 5 {
		t.Errorf("stages = %v", info["stages"])
	}
}

func TestGuideSearchFindsTheStandardFlow(t *testing.T) {
	flowEnv(t)
	a := &app{op: &operation.Record{}}
	if err := cmdGuide(a, []string{"--search", "animatic reviews timing music", "-n", "20"}); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Results []guide.Hit `json:"results"`
	}
	if err := json.Unmarshal(a.op.Data, &out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range out.Results {
		found = found || (h.Topic == "flow" && len(h.Path) > 0 && h.Path[len(h.Path)-1] == "music and animatic")
	}
	if !found {
		t.Errorf("search did not return the standard flow's animatic stage: %+v", out.Results)
	}
}

func TestGuideFlowWarnsAboutReviewsThatCannotBeApproved(t *testing.T) {
	project, _ := flowEnv(t)
	body := "## style frames\noutputs: style/*.png\nreview: yes\n"
	if err := os.WriteFile(filepath.Join(project, "flow.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	want := "flow.md line 1: stage style frames: style/*.png is a pattern; list the .json file to review first, or set review: no"
	_, errOut, _ := flowGuide(t, false)
	if !strings.Contains(errOut, "warning: "+want+"\n") {
		t.Errorf("stderr = %q", errOut)
	}
	_, _, data := flowGuide(t, true)
	info := data["flow"].(map[string]any)
	warns, _ := info["warnings"].([]any)
	if len(warns) != 1 || warns[0] != want || info["valid"] != true {
		t.Errorf("flow = %v", info)
	}
}

func TestGuideFlowFindsTheProjectRootFromASubfolder(t *testing.T) {
	root, _ := flowEnv(t)
	if err := os.Mkdir(filepath.Join(root, ".cav"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "## style\noutputs: style.json\nreview: yes\n"
	if err := os.WriteFile(filepath.Join(root, "flow.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// A schema in the project root makes style.json reviewable; from a subfolder, the check
	// must look in the root and not in the subfolder.
	if err := os.WriteFile(filepath.Join(root, "style.json"), []byte(`{"schema":"cav-moodboard/1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "scenes", "intro")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	out, errOut, data := flowGuide(t, true)
	info := data["flow"].(map[string]any)
	if info["source"] != "project" || info["valid"] != true || !strings.HasSuffix(info["path"].(string), "flow.md") {
		t.Errorf("flow = %v", info)
	}
	if w, _ := info["warnings"].([]any); len(w) != 0 {
		t.Errorf("warnings = %v; the schema check did not look in the root", w)
	}
	if errOut != "" || !strings.Contains(out, "## style") {
		t.Errorf("stderr %q", errOut)
	}
}

func TestGuideFlowJSONKeepsThePathOfABrokenFile(t *testing.T) {
	project, home := flowEnv(t)
	broken := "## a\nreview: maybe\n"
	if err := os.WriteFile(filepath.Join(project, "flow.md"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, data := flowGuide(t, true)
	info := data["flow"].(map[string]any)
	if info["source"] != "project" || info["path"] != "flow.md" || info["valid"] != false {
		t.Errorf("project flow = %v", info)
	}
	if e, _ := info["error"].(string); !strings.Contains(e, "line 2") || !strings.Contains(errOut, "line 2") {
		t.Errorf("error %q, stderr %q", e, errOut)
	}

	if err := os.Remove(filepath.Join(project, "flow.md")); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(home, "flow.md")
	if err := os.WriteFile(userFile, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, data = flowGuide(t, true)
	info = data["flow"].(map[string]any)
	if info["source"] != "user" || info["path"] != userFile || info["valid"] != false {
		t.Errorf("user flow = %v", info)
	}
}
