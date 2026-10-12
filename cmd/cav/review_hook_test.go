package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	if code := reviewHook(nil, bytes.NewReader(in), &out, &errb); code != 0 {
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
	if hso["hookEventName"] != "UserPromptSubmit" || !strings.Contains(ctx, "cav review wait "+shellArg("renders/final.mp4")) || !strings.Contains(m["systemMessage"].(string), "2 note") {
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

func runAntigravityHook(t *testing.T, event string, in map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(in)
	var out, errb bytes.Buffer
	if code := reviewHook([]string{"--host", "antigravity", "--event", event}, bytes.NewReader(b), &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %q (%s)", out.String(), errb.String())
	}
	return m
}

func TestAntigravityHookTellsTheAgentOnce(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_OUT_DIR", "")
	other, dir := t.TempDir(), t.TempDir()
	in := map[string]any{"conversationId": "conv-1", "workspacePaths": []string{other, dir}, "invocationNum": 1}
	if m := runAntigravityHook(t, "PreInvocation", in); len(m) != 0 {
		t.Fatalf("no review file: %v", m)
	}
	stop := map[string]any{"conversationId": "conv-1", "workspacePaths": []string{other, dir}, "terminationReason": "model_stop", "fullyIdle": true}
	if m := runAntigravityHook(t, "Stop", stop); m["decision"] != "stop" {
		t.Fatalf("a quiet Stop must let the agent stop: %v", m)
	}

	writeReview(t, dir, review.Send{N: 1, At: time.Now(), Comments: []string{"c_01", "c_02"}})
	m := runAntigravityHook(t, "PreInvocation", in)
	steps, _ := m["injectSteps"].([]any)
	if len(steps) != 1 {
		t.Fatalf("PreInvocation output: %v", m)
	}
	msg, _ := steps[0].(map[string]any)["ephemeralMessage"].(string)
	// With several workspace folders, the render is named in full.
	if !strings.Contains(msg, "cav review wait "+shellArg(filepath.Join(dir, "renders", "final.mp4"))) || !strings.Contains(msg, "send #1") {
		t.Fatalf("ephemeralMessage: %q", msg)
	}
	// PreInvocation runs before every model call: the same send must not come back.
	if m := runAntigravityHook(t, "PreInvocation", in); len(m) != 0 {
		t.Fatalf("second model call repeats the same send: %v", m)
	}
	if m := runAntigravityHook(t, "Stop", stop); m["decision"] != "stop" {
		t.Fatalf("Stop after PreInvocation already told the agent: %v", m)
	}

	writeReview(t, dir, review.Send{N: 1, At: time.Now(), Comments: []string{"c_01", "c_02"}},
		review.Send{N: 2, At: time.Now().Add(time.Second), Comments: []string{"c_03"}})
	failed := map[string]any{"conversationId": "conv-1", "workspacePaths": []string{dir}, "terminationReason": "error", "fullyIdle": true}
	if m := runAntigravityHook(t, "Stop", failed); m["decision"] != "stop" {
		t.Fatalf("a Stop after an error must not continue: %v", m)
	}
	m = runAntigravityHook(t, "Stop", stop)
	if m["decision"] != "continue" || !strings.Contains(m["reason"].(string), "send #2") {
		t.Fatalf("Stop output: %v", m)
	}
	// There is no stop_hook_active: the state file ends the loop.
	if m := runAntigravityHook(t, "Stop", stop); m["decision"] != "stop" {
		t.Fatalf("a continued turn must not be continued again: %v", m)
	}
}

func TestAntigravityHookNeedsAKnownEvent(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	if m := runAntigravityHook(t, "PostToolUse", map[string]any{}); len(m) != 0 {
		t.Fatalf("unknown event: %v", m)
	}
}

func TestShellArgQuotesEveryPath(t *testing.T) {
	cases := map[string][2]string{
		"/work/renders/final.mp4": {`'/work/renders/final.mp4'`, `'/work/renders/final.mp4'`},
		"/My Work/it's/final.mp4": {`'/My Work/it'\''s/final.mp4'`, `'/My Work/it''s/final.mp4'`},
		"/a\\b\nrm -rf x.mp4":     {`'/a\b?rm -rf x.mp4'`, `'/a\b?rm -rf x.mp4'`},
	}
	for in, want := range cases {
		w := want[0]
		if runtime.GOOS == "windows" {
			w = want[1]
		}
		if got := shellArg(in); got != w {
			t.Errorf("shellArg(%q) = %q, want %q", in, got, w)
		}
	}
}

func TestHookFindsTheRenderNextToItsReviewFile(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_OUT_DIR", "")
	dir := t.TempDir()
	// cav review ran in a subfolder and stored the path as typed.
	d := review.Doc{Schema: review.Schema, Source: review.Source{Render: "../renders/final.mp4"}, Sends: []review.Send{{N: 1, At: time.Now(), Comments: []string{"c_01"}}}}
	b, _ := json.Marshal(d)
	os.MkdirAll(filepath.Join(dir, "renders"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "renders", "final.review.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	m := runAntigravityHook(t, "PreInvocation", map[string]any{"workspacePaths": []string{dir}})
	steps, _ := m["injectSteps"].([]any)
	if len(steps) != 1 {
		t.Fatalf("PreInvocation output: %v", m)
	}
	msg, _ := steps[0].(map[string]any)["ephemeralMessage"].(string)
	if !strings.Contains(msg, "cav review wait "+shellArg(filepath.Join(dir, "renders", "final.mp4"))+" ") {
		t.Fatalf("ephemeralMessage: %q", msg)
	}
}

func TestAntigravityHookCountsASharedOutputFolderOnce(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	shared := t.TempDir()
	t.Setenv("CAV_OUT_DIR", filepath.Join(shared, "renders"))
	writeReview(t, shared, review.Send{N: 1, At: time.Now(), Comments: []string{"c_01"}})
	m := runAntigravityHook(t, "PreInvocation", map[string]any{"workspacePaths": []string{t.TempDir(), t.TempDir()}})
	steps, _ := m["injectSteps"].([]any)
	if len(steps) != 1 {
		t.Fatalf("PreInvocation output: %v", m)
	}
	msg, _ := steps[0].(map[string]any)["ephemeralMessage"].(string)
	if n := strings.Count(msg, "send #1"); n != 1 {
		t.Fatalf("send listed %d times: %q", n, msg)
	}
}
