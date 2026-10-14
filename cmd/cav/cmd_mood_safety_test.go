package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoardMoodNeverWritesOutsideTheThumbFolder(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := inNewProject(t)
	clip := "moodboard/reel.mp4"
	os.MkdirAll("moodboard", 0o755)
	if b, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-t", "1", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot make a clip: %v %s", err, b)
	}
	data, _ := os.ReadFile(clip)
	os.WriteFile("moodboard/other.mp4", data, 0o644)
	victim := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(victim, []byte("keep me"), 0o644)
	escape := filepath.Join(filepath.Dir(dir), "escaped.png")
	os.WriteFile("moodboard.json", []byte(`{"schema":"cav-moodboard/1","dir":"moodboard","items":[
		{"id":"reel","kind":"clip","file":"moodboard/reel.mp4","thumb":`+jsonStr(victim)+`},
		{"id":"../../escaped","kind":"clip","file":"moodboard/other.mp4","thumb":"../escaped.png"}]}`), 0o644)
	var err error
	_, _, notes, _ := moodSync(context.Background(), "moodboard", "moodboard.json")
	t.Log(notes)
	captureStreams(t, func() { err = boardMood(&app{json: true}, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep me" {
		t.Error("the victim file was overwritten")
	}
	if _, err := os.Stat(escape); err == nil {
		t.Error("a file was written outside the project")
	}
	entries, _ := os.ReadDir("moodboard/.thumbs")
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if got := strings.Join(names, ","); got != "escaped.png,reel.png" {
		t.Errorf("thumbnails %s", got)
	}
}

func TestBriefShowHintsForRefsOutsideTheMoodBoard(t *testing.T) {
	inNewProject(t)
	os.MkdirAll("assets", 0o755)
	os.WriteFile("assets/logo.png", []byte("x"), 0o644)
	os.WriteFile("moodboard.json", []byte(`{"schema":"cav-moodboard/1","items":[{"id":"yt-aqz-KE-bpKQ","kind":"embed","url":"https://youtu.be/aqz-KE-bpKQ"}]}`), 0o644)
	os.WriteFile("brief.json", []byte(`{"title":"T","goal":"G","audience":"A","tone":"calm","seconds":15,"bpm":120,
		"format":{"width":1080,"height":1920,"fps":30},
		"refs":["assets/logo.png","https://www.youtube.com/watch?v=aqz-KE-bpKQ","https://vimeo.com/123456789","https://example.com/x"]}`), 0o644)
	var err error
	out, _ := captureStreams(t, func() { err = briefShow(&app{}, nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"copy it into moodboard/, then: cav ref add moodboard/logo.png --licence LICENCE_ID --by brief",
		"embed it: cav ref add https://vimeo.com/123456789 --by brief"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, not := range []string{"aqz-KE-bpKQ --by brief", "example.com/x --by"} {
		if strings.Contains(out, not) {
			t.Errorf("output should not hint %q:\n%s", not, out)
		}
	}
}

// jsonStr quotes a path for a JSON fixture: a Windows path has backslashes.
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
