package music

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Go port must write the same scores as the Python code it copies; testdata/yue2 has
// that code's output for one planned score.
func TestYuE2ScoreMatchesThePythonHelper(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "yue2", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	var want struct {
		Planned float64            `json:"planned_seconds"`
		BPM     float64            `json:"bpm"`
		Moved   int                `json:"vocal_notes_moved"`
		Lyrics  string             `json:"lyrics"`
		Style   string             `json:"style"`
		Fits    map[string]float64 `json:"fits"`
	}
	if err := json.Unmarshal([]byte(read("expected.json")), &want); err != nil {
		t.Fatal(err)
	}
	planned := read("planned.abc")
	converted, moved, err := convertScore(planned)
	if err != nil {
		t.Fatal(err)
	}
	if converted != read("converted.abc") || moved != want.Moved {
		t.Fatalf("convertScore moved %d notes (want %d) and wrote:\n%s", moved, want.Moved, converted)
	}
	if got := lyricTags(converted); got != want.Lyrics {
		t.Fatalf("lyricTags: %q", got)
	}
	if got := instrumentalStyle("warm synthwave, 112 BPM"); got != want.Style {
		t.Fatalf("instrumentalStyle: %q", got)
	}
	for sec, nominal := range want.Fits {
		var s float64
		fmt.Sscan(sec, &s)
		fitted, got, err := fitScore(converted, s)
		if err != nil {
			t.Fatalf("fit %s: %v", sec, err)
		}
		if fitted != read("fit-"+sec+".abc") || got != nominal {
			t.Fatalf("fit %s s: %.1f s (want %.1f), score:\n%s", sec, got, nominal, fitted)
		}
		if _, err := validateInstrumental(fitted); err != nil {
			t.Fatalf("fit %s: %v", sec, err)
		}
	}
	in, err := makeInstrumental(planned, "warm synthwave, 112 BPM", 30)
	if err != nil {
		t.Fatal(err)
	}
	if in.abc != read("fit-30.abc") || in.planned != want.Planned || in.nominal != want.Fits["30"] || in.bpm != want.BPM ||
		in.cot != "full" || in.lyrics != "[Intro]\n\n[Verse]\n\n[Outro]\n" || in.style != want.Style {
		t.Fatalf("makeInstrumental: %+v", in)
	}
}

func TestYuE2ScoreRefusesOtherABC(t *testing.T) {
	for name, text := range map[string]string{
		"empty":         "",
		"general ABC":   "X:1\nT:Tune\nM:4/4\nL:1/8\nK:G\nGABc|\n",
		"unknown chord": strings.Replace(mustRead(t, "planned.abc"), `"Em"`, `"Em9"`, 1),
		"short bar":     strings.Replace(mustRead(t, "planned.abc"), "G,8z8", "G,8z4", 1),
	} {
		if _, _, err := convertScore(text); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func mustRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "yue2", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
