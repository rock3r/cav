package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/operation"
)

// This fixture never starts or contacts Cavalry. Both requests and files are disposable.
func fixtureBridge(t *testing.T, handler func(bridge.Request)) *httptest.Server {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CAV_HOME", home)
	t.Setenv("CAV_SPOOL", "")
	t.Setenv("CAV_RELAYED", "1")
	if err := os.MkdirAll(config.JobsDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.TokenPath(), []byte(strings.Repeat("a", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req bridge.Request
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
				return
			}
			handler(req)
			w.WriteHeader(200)
			return
		}
		fmt.Fprint(w, `{"type":"hello","bridge":"cav-bridge","protocol":1}`)
	}))
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv("CAV_BRIDGE_HOST", host)
	t.Setenv("CAV_BRIDGE_PORT", port)
	t.Cleanup(srv.Close)
	return srv
}
func fixtureResult(t *testing.T, id string, v any) {
	t.Helper()
	b, e := json.Marshal(bridge.Result{Type: "result", ID: id, OK: true, Value: mustJSON(t, v), BridgeVersion: "1.0.1", Protocol: 1})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(config.JobsDir(), id+".json"), b, 0600); e != nil {
		t.Fatal(e)
	}
}
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestRenderPreparatoryTimeoutResume(t *testing.T) {
	var mu sync.Mutex
	posts := []bridge.Request{}
	fixtureBridge(t, func(req bridge.Request) {
		mu.Lock()
		posts = append(posts, req)
		number := len(posts)
		mu.Unlock()
		if number == 2 {
			code, e := os.ReadFile(req.File)
			if e != nil {
				t.Error(e)
				return
			}
			match := regexp.MustCompile(`filePath: ("[^"\n]+")`).FindSubmatch(code)
			if match == nil {
				t.Error("missing render staging")
				return
			}
			var dir string
			if e = json.Unmarshal(match[1], &dir); e != nil {
				t.Error(e)
				return
			}
			if e = os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("fixture video"), 0600); e != nil {
				t.Error(e)
			}
			fixtureResult(t, req.ID, map[string]any{"ms": 4})
		}
	})
	bin := t.TempDir()
	if e := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte("#!/bin/sh\necho 3\n"), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := filepath.Join(t.TempDir(), "result.mp4")
	a := &app{json: true}
	code := a.dispatch([]string{"render", "-o", out, "--timeout", "30ms"})
	if code != exitStillRunning {
		t.Fatalf("exit=%d", code)
	}
	r, e := operation.Load(config.Home(), a.op.ID)
	if e != nil {
		t.Fatal(e)
	}
	if r.Phase != "waiting-for-metadata" || len(r.Jobs) != 1 || r.Jobs[0].State != bridge.StateQueued {
		t.Fatalf("record: %+v", r)
	}
	fixtureResult(t, r.Jobs[0].ID, map[string]any{"scenePath": "scratch.cv", "comp": map[string]any{"id": "compNode#1", "name": "scratch", "fps": 30, "startFrame": 0, "endFrame": 2}})
	// Raw job wait finishes only metadata and MUST NOT launch the render.
	raw := &app{json: true}
	if c := raw.dispatch([]string{"job", "wait", r.Jobs[0].ID, "--timeout", "100ms"}); c != 0 {
		t.Fatalf("raw wait=%d", c)
	}
	mu.Lock()
	n := len(posts)
	mu.Unlock()
	if n != 1 {
		t.Fatal("raw wait submitted render")
	}
	resumed := &app{json: true}
	if c := resumed.dispatch([]string{"operation", "resume", r.ID, "--timeout", "2s"}); c != 0 {
		t.Fatalf("resume=%d", c)
	}
	mu.Lock()
	n = len(posts)
	mu.Unlock()
	if n != 2 {
		t.Fatalf("submissions=%d", n)
	}
	b, e := os.ReadFile(out)
	if e != nil || string(b) != "fixture video" {
		t.Fatalf("output %q %v", b, e)
	}
	r, e = operation.Load(config.Home(), r.ID)
	if e != nil || r.Status != "complete" || len(r.Jobs) != 2 {
		t.Fatalf("completion: %+v %v", r, e)
	}
	if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID}); c != 0 {
		t.Fatalf("complete resume=%d", c)
	}
	mu.Lock()
	n = len(posts)
	mu.Unlock()
	if n != 2 {
		t.Fatal("complete resume duplicated submission")
	}
}
func TestUncertainSubmissionNeverReposted(t *testing.T) {
	var mu sync.Mutex
	var requests []bridge.Request
	fixtureBridge(t, func(req bridge.Request) {
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		time.Sleep(80 * time.Millisecond)
	})
	a := &app{json: true}
	if c := a.dispatch([]string{"run", "-e", "return 42", "--no-helpers", "--timeout", "20ms"}); c != exitStillRunning {
		t.Fatalf("exit %d", c)
	}
	r, e := operation.Load(config.Home(), a.op.ID)
	if e != nil {
		t.Fatal(e)
	}
	if r.Jobs[0].Submission != "uncertain" || r.Status != "unknown" {
		t.Fatalf("record %+v", r)
	}
	fixtureResult(t, r.Jobs[0].ID, 42)
	if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID, "--timeout", "1s"}); c != 0 {
		t.Fatalf("resume %d", c)
	}
	mu.Lock()
	n := len(requests)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("reposted %d", n)
	}
}
func TestOperationStatusDoesNotSubmit(t *testing.T) {
	posts := 0
	fixtureBridge(t, func(req bridge.Request) { posts++ })
	r := &operation.Record{Schema: 1, ID: "status-fixture", Command: []string{"render"}, Host: config.Host(), Port: config.Port(), Status: "queued", Phase: "waiting-for-metadata", Jobs: []*operation.Job{{ID: "missing", Submission: "accepted", State: bridge.StateQueued}}}
	if e := operation.Save(config.Home(), r); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(config.Home(), "operations", r.ID+".json"))
	if c := (&app{json: true}).dispatch([]string{"operation", "status", r.ID}); c != 0 {
		t.Fatal(c)
	}
	if c := (&app{json: true}).dispatch([]string{"status"}); c != 0 {
		t.Fatal(c)
	}
	after, _ := os.ReadFile(filepath.Join(config.Home(), "operations", r.ID+".json"))
	if posts != 0 || string(before) != string(after) {
		t.Fatal("status mutated state or submitted work")
	}
	s, _, e := bridge.New().Inspect(context.Background(), "missing")
	if e != nil || s != bridge.StateUnknown {
		t.Fatalf("state=%s %v", s, e)
	}
}
func TestOperationLockAndPreparedRecovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAV_HOME", home)
	unlock, e := operation.Lock(home)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = operation.Lock(home); e == nil {
		t.Fatal("second client acquired lock")
	}
	unlock()
	unlock, e = operation.Lock(home)
	if e != nil {
		t.Fatal(e)
	}
	unlock()
	if _, e = operation.Load(home, "../escape"); e == nil {
		t.Fatal("unsafe id")
	}
	if d := operationTimeout([]string{"render", "--timeout=90m"}); d != 90*time.Minute {
		t.Fatal(d)
	}
}

