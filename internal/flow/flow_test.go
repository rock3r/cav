package flow

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const specExample = `# My flow

## brief
Ask first.
outputs: brief.json
review: yes

## style frames
Make three frames before anything else.
outputs: style/*.png, style/notes.md
review: no
use: cav gen image

## storyboard
outputs: storyboard.json, board/
review: yes

## build
outputs: renders/final.mp4
review: no
`

func TestSpecExampleParses(t *testing.T) {
	f, err := Parse("flow.md", []byte(specExample))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stages) != 4 {
		t.Fatalf("got %d stages: %+v", len(f.Stages), f.Stages)
	}
	s := f.Stages[1]
	if s.Name != "style frames" || s.Line != 8 || s.Review || s.Use != "cav gen image" {
		t.Errorf("stage 2 = %+v", s)
	}
	if want := []string{"style/*.png", "style/notes.md"}; !reflect.DeepEqual(s.Outputs, want) {
		t.Errorf("outputs = %q, want %q", s.Outputs, want)
	}
	if got := f.Stages[2].Outputs; got[1] != "board/" {
		t.Errorf("folder output = %q", got)
	}
	if !f.Stages[0].Review || !f.Stages[2].Review || f.Stages[3].Review {
		t.Error("review flags are wrong")
	}
}

func TestStandardFlow(t *testing.T) {
	f := Standard()
	if f.Source != SourceStandard || f.Label() != "standard (built in)" {
		t.Errorf("source %q label %q", f.Source, f.Label())
	}
	if len(f.Stages) != 5 {
		t.Fatalf("got %d stages", len(f.Stages))
	}
	if f.Stages[1].Outputs[0] != "moodboard.json" {
		t.Errorf("mood board outputs = %q", f.Stages[1].Outputs)
	}
	var review []bool
	for _, s := range f.Stages {
		review = append(review, s.Review)
		if s.Use == "" || len(s.Outputs) == 0 {
			t.Errorf("stage %q has no use: or outputs:", s.Name)
		}
		if s.Review {
			if target, problem := s.ReviewTarget(); problem != "" || target == "" {
				t.Errorf("stage %q review target: %q, %q", s.Name, target, problem)
			}
		}
	}
	if want := []bool{true, true, true, false, false}; !reflect.DeepEqual(review, want) {
		t.Errorf("review flags = %v, want %v", review, want)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"empty outputs", "## a\noutputs:\n", "line 2: outputs: is empty"},
		{"absolute", "## a\noutputs: /abs/file.json\n", "line 2: outputs are paths inside the project; /abs/file.json is not"},
		{"parent", "## a\noutputs: ../x.json\n", "line 2: outputs are paths inside the project; ../x.json is not"},
		{"double star", "## a\noutputs: a/**/b.png\n", "line 2: a/**/b.png: ** is not supported; * matches inside one folder"},
		{"bad review", "## a\noutputs: a.json\nreview: maybe\n", `line 3: review: must be yes or no, not "maybe"`},
		{"repeated outputs", "## storyboard\noutputs: a.json\n\noutputs: b.json\n", "line 4: stage storyboard already has outputs: (line 2); put all paths on one line"},
		{"key first", "\n\noutputs: a.json\n## a\n", "line 3: outputs: comes before the first stage; put it under a line like `## storyboard`"},
		{"review without outputs", "## brief\nreview: yes\n", "stage brief (line 1) has review: yes but no outputs:"},
		{"no stages", "just text\n", "has no stages: start each stage with a line like `## storyboard`"},
		{"empty name", "##\noutputs: a.json\n", "line 1: a stage needs a name"},
		{"duplicate", "## Brief\n## brief\n", "line 2: stage brief is already defined on line 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse("flow.md", []byte(c.src))
			if err == nil {
				t.Fatalf("no error; flow %+v", f)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to contain %q", err, c.want)
			}
			if c.name != "no stages" && !strings.Contains(err.Error(), "flow.md") {
				t.Errorf("error does not name the file: %q", err)
			}
		})
	}
}

