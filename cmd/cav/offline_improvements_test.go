package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/operation"
)

func TestSceneOpenResumeNeverOpensTwice(t *testing.T) {
	for _, pending := range []int{2, 3} {
		t.Run(map[int]string{2: "native-open", 3: "follow-up-metadata"}[pending], func(t *testing.T) {
			var mu sync.Mutex
			var posts []bridge.Request
			scene := filepath.Join(t.TempDir(), "large.cv")
			if err := os.WriteFile(scene, []byte("scene fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			fixtureBridge(t, func(req bridge.Request) {
				mu.Lock()
				posts = append(posts, req)
				n := len(posts)
				mu.Unlock()
				if n == pending {
					return
				}
				switch n {
				case 1:
					fixtureResult(t, req.ID, map[string]any{"unsaved": false})
				case 2:
					fixtureResult(t, req.ID, true)
				case 3:
					fixtureResult(t, req.ID, map[string]any{"scenePath": scene})
				}
			})
			a := &app{json: true}
			if c := a.dispatch([]string{"scene", "open", scene, "--timeout", "1s"}); c != exitStillRunning {
				t.Fatalf("exit %d", c)
			}
			r, err := operation.Load(config.Home(), a.op.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Jobs) != pending {
				t.Fatalf("jobs=%d", len(r.Jobs))
			}
			wantPhase := map[int]string{2: "opening-scene", 3: "waiting-for-scene"}[pending]
			if r.Phase != wantPhase {
				t.Fatalf("phase=%s", r.Phase)
			}
			if pending == 2 {
				fixtureResult(t, r.Jobs[pending-1].ID, true)
			} else {
				fixtureResult(t, r.Jobs[pending-1].ID, map[string]any{"scenePath": scene})
			}
			if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID, "--timeout", "2s"}); c != 0 {
				t.Fatalf("resume %d", c)
			}
			mu.Lock()
			n := len(posts)
			mu.Unlock()
			if n != 3 {
				t.Fatalf("native submissions=%d", n)
			}
			if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID}); c != 0 {
				t.Fatalf("complete resume %d", c)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(posts) != 3 {
				t.Fatal("completed resume reposted")
			}
		})
	}
}
func TestSceneOpenChangedInputRefusesResume(t *testing.T) {
	var mu sync.Mutex
	posts := 0
	fixtureBridge(t, func(req bridge.Request) { mu.Lock(); posts++; mu.Unlock() })
	scene := filepath.Join(t.TempDir(), "scene.cv")
	if err := os.WriteFile(scene, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	a := &app{json: true}
	if c := a.dispatch([]string{"scene", "open", scene, "--force", "--timeout", "1s"}); c != exitStillRunning {
		t.Fatal(c)
	}
	if err := os.WriteFile(scene, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if c := (&app{json: true}).dispatch([]string{"operation", "resume", a.op.ID, "--timeout", "1s"}); c != exitError {
		t.Fatalf("changed input exit=%d", c)
	}
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 {
		t.Fatalf("reposted %d", posts)
	}
}
func TestImageTimeoutResumePublishesWithoutRerender(t *testing.T) {
	for _, command := range []string{"frame", "sheet"} {
		t.Run(command, func(t *testing.T) {
			var mu sync.Mutex
			var posts []bridge.Request
			fixtureBridge(t, func(req bridge.Request) {
				mu.Lock()
				posts = append(posts, req)
				n := len(posts)
				mu.Unlock()
				if n == 1 {
					fixtureResult(t, req.ID, map[string]any{"scenePath": "scratch.cv", "frame": 7, "comp": map[string]any{"id": "comp", "startFrame": 0, "endFrame": 2, "fps": 30}})
				}
			})
			out := filepath.Join(t.TempDir(), "result.png")
			if err := os.WriteFile(out, []byte("prior image"), 0600); err != nil {
				t.Fatal(err)
			}
			frames := "0,2"
			if command == "frame" {
				frames = "0"
			}
			a := &app{json: true}
			if c := a.dispatch([]string{command, frames, "-o", out, "--timeout", "1s"}); c != exitStillRunning {
				t.Fatalf("exit=%d", c)
			}
			r, err := operation.Load(config.Home(), a.op.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Jobs) != 2 {
				t.Fatalf("jobs=%d", len(r.Jobs))
			}
			code, err := os.ReadFile(filepath.Join(config.JobsDir(), r.Jobs[1].ID+".js"))
			if err != nil {
				t.Fatal(err)
			}
			m := regexp.MustCompile(`var sample = (\{[^\n]+\});`).FindSubmatch(code)
			if m == nil {
				t.Fatalf("missing sample: %s", code)
			}
			var sample struct {
				Frames []int
				Dir    string
			}
			if err = json.Unmarshal(m[1], &sample); err != nil {
				t.Fatal(err)
			}
			var paths []string
			for _, f := range sample.Frames {
				p := filepath.Join(sample.Dir, fmtFrame(f))
				file, err := os.Create(p)
				if err != nil {
					t.Fatal(err)
				}
				if err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
					t.Fatal(err)
				}
				if err = file.Close(); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, p)
			}
			fixtureResult(t, r.Jobs[1].ID, paths)
			if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID, "--timeout", "2s"}); c != 0 {
				t.Fatalf("resume=%d", c)
			}
			result := out
			file, err := os.Open(result)
			if err != nil {
				t.Fatal(err)
			}
			_, err = png.Decode(file)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			if command == "sheet" {
				if _, err = os.Stat(sample.Dir); !os.IsNotExist(err) {
					t.Fatalf("frames not cleaned up: %v", err)
				}
			}
			// Simulate a crash after publication/cleanup but before the final completion record.
			saved, err := operation.Load(config.Home(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			saved.Status = "failed"
			saved.Phase = "publication-complete"
			if err = operation.Save(config.Home(), saved); err != nil {
				t.Fatal(err)
			}
			if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID}); c != 0 {
				t.Fatalf("publication checkpoint resume=%d", c)
			}
			if c := (&app{json: true}).dispatch([]string{"operation", "resume", r.ID}); c != 0 {
				t.Fatalf("complete resume=%d", c)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(posts) != 2 {
				t.Fatalf("rerendered %d", len(posts))
			}
		})
	}
}
func fmtFrame(f int) string { return fmt.Sprintf("f_%d.png", f) }
func TestProfilePlan(t *testing.T) {
	static := structureData{Start: 0, End: 2000, TotalLayers: intPointer(1), Layers: []structureLayer{{Type: "basicShape"}}}
	sim := static
	sim.Layers = []structureLayer{{Type: "particleShape"}}
	for _, tc := range []struct {
		name, spec string
		data       structureData
		max        int
		want       []int
		warmup     int
		fail       bool
	}{
		{"static targets", "1908,840,840,1560", static, 0, []int{840, 1560, 1908}, 0, false},
		{"simulation targets", "1908,840,1560", sim, 2000, []int{840, 1560, 1908}, 1906, false},
		{"warmup refused", "840,1560,1908", sim, 600, nil, 0, true},
		{"exact allowance", "2,5", sim, 4, []int{2, 5}, 4, false},
		{"zero allowance", "0,1", sim, 0, []int{0, 1}, 0, false},
		{"out of range", "2001", static, 600, nil, 0, true},
		{"huge expansion", "0-9223372036854775807", static, 600, nil, 0, true},
		{"empty selection", ",", static, 600, nil, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := planProfile(tc.data, 3, tc.spec, tc.max)
			if (err != nil) != tc.fail {
				t.Fatalf("plan=%+v err=%v", plan, err)
			}
			if !tc.fail && (!reflect.DeepEqual(plan.Frames, tc.want) || plan.Warmup != tc.warmup) {
				t.Fatalf("plan=%+v", plan)
			}
		})
	}
}
func TestNestedFindingsRetainCompContext(t *testing.T) {
	d := structureData{TotalLayers: intPointer(2), Layers: []structureLayer{{ID: "js", Type: "javaScript", Comp: "child", CompPath: []string{"root", "child"}}, {ID: "dup", Type: "duplicator", Comp: "child", CompPath: []string{"root", "child"}}}, Edges: []structureEdge{{From: "js", To: "dup", ToAttr: "shapePosition.x"}}}
	findings := performanceFindings(d)
	if len(findings) != 1 || findings[0].Comp != "child" || !reflect.DeepEqual(findings[0].CompPath, []string{"root", "child"}) {
		t.Fatalf("findings=%+v", findings)
	}
	incomplete := false
	d.CoverageComplete = &incomplete
	if !needsChronological(d) {
		t.Fatal("incomplete nested coverage allowed seeking")
	}
	d.CoverageComplete = nil
	d.Layers[1].Type = "compositionReference"
	if !needsChronological(d) {
		t.Fatal("legacy uninspected precomp allowed seeking")
	}
}
func TestPublishImageExpiredBudgetPreservesDestination(t *testing.T) {
	src, dst := filepath.Join(t.TempDir(), "src.png"), filepath.Join(t.TempDir(), "dest.png")
	if err := os.WriteFile(src, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := publishImage(ctx, src, dst); err != context.Canceled {
		t.Fatalf("err=%v", err)
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "old" {
		t.Fatalf("destination=%q %v", b, err)
	}
}
func TestRacingResumeClientsSerialize(t *testing.T) {
	var mu sync.Mutex
	var observed sync.Once
	posts := 0
	resuming := false
	entered := make(chan struct{})
	fixtureBridgeWithGet(t, func(req bridge.Request) { mu.Lock(); posts++; mu.Unlock() }, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		watch := resuming
		mu.Unlock()
		// Resume reaches bridge inspection only after it has acquired the operation lock.
		if watch {
			observed.Do(func() { close(entered) })
		}
		fmt.Fprint(w, `{"type":"hello","bridge":"cav-bridge","protocol":1}`)
	})
	a := &app{json: true}
	if c := a.dispatch([]string{"run", "-e", "return 42", "--no-helpers", "--timeout", "1s"}); c != exitStillRunning {
		t.Fatal(c)
	}
	mu.Lock()
	resuming = true
	mu.Unlock()
	done := make(chan int, 1)
	go func() {
		done <- (&app{json: true}).dispatch([]string{"operation", "resume", a.op.ID, "--timeout", "5s"})
	}()
	select {
	case <-entered:
		if c := (&app{json: true}).dispatch([]string{"operation", "resume", a.op.ID, "--timeout", "1s"}); c == 0 {
			t.Error("racing resume was not refused")
		}
	case <-time.After(3 * time.Second):
		t.Error("first resume never reached bridge inspection")
	}
	fixtureResult(t, a.op.Jobs[0].ID, 42)
	if c := <-done; c != 0 {
		t.Errorf("first resume=%d", c)
	}
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 {
		t.Fatalf("reposted %d", posts)
	}
}

func TestTargetedSimulationProfileEnablesPNGAndReportsWarmup(t *testing.T) {
	var mu sync.Mutex
	posts := 0
	fixtureBridge(t, func(req bridge.Request) {
		mu.Lock()
		posts++
		n := posts
		mu.Unlock()
		if n == 1 {
			fixtureResult(t, req.ID, map[string]any{"comp": "comp", "scenePath": "scratch.cv", "start": 0, "end": 10, "totalLayers": 1, "coverageComplete": true, "layers": []any{map[string]any{"id": "sim", "type": "particleShape"}}})
			return
		}
		code, err := os.ReadFile(req.File)
		if err != nil {
			t.Error(err)
			return
		}
		m := regexp.MustCompile(`var sample=(\{[^\n]+\});`).FindSubmatch(code)
		if m == nil {
			t.Errorf("missing profile sample: %s", code)
			return
		}
		var sample struct {
			Frames              []int
			Chronological       bool
			WarmupCount         int
			Images, WarmupImage string
		}
		if err = json.Unmarshal(m[1], &sample); err != nil {
			t.Error(err)
			return
		}
		if !sample.Chronological || sample.WarmupCount != 4 || !reflect.DeepEqual(sample.Frames, []int{2, 5}) || sample.Images == "" || sample.WarmupImage == "" {
			t.Errorf("sample=%+v", sample)
		}
		fixtureResult(t, req.ID, map[string]any{"warmup": map[string]any{"frames": 4, "planned": 4, "ms": 77, "rendered": true}, "profile": []any{map[string]any{"frame": 2, "setFrameMs": 1, "renderPNGMs": 2}, map[string]any{"frame": 5, "setFrameMs": 3, "renderPNGMs": 4}}, "restored": true})
	})
	a := &app{json: true}
	if c := a.dispatch([]string{"check", "--profile-frames", "5,2", "--max-warmup", "4", "--timeout", "2s"}); c != 0 {
		t.Fatalf("exit=%d", c)
	}
	var data struct {
		Warmup  profileWarmup
		Profile []profileResult
	}
	if err := json.Unmarshal(a.op.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Warmup.MS != 77 || data.Warmup.Frames != 4 || len(data.Profile) != 2 || data.Profile[0].RenderPNGMS == nil {
		t.Fatalf("profile=%+v", data)
	}
	mu.Lock()
	defer mu.Unlock()
	if posts != 2 {
		t.Fatalf("submissions=%d", posts)
	}
}