func TestPendingOperationBlocksNewSubmissions(t *testing.T) {
	posts := 0
	fixtureBridge(t, func(req bridge.Request) { posts++ })
	a := &app{json: true}
	if c := a.dispatch([]string{"run", "-e", "return 1", "--async", "--no-helpers"}); c != 0 {
		t.Fatal(c)
	}
	if c := (&app{json: true}).dispatch([]string{"run", "-e", "return 2", "--async", "--no-helpers"}); c != exitStillRunning {
		t.Fatalf("new client exit %d", c)
	}
	if posts != 1 {
		t.Fatalf("queued duplicate work: %d", posts)
	}
	if c := (&app{json: true}).dispatch([]string{"operation", "abandon", a.op.ID}); c != exitError {
		t.Fatalf("unacknowledged abandonment %d", c)
	}
	if c := (&app{json: true}).dispatch([]string{"operation", "abandon", a.op.ID, "--acknowledge-unknown-outcome"}); c != 0 {
		t.Fatal(c)
	}
	r, e := operation.Load(config.Home(), a.op.ID)
	if e != nil || r.Status != "abandoned" || len(r.Jobs) != 1 {
		t.Fatalf("abandon: %v %v", r, e)
	}
}
func TestPreparedCheckpointAndChangedSession(t *testing.T) {
	posts := 0
	fixtureBridge(t, func(req bridge.Request) { posts++; fixtureResult(t, req.ID, 7) })
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	r := &operation.Record{Schema: 1, ID: "prepared-fixture", Command: []string{"run", "-e", "return 7", "--no-helpers"}, Cwd: cwd, Host: config.Host(), Port: config.Port(), Phase: "execution", Status: "unknown", Session: "previous-window", Jobs: []*operation.Job{{ID: "prepared-job", Code: "return 7", Phase: "execution", Submission: "prepared", State: bridge.StateUnknown}}}
	if e = operation.Save(config.Home(), r); e != nil {
		t.Fatal(e)
	}
	if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID, "--timeout", "1s"}); c != exitError {
		t.Fatalf("changed session exit %d", c)
	}
	if posts != 0 {
		t.Fatal("submitted into changed scene session")
	}
	r.Session = ""
	if e = operation.Save(config.Home(), r); e != nil {
		t.Fatal(e)
	}
	if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID, "--timeout", "1s"}); c != 0 {
		t.Fatal(c)
	}
	if posts != 1 {
		t.Fatal("prepared work did not resume exactly once")
	}
}
func TestAsyncEqualsFlagResumesWithoutReadingChangedInput(t *testing.T) {
	posts := 0
	fixtureBridge(t, func(req bridge.Request) { posts++ })
	a := &app{json: true}
	if c := a.dispatch([]string{"run", "-e", "return 1", "--async=true", "--no-helpers"}); c != 0 {
		t.Fatal(c)
	}
	fixtureResult(t, a.op.Jobs[0].ID, 1)
	if c := (&app{json: true}).dispatch([]string{"operation", "resume", a.op.ID, "--timeout", "1s"}); c != 0 {
		t.Fatal(c)
	}
	if posts != 1 {
		t.Fatal("async replay submitted twice")
	}
}