func TestParseReportsEveryProblem(t *testing.T) {
	_, err := Parse("flow.md", []byte("## a\nreview: maybe\noutputs: /x\n## a\n"))
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"line 2", "line 3", "line 4"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q is missing from %q", want, err)
		}
	}
}

func TestParseCRLFAndBOM(t *testing.T) {
	src := "\xef\xbb\xbf# F\r\n\r\n## one\r\noutputs: a.json\r\nreview: YES\r\nuse: do it\r\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stages) != 1 || !f.Stages[0].Review || f.Stages[0].Use != "do it" || f.Stages[0].Outputs[0] != "a.json" {
		t.Errorf("stages = %+v", f.Stages)
	}
	if strings.Contains(f.Text, "\r") || strings.HasPrefix(f.Text, "\xef\xbb\xbf") {
		t.Errorf("text still has CR or BOM: %q", f.Text)
	}
}

func TestKeysInsideAFenceAreProse(t *testing.T) {
	src := "## one\noutputs: a.json\n```\n## fake\noutputs: /bad\nreview: maybe\n```\nreview: yes\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stages) != 1 || !f.Stages[0].Review || !reflect.DeepEqual(f.Stages[0].Outputs, []string{"a.json"}) {
		t.Errorf("stages = %+v", f.Stages)
	}
}

func TestIndentedCodeIsProse(t *testing.T) {
	src := "## one\noutputs: a.json\n\n    review: maybe\n    ## fake\n\toutputs: /bad\n\n   review: yes\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// Three spaces still count; four spaces and a tab are code.
	if len(f.Stages) != 1 || !f.Stages[0].Review || !reflect.DeepEqual(f.Stages[0].Outputs, []string{"a.json"}) {
		t.Errorf("stages = %+v", f.Stages)
	}
	// A heading indented by up to three spaces is still a heading.
	f, err = Parse("flow.md", []byte("  ## two\noutputs: b.json\n"))
	if err != nil || len(f.Stages) != 1 || f.Stages[0].Name != "two" {
		t.Errorf("indented heading: %+v, %v", f, err)
	}
	// A fence opened with four spaces is code, not a fence.
	f, err = Parse("flow.md", []byte("## one\n    ```\noutputs: a.json\n"))
	if err != nil || len(f.Stages[0].Outputs) != 1 {
		t.Errorf("indented fence: %+v, %v", f, err)
	}
}

func TestHTMLCommentsDisableStages(t *testing.T) {
	src := "## brief\noutputs: brief.json\n\n" +
		"<!--\n## brief\noutputs: /bad\nreview: maybe\nreview: no\n-->\n\n" +
		"<!-- ## one line\noutputs: x.json -->\n" +
		"<!-- ## inline --> ## build\noutputs: final.mp4 <!-- , old.mp4 -->\nreview: no\n\n" +
		"```\n<!--\n```\n" +
		"## after the fence\noutputs: a.json\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range f.Stages {
		names = append(names, s.Name)
	}
	if want := []string{"brief", "build", "after the fence"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("stages = %q, want %q", names, want)
	}
	if got := f.Stages[1].Outputs; !reflect.DeepEqual(got, []string{"final.mp4"}) {
		t.Errorf("build outputs = %q", got)
	}
	if f.Stages[0].Review {
		t.Error("review: no inside the comment changed the brief stage")
	}
}

func TestHTMLCommentMarkerInsideACodeSpanIsText(t *testing.T) {
	src := "## one\nUse `<!--` to start a comment, or ``a ` <!-- b`` too.\noutputs: a.json\n\n## two\noutputs: b.json\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stages) != 2 || f.Unclosed != 0 {
		t.Errorf("stages = %+v, unclosed %d", f.Stages, f.Unclosed)
	}
	// A lone backtick has no closer, so it is text and the comment after it starts.
	f, err = Parse("flow.md", []byte("## one\noutputs: a.json\nit` <!-- hidden\n## two\noutputs: b.json\n"))
	if err != nil || len(f.Stages) != 1 || f.Unclosed != 3 {
		t.Errorf("lone backtick: %+v, %v", f, err)
	}
}

