package review

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/board"
)

const testNonce = "test-nonce-0123456789abcdef"

func TestApproveNeedsTheNonce(t *testing.T) {
	s, h, _, _ := newStillsServer(t)
	s.Page = []byte(`<html><meta name="cav-nonce" content="__CAV_NONCE__">`)
	st := getState(t, h)
	for name, body := range map[string]map[string]any{
		"no nonce":    {"items": st.shas()},
		"wrong nonce": {"items": st.shas(), "nonce": "guess"},
		"withdraw":    {"withdraw": true},
	} {
		if w := do(t, h, "POST", "/api/approve", body, nil); w.Code != 403 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if d, _ := s.Store.Load(); d.Approved != nil || len(d.Sends) != 0 {
		t.Fatalf("a refused request changed the review: %+v", d)
	}

	// A server with no nonce refuses every approval.
	s.Nonce = ""
	if w := do(t, h, "POST", "/api/approve", map[string]any{"items": st.shas(), "nonce": ""}, nil); w.Code != 403 {
		t.Errorf("no server nonce: %d", w.Code)
	}
	s.Nonce = testNonce

	// The page carries the nonce; nothing else does.
	if w := do(t, h, "GET", "/", nil, nil); !strings.Contains(w.Body.String(), `content="`+testNonce+`"`) {
		t.Errorf("the page should carry the nonce: %s", w.Body)
	}
	approveStage(t, h, st, 200)
	if w := do(t, h, "GET", "/api/state", nil, nil); strings.Contains(w.Body.String(), testNonce) {
		t.Error("/api/state must not show the nonce")
	}
	if b, _ := os.ReadFile(s.Store.DocPath); strings.Contains(string(b), testNonce) {
		t.Error("the review file must not hold the nonce")
	}
}

func TestApproveRefusesAFileWithNoItems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "moodboard.json")
	writeFile(t, path, `{"schema":"cav-moodboard/1","items":[]}`)
	s := &Server{Store: Open(path), Stills: &Stills{Kind: KindMoodboard, Path: path}, Author: "seb", Nonce: testNonce}
	h := s.Handler()
	if w := do(t, h, "POST", "/api/approve", map[string]any{"items": map[string]string{}, "nonce": testNonce}, nil); w.Code != 409 {
		t.Fatalf("approving nothing: %d %s", w.Code, w.Body)
	}
	if st := getState(t, h); st.Approval.CanApprove {
		t.Error("the page must not offer to approve nothing")
	}
}

func TestStillsRefuseOtherOrigins(t *testing.T) {
	_, h, _, _ := newStillsServer(t)
	st := getState(t, h)
	evil := map[string]string{"Origin": "http://evil.example"}
	for name, req := range map[string]struct {
		path string
		body map[string]any
	}{
		"approve":  {"/api/approve", map[string]any{"items": st.shas(), "nonce": testNonce}},
		"withdraw": {"/api/approve", map[string]any{"withdraw": true, "nonce": testNonce}},
		"verdict":  {"/api/comments", map[string]any{"item": "s1", "verdict": "redo"}},
		"note":     {"/api/comments", map[string]any{"item": "s1", "text": "hi"}},
	} {
		if w := do(t, h, "POST", req.path, req.body, evil); w.Code != http.StatusForbidden {
			t.Errorf("%s from another origin: %d %s", name, w.Code, w.Body)
		}
	}
	if st = getState(t, h); st.Approval.Approved || len(st.Approval.Blockers) != 0 {
		t.Errorf("a refused request changed the review: %+v", st.Approval)
	}
}

func TestSnapshotNamesAndDeletesStayInTheReviewFolder(t *testing.T) {
	s, h, _, dir := newStillsServer(t)
	// A bare verdict has nothing to show: a snapshot sent with it is dropped.
	c := postComment(t, h, map[string]any{"item": "s1", "verdict": "redo", "snapshot": pngDataURL(t)}, 200)
	if c.Snapshot != "" {
		t.Fatalf("a bare verdict kept a snapshot: %+v", c)
	}
	// A review file edited by hand can name any id and any snapshot path.
	victim := filepath.Join(dir, "victim.png")
	writeFile(t, victim, "keep me")
	if _, err := s.Store.Update(func(d *Doc) error {
		d.Comments = append(d.Comments, Comment{ID: "c_77", Item: "s2", ItemSHA: d.Comments[0].ItemSHA, Text: "x", Status: "open", Snapshot: victim})
		d.Comments[0].ID = "../../escape" // the bare verdict on s1
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if w := do(t, h, "DELETE", "/api/comments/c_77", nil, nil); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if b, err := os.ReadFile(victim); err != nil || string(b) != "keep me" {
		t.Fatalf("deleting a note removed a file outside the review folder: %v", err)
	}
	// A new bare verdict takes over the edited id. It has no snapshot, so no file is written
	// under that name; a note with a drawing gets a fresh id and stays in the review folder.
	again := postComment(t, h, map[string]any{"item": "s1", "verdict": "approve", "snapshot": pngDataURL(t)}, 200)
	n := postComment(t, h, map[string]any{"item": "s2", "text": "x", "snapshot": pngDataURL(t)}, 200)
	if again.Snapshot != "" || filepath.Dir(n.Snapshot) != s.Store.SnapDir {
		t.Fatalf("snapshots: %q, %q", again.Snapshot, n.Snapshot)
	}
	if itemFileName("../../x") != ".._.._x" {
		t.Fatalf("itemFileName: %s", itemFileName("../../x"))
	}
}

func TestApprovalClearedByAChangeToTheBoardSettings(t *testing.T) {
	_, h, path, _ := newStillsServer(t)
	approveStage(t, h, getState(t, h), 200)
	sb, _ := board.Load(path)
	sb.BPM = 100
	sb.Save(path)
	st := getState(t, h)
	if st.Approval.Approved || st.ApprovalCleared == nil || st.ApprovalCleared.Reason != "board changed" {
		t.Fatalf("a new tempo should remove the approval: %+v %+v", st.Approval, st.ApprovalCleared)
	}
}
