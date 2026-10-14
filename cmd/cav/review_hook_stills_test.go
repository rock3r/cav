package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/review"
)

func TestHookFindsStillsReviewAtProjectRoot(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_OUT_DIR", "")
	dir := t.TempDir()
	// A stills review file sits next to its target in the project, not in renders/.
	d := review.Doc{Schema: review.Schema, Kind: "storyboard", Source: review.Source{Render: "storyboard.json"},
		Sends: []review.Send{{N: 1, At: time.Now(), Comments: []string{}, Approved: true}}}
	b, _ := json.Marshal(d)
	if err := os.WriteFile(filepath.Join(dir, "storyboard.review.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	m := runHook(t, "UserPromptSubmit", dir, false)
	hso, _ := m["hookSpecificOutput"].(map[string]any)
	ctx, _ := hso["additionalContext"].(string)
	if !strings.Contains(ctx, "approved `storyboard.json`") || !strings.Contains(ctx, "cav review wait "+shellArg("storyboard.json")) ||
		m["systemMessage"] != "cav review: the reviewer approved storyboard.json" {
		t.Fatalf("hook output: %v", m)
	}

	// Without a recorded source, a stills review points at its .json, not at an .mp4.
	os.WriteFile(filepath.Join(dir, "moodboard.review.json"), []byte(`{"kind":"moodboard","sends":[{"n":1,"at":"2026-01-01T00:00:00Z","comments":["c_01"]}]}`), 0o644)
	var found []string
	for _, p := range findPending(dir) {
		found = append(found, p.video)
	}
	if !strings.Contains(strings.Join(found, " "), "moodboard.json") {
		t.Fatalf("pending targets %v", found)
	}
}

func TestFindPendingCountsAFolderOnce(t *testing.T) {
	t.Setenv("CAV_OUT_DIR", ".")
	dir := t.TempDir()
	d := review.Doc{Schema: review.Schema, Source: review.Source{Render: "final.mp4"}, Sends: []review.Send{{N: 1, At: time.Now(), Comments: []string{"c_01"}}}}
	b, _ := json.Marshal(d)
	os.WriteFile(filepath.Join(dir, "final.review.json"), b, 0o644)
	if got := findPending(dir); len(got) != 1 {
		t.Fatalf("the renders folder is the project folder: %+v", got)
	}
}

func TestHookMessageSaysApproved(t *testing.T) {
	text, msg := hookMessage([]pendingSend{{video: "storyboard.json", n: 2, approved: true}})
	if !strings.Contains(text, "the reviewer approved `storyboard.json` (send #2") || !strings.Contains(text, "to confirm") || msg != "cav review: the reviewer approved storyboard.json" {
		t.Fatalf("approved: %q / %q", text, msg)
	}
	text, msg = hookMessage([]pendingSend{{video: "a.mp4", n: 1, notes: 3}})
	if !strings.Contains(text, "3 note(s) on `a.mp4`") || !strings.Contains(text, "for a brief, mood board or storyboard") || msg != "cav review: the reviewer sent 3 note(s) on a.mp4" {
		t.Fatalf("notes: %q / %q", text, msg)
	}
}