func TestUnclosedCommentIsAWarning(t *testing.T) {
	src := "## one\noutputs: a.json\n\n<!-- forgot to close\n## two\noutputs: b.json\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	f.Path = "flow.md"
	want := []string{"flow.md line 4: <!-- is never closed; the rest of the file is ignored"}
	if got := f.Warnings(""); !reflect.DeepEqual(got, want) || len(f.Stages) != 1 {
		t.Errorf("warnings %q, stages %+v", got, f.Stages)
	}
	// A comment that is closed later in the file is not a warning.
	f, err = Parse("flow.md", []byte("## one\noutputs: a.json\n<!-- a -->\n<!-- b\n-->\n"))
	if err != nil || f.Unclosed != 0 || len(f.Warnings("")) != 0 {
		t.Errorf("closed comments: %+v, %v", f, err)
	}
}

func TestHTMLCommentInsideIndentedCodeIsNotAComment(t *testing.T) {
	f, err := Parse("flow.md", []byte("## one\n    <!--\noutputs: a.json\n"))
	if err != nil || len(f.Stages[0].Outputs) != 1 {
		t.Errorf("stages = %+v, %v", f, err)
	}
}

func TestIndentedFenceDoesNotCloseAFence(t *testing.T) {
	src := "## one\noutputs: a.json\n```\n    ```\noutputs: /bad\nreview: maybe\n```\nreview: yes\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stages) != 1 || !f.Stages[0].Review || !reflect.DeepEqual(f.Stages[0].Outputs, []string{"a.json"}) {
		t.Errorf("stages = %+v", f.Stages)
	}
	// Up to three spaces still close it.
	f, err = Parse("flow.md", []byte("## one\n```\n   ```\noutputs: a.json\n"))
	if err != nil || len(f.Stages[0].Outputs) != 1 {
		t.Errorf("three-space closer: %+v, %v", f, err)
	}
}

