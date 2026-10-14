package main

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/rock3r/cav/assets"
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
	os.Stdout, os.Stderr = outW, errW
	f()
	outW.Close()
	errW.Close()
	o, _ := io.ReadAll(outR)
	e, _ := io.ReadAll(errR)
	return string(o), string(e)
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
	span := regexp.MustCompile("`(cav [^`]+)`")
	word := regexp.MustCompile(`^[a-z]+$`)
	seen := 0
	for _, m := range span.FindAllStringSubmatch(string(b), -1) {
		f := strings.Fields(m[1])
		if len(f) < 2 {
			continue
		}
		seen++
		c, ok := byName[f[1]]
		if !ok {
			t.Errorf("%q: cav has no command %q", m[1], f[1])
			continue
		}
		if len(f) > 2 && word.MatchString(f[2]) && !strings.Contains(c.args, f[2]) && !strings.Contains(longHelp[c.name], f[2]) {
			t.Errorf("%q: cav %s has no subcommand %q", m[1], c.name, f[2])
		}
	}
	if seen < 15 {
		t.Errorf("found only %d commands in flow.md; the parser is broken", seen)
	}
}
