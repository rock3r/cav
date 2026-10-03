package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/operation"
)

func renderFixtureStage(t *testing.T, req bridge.Request) string {
	t.Helper()
	b, err := os.ReadFile(req.File)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`filePath: ("[^"\n]+")`).FindSubmatch(b)
	if len(m) != 2 {
		t.Fatal("no staged render directory")
	}
	var dir string
	if err := json.Unmarshal(m[1], &dir); err != nil {
		t.Fatal(err)
	}
	return dir
}
func renderMetadataFixture(t *testing.T, id string) {
	fixtureResult(t, id, map[string]any{"scenePath": "scratch.cv", "comp": map[string]any{"id": "compNode#1", "name": "scratch", "fps": 30, "startFrame": 0, "endFrame": 5}})
}
func TestLostRenderPreservesPartialOutputWithoutRetry(t *testing.T) {
	for _, reason := range []string{"bridge-disconnected", "bridge-session-changed"} {
		t.Run(reason, func(t *testing.T) {
			var mu sync.Mutex
			var posts []bridge.Request
			var server *httptest.Server
			server = fixtureBridgeWithGet(t, func(req bridge.Request) {
				mu.Lock()
				posts = append(posts, req)
				n := len(posts)
				mu.Unlock()
				if n == 1 {
					renderMetadataFixture(t, req.ID)
					return
				}
				stage := renderFixtureStage(t, req)
				if err := os.WriteFile(filepath.Join(stage, "video.mp4"), []byte("partial video"), 0600); err != nil {
					t.Error(err)
				}
			}, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				n := len(posts)
				job := ""
				if n > 0 {
					job = posts[n-1].ID
				}
				mu.Unlock()
				if n < 2 {
					fmt.Fprint(w, `{"type":"hello","bridgeSession":"original"}`)
					return
				}
				if reason == "bridge-session-changed" {
					fmt.Fprint(w, `{"type":"hello","bridgeSession":"restarted"}`)
					return
				}
				w.Header().Set("Connection", "close")
				fmt.Fprintf(w, `{"type":"running","id":%q,"bridgeSession":"original"}`, job)
				// Close only this disposable listener. Existing reply completes; the next GET
				// is refused exactly as when a native app/bridge exits.
				_ = server.Listener.Close()
			})
			out := filepath.Join(t.TempDir(), "final.mp4")
			a := &app{json: true}
			if code := a.dispatch([]string{"render", "-o", out, "--timeout", "2s"}); code != exitLost {
				t.Fatalf("render exit=%d", code)
			}
			record, err := operation.Load(config.Home(), a.op.ID)
			if err != nil {
				t.Fatal(err)
			}
			if record.Status != "unknown" || record.FailureReason != reason || record.Session != "original" {
				t.Fatalf("record=%+v", record)
			}
			progress := renderProgress(record)
			if progress["expectedFrames"] != 6 || len(progress["artifacts"].([]map[string]any)) != 1 {
				t.Fatalf("progress=%v", progress)
			}
			artifact := progress["artifacts"].([]map[string]any)[0]
			if artifact["bytes"] != int64(len("partial video")) || artifact["validation"] != "unvalidated" {
				t.Fatal(artifact)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("partial video was published: %v", err)
			}
			before, _ := os.ReadFile(filepath.Join(config.Home(), "operations", record.ID+".json"))
			if code := (&app{json: true}).dispatch([]string{"operation", "status", record.ID}); code != 0 {
				t.Fatal(code)
			}
			after, _ := os.ReadFile(filepath.Join(config.Home(), "operations", record.ID+".json"))
			if string(before) != string(after) {
				t.Fatal("status changed the checkpoint")
			}
			if code := (&app{json: true}).dispatch([]string{"operation", "resume", record.ID, "--timeout", "2s"}); code != exitLost {
				t.Fatalf("resume exit=%d", code)
			}
			mu.Lock()
			n := len(posts)
			mu.Unlock()
			if n != 2 {
				t.Fatalf("recovery resubmitted native render: posts=%d", n)
			}
			data, err := os.ReadFile(artifact["file"].(string))
			if err != nil || string(data) != "partial video" {
				t.Fatalf("partial output changed: %q %v", data, err)
			}
		})
	}
}
func TestShortVideoRejectedEvenWhenBridgeReturnsSuccess(t *testing.T) {
	count := 0
	fixtureBridge(t, func(req bridge.Request) {
		count++
		if count == 1 {
			renderMetadataFixture(t, req.ID)
			return
		}
		stage := renderFixtureStage(t, req)
		if err := os.WriteFile(filepath.Join(stage, "video.mp4"), []byte("short video"), 0600); err != nil {
			t.Error(err)
		}
		fixtureResult(t, req.ID, map[string]any{"ms": 3})
	})
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte("#!/bin/sh\necho 2\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := filepath.Join(t.TempDir(), "final.mp4")
	a := &app{json: true}
	if code := a.dispatch([]string{"render", "-o", out, "--timeout", "2s"}); code != exitError {
		t.Fatal(code)
	}
	if a.op.Status != "failed" || a.op.Phase != "validation" {
		t.Fatalf("record=%+v", a.op)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("short video published")
	}
	stage := a.op.Render.Stage
	before, err := os.ReadFile(filepath.Join(stage, "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if code := (&app{json: true}).dispatch([]string{"operation", "resume", a.op.ID, "--timeout", "2s"}); code != exitError {
		t.Fatal(code)
	}
	after, err := os.ReadFile(filepath.Join(stage, "video.mp4"))
	if err != nil || string(before) != string(after) || count != 2 {
		t.Fatalf("retry changed output or resubmitted: count=%d err=%v", count, err)
	}
}
func TestStaleFileActivityDoesNotInferCrashOrCompletion(t *testing.T) {
	stage := t.TempDir()
	file := filepath.Join(stage, "video.mp4")
	if err := os.WriteFile(file, []byte("unvalidated"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	r := &operation.Record{Status: "running", Render: &operation.Render{Stage: stage, ExpectedFrames: 100, FPS: 30}}
	artifact := renderProgress(r)["artifacts"].([]map[string]any)[0]
	if r.Status != "running" || artifact["validation"] != "unvalidated" || artifact["secondsSinceModification"].(float64) < 3600 {
		t.Fatalf("status=%s artifact=%v", r.Status, artifact)
	}
}

func TestRealMP4ValidationRejectsShortOrTruncatedRender(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("real MP4 fixture requires ffmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("real MP4 fixture requires ffprobe")
	}
	for _, tc := range []struct {
		name     string
		frames   int
		truncate bool
		want     int
	}{
		{"complete", 6, false, exitOK},
		{"short", 2, false, exitError},
		{"truncated", 6, true, exitError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "fixture.mp4")
			// A tiny software-only file fixture, with no Cavalry or scene access.
			cmd := exec.CommandContext(t.Context(), ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=c=black:s=16x16:r=30", "-frames:v", fmt.Sprint(tc.frames), "-c:v", "mpeg4", source)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fixture encode: %s: %v", output, err)
			}
			video, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if tc.truncate {
				video = video[:len(video)/2]
			}
			posts := 0
			fixtureBridge(t, func(req bridge.Request) {
				posts++
				if posts == 1 {
					renderMetadataFixture(t, req.ID)
					return
				}
				if err := os.WriteFile(filepath.Join(renderFixtureStage(t, req), "video.mp4"), video, 0600); err != nil {
					t.Error(err)
				}
				fixtureResult(t, req.ID, map[string]any{"ms": 1})
			})
			out := filepath.Join(t.TempDir(), "final.mp4")
			a := &app{json: true}
			if code := a.dispatch([]string{"render", "-o", out, "--timeout", "10s"}); code != tc.want {
				t.Fatalf("exit=%d want=%d", code, tc.want)
			}
			_, err = os.Stat(out)
			if tc.want == exitOK && err != nil {
				t.Fatal(err)
			}
			if tc.want != exitOK && !os.IsNotExist(err) {
				t.Fatalf("invalid output published: %v", err)
			}
		})
	}
}
