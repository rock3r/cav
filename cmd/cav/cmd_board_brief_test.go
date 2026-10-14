package main

import (
	"os"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/board"
)

const verticalBrief = `{"title":"Launch day sting","goal":"g","audience":"a","tone":"calm","seconds":15,"bpm":90,"format":{"width":1080,"height":1920,"fps":30}}`

func TestBoardInitTakesTheBrief(t *testing.T) {
	inNewProject(t)
	os.WriteFile("brief.json", []byte(verticalBrief), 0o644)
	var err error
	out, _ := captureStreams(t, func() { err = boardInit(&app{}, []string{"--shots", "4"}) })
	if err != nil {
		t.Fatal(err)
	}
	sb, err := board.Load("storyboard.json")
	if err != nil {
		t.Fatal(err)
	}
	if sb.Width != 1080 || sb.Height != 1920 || sb.FPS != 30 || sb.Seconds != 15 || sb.BPM != 90 || sb.Title != "Launch day sting" {
		t.Fatalf("%+v", sb)
	}
	if !strings.Contains(out, "from brief.json: 1080 x 1920, 30 fps, 15 s, 90 bpm") {
		t.Errorf("it should say what it took:\n%s", out)
	}
}

func TestBoardInitFlagsWinOverTheBrief(t *testing.T) {
	inNewProject(t)
	os.WriteFile("brief.json", []byte(verticalBrief), 0o644)
	var err error
	out, _ := captureStreams(t, func() { err = boardInit(&app{}, []string{"--shots", "4", "--fps", "60"}) })
	if err != nil {
		t.Fatal(err)
	}
	sb, _ := board.Load("storyboard.json")
	if sb.FPS != 60 || sb.Width != 1080 {
		t.Fatalf("%+v", sb)
	}
	if !strings.Contains(out, "(--fps given)") || strings.Contains(out, "30 fps") {
		t.Errorf("it should report the override:\n%s", out)
	}
	// Width and height travel together: giving one takes neither from the brief.
	if err := boardInit(&app{json: true}, []string{"--force", "--width", "800"}); err != nil {
		t.Fatal(err)
	}
	sb, _ = board.Load("storyboard.json")
	if sb.Width != 800 || sb.Height != 1080 {
		t.Fatalf("a lone --width must leave the height at its built-in value: %+v", sb)
	}
}

func TestBoardInitNoBriefUsesBuiltInDefaults(t *testing.T) {
	inNewProject(t)
	os.WriteFile("brief.json", []byte(verticalBrief), 0o644)
	if err := boardInit(&app{json: true}, []string{"--no-brief"}); err != nil {
		t.Fatal(err)
	}
	sb, _ := board.Load("storyboard.json")
	if sb.Width != 1920 || sb.Height != 1080 || sb.FPS != 60 || sb.Seconds != 16 || sb.BPM != 120 || sb.Title != "" {
		t.Fatalf("%+v", sb)
	}
}

func TestBoardInitWithABrokenBriefNamesTheWayOut(t *testing.T) {
	inNewProject(t)
	os.WriteFile("brief.json", []byte(`{"seconds":"fifteen"}`), 0o644)
	err := boardInit(&app{json: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "seconds must be a number") {
		t.Fatalf("%v", err)
	}
	if ce, ok := err.(*cliError); !ok || !strings.Contains(ce.hint, "--no-brief") {
		t.Fatalf("the hint should name --no-brief: %v", err)
	}
	if _, statErr := os.Stat("storyboard.json"); statErr == nil {
		t.Fatal("nothing should be written")
	}
	if err := boardInit(&app{json: true}, []string{"--brief", "nope.json"}); err == nil {
		t.Error("a brief named with --brief must exist")
	}
}

func TestBoardInitBriefWithoutBPMKeepsTheDefaultAndSaysSo(t *testing.T) {
	inNewProject(t)
	os.WriteFile("brief.json", []byte(`{"title":"T","seconds":10,"format":{"width":1280,"height":720}}`), 0o644)
	var err error
	out, _ := captureStreams(t, func() { err = boardInit(&app{}, nil) })
	if err != nil {
		t.Fatal(err)
	}
	sb, _ := board.Load("storyboard.json")
	if sb.BPM != 120 || sb.FPS != 60 || sb.Seconds != 10 || sb.Width != 1280 {
		t.Fatalf("%+v", sb)
	}
	if !strings.Contains(out, "brief.json has no bpm; using 120") {
		t.Errorf("output:\n%s", out)
	}
}