func TestOutputsAreNormalised(t *testing.T) {
	f, err := Parse("flow.md", []byte("## one\nOutputs: `a.json`, renders\\final.mp4 ,, `board/`\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.json", "renders/final.mp4", "board/"}
	if got := f.Stages[0].Outputs; !reflect.DeepEqual(got, want) {
		t.Errorf("outputs = %q, want %q", got, want)
	}
}

func TestOtherHeadingsAreProse(t *testing.T) {
	f, err := Parse("flow.md", []byte("# Title\n## one\n### Notes\n#hashtag\noutputs: a.json\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stages) != 1 || f.Stages[0].Name != "one" {
		t.Errorf("stages = %+v", f.Stages)
	}
}

func TestLookupOrder(t *testing.T) {
	project, home := t.TempDir(), t.TempDir()
	t.Setenv("CAV_HOME", home)

	if path, source := Find(project); path != "" || source != SourceStandard {
		t.Errorf("empty dirs: %q, %q", path, source)
	}
	f, err := Load(project)
	if err != nil || f.Source != SourceStandard {
		t.Fatalf("standard load: %+v, %v", f, err)
	}

	userFlow := "## user stage\noutputs: u.json\n"
	if err := os.WriteFile(filepath.Join(home, "flow.md"), []byte(userFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err = Load(project)
	if err != nil || f.Source != SourceUser || f.Stages[0].Name != "user stage" {
		t.Fatalf("user load: %+v, %v", f, err)
	}
	if f.Path != filepath.Join(home, "flow.md") || f.Label() != f.Path {
		t.Errorf("path %q label %q", f.Path, f.Label())
	}

	projFlow := "## project stage\noutputs: p.json\n"
	if err := os.WriteFile(filepath.Join(project, "flow.md"), []byte(projFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err = Load(project)
	if err != nil || f.Source != SourceProject || f.Stages[0].Name != "project stage" || f.Label() != "flow.md in this project" {
		t.Fatalf("project load: %+v, %v", f, err)
	}
	if f.Text != projFlow {
		t.Errorf("text = %q", f.Text)
	}
}

func TestBrokenProjectFileDoesNotFallBack(t *testing.T) {
	project, home := t.TempDir(), t.TempDir()
	t.Setenv("CAV_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "flow.md"), []byte("## ok\noutputs: a.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "flow.md"), []byte("## bad\nreview: maybe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(project)
	if err == nil || f != nil {
		t.Fatalf("Load = %+v, %v; want an error", f, err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error = %q", err)
	}
}

func TestReviewTarget(t *testing.T) {
	cases := []struct {
		first, target, problem string
	}{
		{"storyboard.json", "storyboard.json", ""},
		{"moodboard/", "", "moodboard/ is a folder; list the .json file to review first, or set review: no"},
		{"style/*.png", "", "style/*.png is a pattern; list the .json file to review first, or set review: no"},
		{"renders/final.mp4", "", "renders/final.mp4 is a video; a video review holds notes but no approval: set review: no, or list a .json file first"},
		{"notes.md", "", "notes.md is not a .json file"},
	}
	for _, c := range cases {
		target, problem := Stage{Outputs: []string{c.first, "other.json"}}.ReviewTarget()
		if target != c.target || !strings.HasPrefix(problem, c.problem) {
			t.Errorf("%s: target %q problem %q", c.first, target, problem)
		}
	}
	if _, problem := (Stage{}).ReviewTarget(); problem == "" {
		t.Error("a stage without outputs has no problem")
	}
}

func TestWarningsNameTheStageAndLine(t *testing.T) {
	src := "## brief\noutputs: brief.json\nreview: yes\n\n## style frames\noutputs: style/*.png\nreview: yes\n\n## cut\noutputs: renders/cut.mp4\nreview: yes\n\n## assets\noutputs: assets/\nreview: yes\n\n## notes\noutputs: notes.md\nreview: yes\n\n## quiet\noutputs: x/*.png\nreview: no\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatalf("a review: yes stage with a pattern must still parse: %v", err)
	}
	f.Path = "flow.md"
	got := f.Warnings("")
	want := []string{
		"flow.md line 5: stage style frames: style/*.png is a pattern; list the .json file to review first, or set review: no",
		"flow.md line 9: stage cut: renders/cut.mp4 is a video; a video review holds notes but no approval: set review: no, or list a .json file first",
		"flow.md line 13: stage assets: assets/ is a folder; list the .json file to review first, or set review: no",
		"flow.md line 17: stage notes: notes.md is not a .json file; list the .json file to review first, or set review: no",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("warnings:\n%q\nwant:\n%q", got, want)
	}
	if w := Standard().Warnings(""); len(w) != 0 {
		t.Errorf("the standard flow has warnings: %q", w)
	}
}

func TestWarningsForJSONFilesCavReviewCannotOpen(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("style.json", `{"schema":"cav-moodboard/1","items":[]}`)
	write("plain.json", `{"hello":"world"}`)
	src := "## named\noutputs: style.json\nreview: yes\n\n## other\noutputs: plain.json\nreview: yes\n\n## missing\noutputs: nope.json\nreview: yes\n\n## nested\noutputs: sub/storyboard.json\nreview: yes\n"
	f, err := Parse("flow.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	f.Path = "flow.md"
	const tail = " is not a file cav review can approve (brief.json, moodboard.json or storyboard.json, or a file with their schema); set review: no or list one of those first"
	want := []string{
		"flow.md line 5: stage other: plain.json" + tail,
		"flow.md line 9: stage missing: nope.json" + tail,
	}
	if got := f.Warnings(root); !reflect.DeepEqual(got, want) {
		t.Errorf("with the files:\n%q\nwant:\n%q", got, want)
	}
	// Without a folder to look in, only the file name counts.
	if got := f.Warnings(""); len(got) != 3 || !strings.Contains(got[0], "style.json"+tail) {
		t.Errorf("without a folder: %q", got)
	}
}
