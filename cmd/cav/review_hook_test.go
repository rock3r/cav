package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/review"
)

func writeReview(t *testing.T, dir string, sends ...review.Send) {
	t.Helper()
	d := review.Doc{Schema: review.Schema, Source: review.Source{Render: "renders/final.mp4"}, Sends: sends}
	b, _ := json.Marshal(d)
	os.MkdirAll(filepath.Join(dir, "renders"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "renders", "final.review.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func runHook(t *testing.T, event, cwd string, stopActive bool) map[string]any {
	t.Helper()
	in, _ := json.Marshal(map[string]any{"hook_event_name": event, "cwd": cwd, "stop_hook_active": stopActive})
	var out, errb bytes.Buffer
	if code := reviewHook(bytes.NewReader(in), &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if strings.TrimSpace(out.String()) == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %q", out.String())
	}
	return m
}

func TestReviewHookTellsTheAgentOnce(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_OUT_DIR", "")
	dir := t.TempDir()
	if m := runHook(t, "UserPromptSubmit", dir, false); m != nil {
		t.Fatalf("no review file: %v", m)
	}
	delivered := time.Now()
	writeReview(t, dir, review.Send{N: 1, At: time.Now(), Comments: []string{"c_01"}, DeliveredAt: &delivered})
	if m := runHook(t, "UserPromptSubmit", dir, false); m != nil {
		t.Fatalf("a received send must stay quiet: %v", m)
	}

	writeReview(t, dir, review.Send{N: 1, At: time.Now(), Comments: []string{"c_01"}, DeliveredAt: &delivered},
		review.Send{N: 2, At: time.Now(), Comments: []string{"c_02", "c_03"}})
	m := runHook(t, "UserPromptSubmit", dir, false)
	hso, _ := m["hookSpecificOutput"].(map[string]any)
	ctx, _ := hso["additionalContext"].(string)
	if hso["hookEventName"] != "UserPromptSubmit" || !strings.Contains(ctx, "cav review wait renders/final.mp4") || !strings.Contains(m["systemMessage"].(string), "2 note") {
		t.Fatalf("prompt hook output: %v", m)
	}
	if m := runHook(t, "UserPromptSubmit", dir, false); m != nil {
		t.Fatalf("second prompt repeats the same send: %v", m)
	}
	if m := runHook(t, "Stop", dir, false); len(m) != 0 {
		t.Fatalf("Stop after the prompt already told the agent: %v", m)
	}

	writeReview(t, dir, review.Send{N: 3, At: time.Now().Add(time.Second), Comments: []string{"c_04"}})
	if m := runHook(t, "Stop", dir, true); len(m) != 0 {
		t.Fatalf("a continued turn must not be continued again: %v", m)
	}
	m = runHook(t, "Stop", dir, false)
	if m["decision"] != "block" || !strings.Contains(m["reason"].(string), "send #3") {
		t.Fatalf("Stop output: %v", m)
	}
}