func TestRecoveryLogKeepsJobTelemetry(t *testing.T) {
	fixtureBridge(t, func(req bridge.Request) { fixtureResult(t, req.ID, 42) })
	a := &app{json: true}
	if c := a.dispatch([]string{"run", "-e", "return 42", "--no-helpers"}); c != 0 {
		t.Fatal(c)
	}
	if a.logJob["job"] != a.op.Jobs[0].ID || a.logJob["jobOk"] != true || a.logJob["operationStatus"] != "complete" {
		t.Fatalf("lost job telemetry: %v", a.logJob)
	}
}
func TestRecoveredRenderRejectsChangedAudio(t *testing.T) {
	posts := 0
	fixtureBridge(t, func(req bridge.Request) { posts++ })
	audio := filepath.Join(t.TempDir(), "audio.wav")
	if e := os.WriteFile(audio, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	a := &app{json: true}
	out := filepath.Join(t.TempDir(), "video.mp4")
	if c := a.dispatch([]string{"render", "-o", out, "--audio", audio, "--timeout", "100ms"}); c != exitStillRunning {
		t.Fatal(c)
	}
	if len(a.op.IntendedOutputs) != 1 || a.op.IntendedOutputs[0] != out {
		t.Fatalf("lost intended output: %v", a.op)
	}
	if e := os.WriteFile(audio, []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	if c := (&app{json: true}).dispatch([]string{"operation", "resume", a.op.ID, "--timeout", "1s"}); c != exitError {
		t.Fatalf("changed audio accepted: %d", c)
	}
	if posts != 1 {
		t.Fatalf("changed input submitted %d jobs", posts)
	}
}
