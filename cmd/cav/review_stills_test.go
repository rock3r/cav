package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/review"
)

// stillsProject writes a storyboard of two shots, with a frame each, and returns its path.
func stillsProject(t *testing.T) string {
	t.Helper()
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_OUT_DIR", "")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "board"), 0o755); err != nil {
		t.Fatal(err)
	}
	sb := &board.Board{BPM: 120, Shots: []board.Shot{
		{ID: "s1", Beats: [2]float64{0, 4}, What: "logo", Frame: "board/s1.png"},
		{ID: "s2", Beats: [2]float64{4, 8}, What: "tagline", Frame: "board/s2.png"},
	}}
	path := filepath.Join(dir, "storyboard.json")
	if err := sb.Save(path); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s1", "s2"} {
		if err := os.WriteFile(filepath.Join(dir, "board", id+".png"), []byte("frame "+id), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// cav runs a command in this process and returns its exit code and what it printed.
func cav(t *testing.T, json bool, args ...string) (int, string) {
	t.Helper()
	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	code := (&app{json: json}).dispatch(args)
	os.Stdout = stdout
	w.Close()
	return code, <-done
}

// approveAll writes what the page writes when the person approves: the approval with the
// hash of every item, and the send.
func approveAll(t *testing.T, path string) {
	t.Helper()
	items, err := review.Items(review.KindStoryboard, path)
	if err != nil {
		t.Fatal(err)
	}
	shas := map[string]string{}
	for _, it := range items {
		shas[it.ID] = it.SHA
	}
	now := time.Now().UTC()
	_, err = review.Open(path).Update(func(d *review.Doc) error {
		d.Kind = "storyboard"
		d.Approved = &review.Approval{By: "seb", At: now, Items: shas}
		d.Sends = append(d.Sends, review.Send{N: len(d.Sends) + 1, At: now, Comments: []string{}, Approved: true})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func itemSHA(t *testing.T, path, id string) string {
	t.Helper()
	items, err := review.Items(review.KindStoryboard, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID == id {
			return it.SHA
		}
	}
	t.Fatalf("no item %s", id)
	return ""
}

func TestReviewWaitPrintsItemNotesAndApproval(t *testing.T) {
	path := stillsProject(t)
	now := time.Now().UTC()
	oldSHA := itemSHA(t, path, "s2")
	_, err := review.Open(path).Update(func(d *review.Doc) error {
		d.Kind = "storyboard"
		d.Comments = []review.Comment{{ID: "c_01", Status: "open", Author: "seb", CreatedAt: now, Item: "s2", ItemSHA: oldSHA, Verdict: "redo",
			Text: "the tagline is too small", Fragment: "#xywh=percent:20,20,40,20", Shapes: []review.Shape{{Type: "rect"}}, Sent: 1}}
		d.Sends = []review.Send{{N: 1, At: now, Comments: []string{"c_01"}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The frame changes after the note was made.
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "board", "s2.png"), []byte("a fixed frame"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := cav(t, false, "review", "wait", path, "--timeout", "5s")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, want := range []string{"send #1 from the review of", "c_01  open  s2 · redo  by seb [item changed since]", "the tagline is too small",
		"drawing: rect (#xywh=percent:20,20,40,20)", "Fix them in " + path, "cav review resolve <id>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if code, _ := cav(t, false, "review", "wait", path, "--timeout", "1s"); code != exitStillRunning {
		t.Fatalf("a received send must not come twice: exit %d", code)
	}

	// The person approves: wait returns even with no notes.
	_, err = review.Open(path).Update(func(d *review.Doc) error {
		d.Comments[0].Status = "resolved"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	approveAll(t, path)
	code, out = cav(t, false, "review", "wait", path, "--timeout", "5s")
	if code != exitOK || !strings.Contains(out, "is approved by seb") || !strings.Contains(out, "next stage") {
		t.Fatalf("approval: exit %d\n%s", code, out)
	}
	approveAll(t, path)
	code, out = cav(t, true, "review", "wait", path, "--timeout", "5s")
	var got struct {
		Approved   bool
		ApprovedBy string
		Kind       string
	}
	if json.Unmarshal([]byte(out), &got) != nil || code != exitOK || !got.Approved || got.ApprovedBy != "seb" || got.Kind != "storyboard" {
		t.Fatalf("json: exit %d %s", code, out)
	}
}

func TestReviewExportStillsJSON(t *testing.T) {
	path := stillsProject(t)
	type exported struct {
		Kind  string
		Items []struct {
			ID, Title, SHA, Verdict string
			Outdated                bool
			OpenNotes               int
		}
		Approved        bool
		ApprovalCleared *review.Cleared
		Blockers        []string
		Notes           []json.RawMessage
	}
	export := func() exported {
		code, out := cav(t, true, "review", "export", path)
		var e exported
		if err := json.Unmarshal([]byte(out), &e); err != nil || code != exitOK {
			t.Fatalf("export: exit %d %v\n%s", code, err, out)
		}
		return e
	}
	e := export()
	if e.Kind != "storyboard" || len(e.Items) != 3 || e.Approved || len(e.Blockers) != 0 || e.Items[0].SHA == "" {
		t.Fatalf("fresh export %+v", e)
	}

	_, err := review.Open(path).Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments, review.Comment{ID: "c_01", Status: "open", Item: "s1", ItemSHA: itemSHA(t, path, "s1"), Verdict: "redo", CreatedAt: time.Now()})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	e = export()
	if len(e.Blockers) != 1 || e.Blockers[0] != "s1 is marked redo" || e.Items[1].Verdict != "redo" {
		t.Fatalf("export with a redo %+v", e)
	}
	if _, out := cav(t, false, "review", "export", path); !strings.Contains(out, "not approved: s1 is marked redo") || !strings.Contains(out, "s1  redo") {
		t.Fatalf("human export:\n%s", out)
	}

	// Approved, then a frame changes: the export shows the approval removed, and why.
	review.Open(path).Update(func(d *review.Doc) error { d.Comments = nil; return nil })
	approveAll(t, path)
	if e = export(); !e.Approved {
		t.Fatalf("approved export %+v", e)
	}
	if _, out := cav(t, false, "review", "export", path); !strings.Contains(out, "approved by seb on") {
		t.Fatalf("human export:\n%s", out)
	}
	os.WriteFile(filepath.Join(filepath.Dir(path), "board", "s1.png"), []byte("another frame"), 0o644)
	if e = export(); e.Approved || e.ApprovalCleared == nil || !strings.Contains(e.ApprovalCleared.Reason, "s1") {
		t.Fatalf("export after a change %+v", e)
	}
}

func TestReopenAfterApprovalClearsIt(t *testing.T) {
	path := stillsProject(t)
	review.Open(path).Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments, review.Comment{ID: "c_01", Status: "resolved", Item: "s1", ItemSHA: itemSHA(t, path, "s1"), Text: "kerning", CreatedAt: time.Now()})
		return nil
	})
	approveAll(t, path)
	if code, out := cav(t, false, "review", "reopen", "c_01", path); code != exitOK {
		t.Fatalf("reopen: %d %s", code, out)
	}
	d, err := review.Open(path).Load()
	if err != nil || d.Approved != nil || d.ApprovalCleared == nil || d.ApprovalCleared.Reason != "c_01 is open" {
		t.Fatalf("after reopening: %+v %v", d, err)
	}
	if code, _ := cav(t, false, "review", "resolve", "c_01", "--note", "fixed", path); code != exitOK {
		t.Fatal("resolve")
	}
}

func TestWaitAndExportNeverSayApprovedForATargetThatDoesNotLoad(t *testing.T) {
	path := stillsProject(t)
	approveAll(t, path)
	// A storyboard in the middle of an edit: valid JSON, but it fails the board's own checks.
	if err := os.WriteFile(path, []byte(`{"schema":"cav-board/1","shots":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := cav(t, false, "review", "export", path)
	if code != exitOK || !strings.Contains(out, "approval unknown: "+path+" does not load") || strings.Contains(out, "approved by") {
		t.Fatalf("export: exit %d\n%s", code, out)
	}
	code, out = cav(t, true, "review", "export", path)
	var e struct {
		Approved        bool
		ApprovalUnknown bool
		ItemsError      string
	}
	if json.Unmarshal([]byte(out), &e) != nil || code != exitOK || e.Approved || !e.ApprovalUnknown || e.ItemsError == "" {
		t.Fatalf("export json: exit %d\n%s", code, out)
	}
	code, out = cav(t, false, "review", "wait", path, "--timeout", "3s")
	if code != exitOK || !strings.Contains(out, "approval unknown") || strings.Contains(out, "is approved by") || strings.Contains(out, "Go on to the next stage") || !strings.Contains(out, "Do not go on") {
		t.Fatalf("wait: exit %d\n%s", code, out)
	}
}

func TestReviewTakesOnlyKnownJSONTargets(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	dir := t.TempDir()
	p := filepath.Join(dir, "notes.json")
	os.WriteFile(p, []byte(`{}`), 0o644)
	if code, _ := cav(t, false, "review", "export", p); code == exitOK {
		t.Fatal("notes.json is not a review target")
	}
}
