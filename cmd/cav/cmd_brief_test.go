package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/brief"
	"github.com/rock3r/cav/internal/library"
)

// inNewProject runs the test in an empty project folder with its own CAV_HOME.
func inNewProject(t *testing.T) string {
	t.Helper()
	t.Setenv("CAV_HOME", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func TestBriefInitWritesTheSpecSketch(t *testing.T) {
	inNewProject(t)
	err := briefInit(&app{json: true}, []string{"--title", "Launch day sting", "--goal", "Announce the launch", "--audience", "Existing users",
		"--tone", "confident", "--tone", "playful", "--seconds", "15", "--bpm", "120", "--width", "1080", "--height", "1920", "--fps", "30",
		"--deliverable", "15 s vertical", "--must-have", "the logo, big", "--avoid", "stock music", "--music", "upbeat",
		"--ref", "https://vimeo.com/123456789"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := brief.Load("brief.json")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Launch day sting" || b.Seconds != 15 || b.BPM != 120 || b.Format.Width != 1080 || b.Format.Height != 1920 || b.Format.FPS != 30 {
		t.Fatalf("%+v", b)
	}
	if strings.Join(b.Tone, "|") != "confident|playful" || len(b.MustHave) != 1 || b.MustHave[0] != "the logo, big" {
		t.Fatalf("lists repeat, and a comma stays inside one value: %+v %+v", b.Tone, b.MustHave)
	}
	if len(b.Missing()) != 0 {
		t.Errorf("missing %v", b.Missing())
	}
	data, _ := os.ReadFile("brief.json")
	last := -1
	for _, k := range append([]string{"schema"}, brief.Order...) {
		i := strings.Index(string(data), `"`+k+`"`)
		if i < 0 || i < last {
			t.Fatalf("%s is missing or out of order:\n%s", k, data)
		}
		last = i
	}
}

func TestBriefInitWithoutFlagsWritesATemplate(t *testing.T) {
	inNewProject(t)
	var err error
	out, _ := captureStreams(t, func() { err = briefInit(&app{}, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Still to fill: title, goal, audience, tone, seconds, format") {
		t.Errorf("output:\n%s", out)
	}
	if !strings.Contains(out, "cav brief show") || !strings.Contains(out, "cav review brief.json") {
		t.Errorf("output lacks the next steps:\n%s", out)
	}
	data, _ := os.ReadFile("brief.json")
	for _, not := range []string{`"seconds"`, `"bpm"`, `"format"`, "null"} {
		if strings.Contains(string(data), not) {
			t.Errorf("template has %s:\n%s", not, data)
		}
	}
	if !strings.Contains(string(data), `"tone": []`) {
		t.Errorf("template:\n%s", data)
	}
}

func TestBriefInitRefusesToOverwrite(t *testing.T) {
	inNewProject(t)
	os.WriteFile("brief.json", []byte(`{"title":"mine"}`), 0o644)
	err := briefInit(&app{json: true}, []string{"--title", "other"})
	if err == nil || !strings.Contains(err.Error(), "brief.json already exists") {
		t.Fatalf("%v", err)
	}
	if got, _ := os.ReadFile("brief.json"); string(got) != `{"title":"mine"}` {
		t.Fatalf("the file changed: %s", got)
	}
	if err := briefInit(&app{json: true}, []string{"--force", "--title", "other"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := brief.Load("brief.json"); b.Title != "other" {
		t.Fatalf("%+v", b)
	}
}

func TestBriefInitWithBadValuesWritesNothing(t *testing.T) {
	inNewProject(t)
	for _, args := range [][]string{{"--seconds", "-1"}, {"--bpm", "5"}, {"--width", "1080"}, {"--ref", "ftp://x.test/a"}, {"--tone", ""}} {
		err := briefInit(&app{json: true}, args)
		if err == nil {
			t.Errorf("%v should fail", args)
		}
		if _, statErr := os.Stat("brief.json"); statErr == nil {
			t.Fatalf("%v wrote a file", args)
		}
	}
	err := briefInit(&app{json: true}, []string{"--seconds", "-1"})
	if err == nil || !strings.Contains(err.Error(), "seconds must be more than 0") {
		t.Fatalf("%v", err)
	}
}

func TestBriefShowReportsRefs(t *testing.T) {
	dir := inNewProject(t)
	os.Mkdir(".cav", 0o755)
	os.MkdirAll("moodboard", 0o755)
	os.WriteFile("moodboard/kept.jpg", []byte("x"), 0o644)
	os.WriteFile("moodboard/new.jpg", []byte("y"), 0o644)
	m := &library.Manifest{}
	m.Put(dir, library.Entry{Path: "moodboard/kept.jpg", Kind: "image", Source: "arena", Licence: "cc0"})
	data, _ := json.Marshal(m)
	os.WriteFile(library.ManifestPath(dir), data, 0o644)
	os.WriteFile("brief.json", []byte(`{"title":"T","goal":"G","audience":"A","tone":"calm","seconds":15,"bpm":120,
		"format":{"width":1080,"height":1920,"fps":30},"zeta":1,
		"refs":["https://vimeo.com/123456789","moodboard/kept.jpg","moodboard/new.jpg","moodboard/gone.jpg"]}`), 0o644)

	var err error
	out, _ := captureStreams(t, func() { err = briefShow(&app{}, nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"T (brief.json)", "Tone          calm", "15 s at 120 bpm (30 beats)", "1080 x 1920 (9:16) at 30 fps",
		"stays an embed, never downloaded", "recorded, licence cc0", "file; not recorded yet", "file; not found",
		"record it: cav ref add moodboard/new.jpg --licence <id>", `cav does not use "zeta"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Still to fill") {
		t.Errorf("the brief is complete:\n%s", out)
	}

	out, _ = captureStreams(t, func() { err = briefShow(&app{json: true}, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Missing []string
		Refs    []briefRef
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := []briefRef{
		{Ref: "https://vimeo.com/123456789", Kind: "link", Exists: true},
		{Ref: "moodboard/kept.jpg", Kind: "file", Exists: true, Recorded: true, Licence: "cc0"},
		{Ref: "moodboard/new.jpg", Kind: "file", Exists: true},
		{Ref: "moodboard/gone.jpg", Kind: "file"},
	}
	if len(got.Refs) != len(want) {
		t.Fatalf("%+v", got.Refs)
	}
	for i := range want {
		if got.Refs[i] != want[i] {
			t.Errorf("ref %d: %+v, want %+v", i, got.Refs[i], want[i])
		}
	}
	if len(got.Missing) != 0 {
		t.Errorf("missing %v", got.Missing)
	}
}

func TestBriefShowFailsOnProblemsButNotOnGaps(t *testing.T) {
	inNewProject(t)
	if err := briefShow(&app{json: true}, nil); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a missing file: %v", err)
	}
	os.WriteFile("brief.json", []byte(`{"title":"T"}`), 0o644)
	var err error
	out, _ := captureStreams(t, func() { err = briefShow(&app{}, nil) })
	if err != nil {
		t.Fatalf("gaps are not an error: %v", err)
	}
	if !strings.Contains(out, "Still to fill: goal, audience, tone, seconds, format") {
		t.Errorf("output:\n%s", out)
	}
	os.WriteFile("brief.json", []byte(`{"seconds":"fifteen"}`), 0o644)
	err = briefShow(&app{}, nil)
	if err == nil || !strings.Contains(err.Error(), "seconds must be a number of seconds, like 15") {
		t.Fatalf("%v", err)
	}
}
