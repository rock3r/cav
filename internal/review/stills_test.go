package review

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/board"
)

func newStillsServer(t *testing.T) (*Server, http.Handler, string, string) {
	t.Helper()
	path, dir := writeBoard(t)
	s := &Server{Store: Open(path), Stills: &Stills{Kind: KindStoryboard, Path: path}, Author: "seb", Page: []byte("<html>")}
	return s, s.Handler(), path, dir
}

type stillsState struct {
	Kind     string
	Items    []map[string]any
	Verdicts []string
	Approval struct {
		Approved   bool
		CanApprove bool
		Blockers   []string
	}
	ApprovalCleared *Cleared
	TargetError     string
}

func getState(t *testing.T, h http.Handler) stillsState {
	t.Helper()
	w := do(t, h, "GET", "/api/state", nil, nil)
	if w.Code != 200 {
		t.Fatalf("state: %d %s", w.Code, w.Body)
	}
	var st stillsState
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func (st stillsState) item(id string) map[string]any {
	for _, it := range st.Items {
		if it["id"] == id {
			return it
		}
	}
	return nil
}

// shas is what the page sends to approve: the hash of every item it shows.
func (st stillsState) shas() map[string]string {
	m := map[string]string{}
	for _, it := range st.Items {
		m[it["id"].(string)] = it["sha"].(string)
	}
	return m
}

func postComment(t *testing.T, h http.Handler, body map[string]any, want int) Comment {
	t.Helper()
	w := do(t, h, "POST", "/api/comments", body, nil)
	if w.Code != want {
		t.Fatalf("comment %v: %d %s (want %d)", body, w.Code, w.Body, want)
	}
	var c Comment
	json.Unmarshal(w.Body.Bytes(), &c)
	return c
}

func approveStage(t *testing.T, h http.Handler, st stillsState, want int) {
	t.Helper()
	w := do(t, h, "POST", "/api/approve", map[string]any{"items": st.shas()}, nil)
	if w.Code != want {
		t.Fatalf("approve: %d %s (want %d)", w.Code, w.Body, want)
	}
}

func TestStillsVerdictsAndNotes(t *testing.T) {
	s, h, _, _ := newStillsServer(t)
	st := getState(t, h)
	if st.Kind != "storyboard" || len(st.Items) != 3 || strings.Join(st.Verdicts, ",") != "approve,redo" {
		t.Fatalf("state %+v", st)
	}

	redo := postComment(t, h, map[string]any{"item": "s1", "verdict": "redo"}, 200)
	if redo.ID != "c_01" || redo.Status != "open" || redo.Frame != 0 || redo.Timecode != "" || redo.ItemSHA != st.shas()["s1"] {
		t.Fatalf("bare redo %+v", redo)
	}
	ok := postComment(t, h, map[string]any{"item": "s2", "verdict": "approve"}, 200)
	if ok.Status != "resolved" || ok.ResolvedAt == nil {
		t.Fatalf("a bare approve asks for nothing: %+v", ok)
	}

	// A note with a drawing keeps its snapshot under the item's name.
	note := postComment(t, h, map[string]any{"item": "s2", "text": "tagline too small", "snapshot": pngDataURL(t),
		"shapes": []Shape{{Type: "rect", Pts: [][2]float64{{0.2, 0.2}, {0.6, 0.4}}}}}, 200)
	if note.Status != "open" || note.Fragment != "#xywh=percent:20,20,40,20" || filepath.Base(note.Snapshot) != note.ID+"_s2.png" {
		t.Fatalf("note %+v", note)
	}
	if _, err := os.Stat(note.Snapshot); err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
	if got := s.Store.SnapshotFile(note.Snapshot); got == "" {
		t.Fatal("the review folder should serve the snapshot")
	}

	// A new bare verdict that no agent has seen replaces the old one in place.
	postComment(t, h, map[string]any{"item": "s1", "verdict": "approve"}, 200)
	d, _ := s.Store.Load()
	var onS1 []Comment
	for _, c := range d.Comments {
		if c.Item == "s1" {
			onS1 = append(onS1, c)
		}
	}
	if len(onS1) != 1 || onS1[0].Verdict != "approve" || onS1[0].Status != "resolved" || onS1[0].ID != "c_01" {
		t.Fatalf("s1 comments after changing the verdict: %+v", onS1)
	}

	// One that was sent is resolved with a note, and the new one is a new comment.
	sent := postComment(t, h, map[string]any{"item": "s3", "verdict": "redo"}, 200)
	if w := do(t, h, "POST", "/api/send", map[string]any{}, nil); w.Code != 200 {
		t.Fatalf("send: %d %s", w.Code, w.Body)
	}
	now := postComment(t, h, map[string]any{"item": "s3", "verdict": "approve"}, 200)
	d, _ = s.Store.Load()
	old := Find(d, sent.ID)
	if old == nil || old.Status != "resolved" || old.ResolvedNote != "replaced by approve in "+now.ID {
		t.Fatalf("sent redo %+v, new %+v", old, now)
	}
	// Taking the new verdict back does not bring the replaced one back.
	if w := do(t, h, "DELETE", "/api/comments/"+now.ID, nil, nil); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if st := getState(t, h); st.item("s3")["verdict"] != nil || strings.Contains(strings.Join(st.Approval.Blockers, ";"), "s3") {
		t.Fatalf("s3 after taking the verdict back: %+v / %+v", st.item("s3"), st.Approval)
	}
	d, _ = s.Store.Load()
	// Notes with words are never replaced.
	if n := Find(d, note.ID); n == nil || n.Status != "open" {
		t.Fatalf("the note should stay open: %+v", n)
	}

	for name, body := range map[string]map[string]any{
		"keep is for a mood board": {"item": "s1", "verdict": "keep"},
		"no such item":             {"item": "s9", "verdict": "redo"},
		"nothing to say":           {"item": "s1"},
		"short sha":                {"item": "s1", "verdict": "redo", "itemSha": "abc"},
	} {
		if w := do(t, h, "POST", "/api/comments", body, nil); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
}

func TestStillsNoteOnAChangedItemIsKeptAndOutdated(t *testing.T) {
	_, h, _, dir := newStillsServer(t)
	st := getState(t, h)
	shown := st.shas()["s2"]
	writeFile(t, filepath.Join(dir, "board", "s2.png"), "a new frame")
	c := postComment(t, h, map[string]any{"item": "s2", "itemSha": shown, "verdict": "redo"}, 200)
	if c.ItemSHA != shown {
		t.Fatalf("the note should keep the hash the page showed: %+v", c)
	}
	st = getState(t, h)
	it := st.item("s2")
	if it["outdated"] != true || it["oldVerdict"] != "redo" || it["verdict"] != nil {
		t.Fatalf("s2 state %+v", it)
	}
	if !st.Approval.CanApprove {
		t.Fatalf("an outdated redo should not block: %+v", st.Approval)
	}
}

func TestApproveRules(t *testing.T) {
	s, h, _, dir := newStillsServer(t)
	st := getState(t, h)
	if !st.Approval.CanApprove {
		t.Fatalf("a fresh storyboard can be approved: %+v", st.Approval)
	}

	// A redo blocks, and the server refuses even when the page sends the right hashes.
	postComment(t, h, map[string]any{"item": "s1", "verdict": "redo"}, 200)
	st = getState(t, h)
	if st.Approval.CanApprove || len(st.Approval.Blockers) != 1 || st.Approval.Blockers[0] != "s1 is marked redo" {
		t.Fatalf("approval %+v", st.Approval)
	}
	w := do(t, h, "POST", "/api/approve", map[string]any{"items": st.shas()}, nil)
	var refused struct {
		Error    string
		Blockers []string
	}
	json.Unmarshal(w.Body.Bytes(), &refused)
	if w.Code != 409 || len(refused.Blockers) != 1 {
		t.Fatalf("approve with a redo: %d %s", w.Code, w.Body)
	}

	// An open note blocks too.
	postComment(t, h, map[string]any{"item": "s1", "verdict": "approve"}, 200)
	note := postComment(t, h, map[string]any{"item": "s2", "text": "check the kerning"}, 200)
	st = getState(t, h)
	if st.Approval.CanApprove || st.Approval.Blockers[0] != note.ID+" is open" {
		t.Fatalf("approval %+v", st.Approval)
	}
	approveStage(t, h, st, 409)
	do(t, h, "POST", "/api/comments/"+note.ID, map[string]any{"status": "resolved"}, nil)

	// A redo given to an earlier version of the frame no longer counts.
	old := st.shas()["s2"]
	postComment(t, h, map[string]any{"item": "s2", "itemSha": old, "verdict": "redo"}, 200)
	writeFile(t, filepath.Join(dir, "board", "s2.png"), "fixed frame")
	st = getState(t, h)
	if !st.Approval.CanApprove {
		t.Fatalf("an outdated redo should not block: %+v", st.Approval)
	}

	// The page approves what it showed: hashes that moved on are refused.
	stale := st.shas()
	stale["s2"] = old
	if w := do(t, h, "POST", "/api/approve", map[string]any{"items": stale}, nil); w.Code != 409 || !strings.Contains(w.Body.String(), "look again") {
		t.Fatalf("stale hashes: %d %s", w.Code, w.Body)
	}
	delete(stale, "s2")
	if w := do(t, h, "POST", "/api/approve", map[string]any{"items": stale}, nil); w.Code != 409 {
		t.Fatalf("a missing item: %d %s", w.Code, w.Body)
	}

	approveStage(t, h, st, 200)
	d, _ := s.Store.Load()
	p := Pending(d)
	if d.Approved == nil || d.Approved.By != "seb" || d.Approved.Items["s2"] != st.shas()["s2"] || p == nil || !p.Approved {
		t.Fatalf("approved %+v, pending %+v", d.Approved, p)
	}
	if st = getState(t, h); !st.Approval.Approved || st.ApprovalCleared != nil {
		t.Fatalf("state after approving: %+v", st)
	}

	// Taking the approval back does not make a send.
	n := len(d.Sends)
	if w := do(t, h, "POST", "/api/approve", map[string]any{"withdraw": true}, nil); w.Code != 200 {
		t.Fatalf("withdraw: %d %s", w.Code, w.Body)
	}
	d, _ = s.Store.Load()
	if d.Approved != nil || len(d.Sends) != n {
		t.Fatalf("after withdraw: %+v, %d sends", d.Approved, len(d.Sends))
	}

	// A review of video has no approve.
	_, vh := newTestServer(t)
	if w := do(t, vh, "POST", "/api/approve", map[string]any{"items": map[string]string{}}, nil); w.Code != 404 {
		t.Fatalf("video approve: %d", w.Code)
	}
}

func TestMoodboardVerdictsAndApprovalSend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "moodboard.json")
	writeFile(t, filepath.Join(dir, "a.jpg"), "a")
	writeFile(t, filepath.Join(dir, "b.jpg"), "b")
	writeFile(t, path, `{"schema":"cav-moodboard/1","items":[{"id":"a","file":"a.jpg"},{"id":"b","file":"b.jpg"}]}`)
	s := &Server{Store: Open(path), Stills: &Stills{Kind: KindMoodboard, Path: path}, Author: "seb"}
	h := s.Handler()

	keep := postComment(t, h, map[string]any{"item": "a", "verdict": "keep"}, 200)
	more := postComment(t, h, map[string]any{"item": "b", "verdict": "more"}, 200)
	if keep.Status != "resolved" || more.Status != "open" {
		t.Fatalf("keep %+v more %+v", keep, more)
	}
	postComment(t, h, map[string]any{"item": "a", "verdict": "approve"}, 400)

	// "More like this" is a request, not a block: the stage can be approved, and the send
	// carries the request to the agent.
	st := getState(t, h)
	if !st.Approval.CanApprove {
		t.Fatalf("more should not block: %+v", st.Approval)
	}
	approveStage(t, h, st, 200)
	d, _ := s.Store.Load()
	p := Pending(d)
	if p == nil || !p.Approved || len(p.Comments) != 1 || p.Comments[0] != more.ID {
		t.Fatalf("approval send %+v", p)
	}

	// Dropping a reference blocks, and clears the approval at once.
	postComment(t, h, map[string]any{"item": "b", "verdict": "drop"}, 200)
	st = getState(t, h)
	if st.Approval.Approved || st.Approval.CanApprove || st.ApprovalCleared == nil || st.ApprovalCleared.Reason != "b is marked drop" {
		t.Fatalf("after drop: %+v", st)
	}
}

func TestApprovalClearedWhenAnItemChanges(t *testing.T) {
	s, h, path, dir := newStillsServer(t)
	postComment(t, h, map[string]any{"item": "s2", "verdict": "approve"}, 200)
	approveStage(t, h, getState(t, h), 200)

	writeFile(t, filepath.Join(dir, "board", "s2.png"), "a different frame")
	st := getState(t, h)
	if st.Approval.Approved || st.ApprovalCleared == nil || !strings.Contains(st.ApprovalCleared.Reason, "s2") {
		t.Fatalf("a changed frame should remove the approval: %+v", st.ApprovalCleared)
	}
	if it := st.item("s2"); it["outdated"] != true || it["oldVerdict"] != "approve" {
		t.Fatalf("s2 %+v", it)
	}
	d, _ := s.Store.Load()
	if d.Approved != nil || d.ApprovalCleared == nil {
		t.Fatalf("the review file should say so: %+v", d)
	}

	// A shot added or removed does the same.
	approveStage(t, h, st, 200)
	sb, _ := board.Load(path)
	sb.Shots = append(sb.Shots, board.Shot{ID: "s4", Beats: [2]float64{12, 16}, What: "outro"})
	sb.Save(path)
	if st = getState(t, h); st.Approval.Approved || st.ApprovalCleared.Reason != "s4 added" {
		t.Fatalf("added shot: %+v %+v", st.Approval, st.ApprovalCleared)
	}
	approveStage(t, h, st, 200)
	sb.Shots = sb.Shots[:3]
	sb.Save(path)
	if st = getState(t, h); st.Approval.Approved || st.ApprovalCleared.Reason != "s4 removed" {
		t.Fatalf("removed shot: %+v %+v", st.Approval, st.ApprovalCleared)
	}
}

func TestApprovalClearedByANewRedoOrReopenedNote(t *testing.T) {
	_, h, _, _ := newStillsServer(t)
	approveStage(t, h, getState(t, h), 200)
	redo := postComment(t, h, map[string]any{"item": "s1", "verdict": "redo"}, 200)
	if st := getState(t, h); st.Approval.Approved || st.ApprovalCleared.Reason != "s1 is marked redo" {
		t.Fatalf("a new redo: %+v %+v", st.Approval, st.ApprovalCleared)
	}
	if w := do(t, h, "DELETE", "/api/comments/"+redo.ID, nil, nil); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}

	note := postComment(t, h, map[string]any{"item": "s2", "text": "kerning"}, 200)
	do(t, h, "POST", "/api/comments/"+note.ID, map[string]any{"status": "resolved"}, nil)
	approveStage(t, h, getState(t, h), 200)
	if w := do(t, h, "POST", "/api/comments/"+note.ID, map[string]any{"status": "open"}, nil); w.Code != 200 {
		t.Fatalf("reopen: %d", w.Code)
	}
	st := getState(t, h)
	if st.Approval.Approved || st.ApprovalCleared == nil || st.ApprovalCleared.Reason != note.ID+" is open" {
		t.Fatalf("a reopened note: %+v %+v", st.Approval, st.ApprovalCleared)
	}
}

// `cav review storyboard.json` is the usual way to start it: the target is a relative path,
// and the folder it sits in may itself be reached through a link (/tmp on macOS).
func TestStillsServesImagesForARelativeTarget(t *testing.T) {
	_, dir := writeBoard(t)
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	t.Chdir(link)
	s := &Server{Store: Open("storyboard.json"), Stills: &Stills{Kind: KindStoryboard, Path: "storyboard.json"}, Author: "seb"}
	h := s.Handler()
	if st := getState(t, h); st.item("s1")["hasImage"] != true {
		t.Fatalf("s1 should have an image: %+v", st.item("s1"))
	}
	if w := do(t, h, "GET", "/api/item/s1/image", nil, nil); w.Code != 200 || w.Body.String() != "frame s1" {
		t.Fatalf("image: %d %q", w.Code, w.Body)
	}
}

func TestStillsServesOnlyItemImages(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.png"), "secret")
	writeFile(t, filepath.Join(filepath.Dir(dir), "x.png"), "x")
	defer os.Remove(filepath.Join(filepath.Dir(dir), "x.png"))
	os.MkdirAll(filepath.Join(dir, "board"), 0o755)
	writeFile(t, filepath.Join(dir, "board", "good.png"), "good frame")
	writeFile(t, filepath.Join(dir, "board", "notes.txt"), "text")
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(dir, "board", "link.png")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "board", "dir")); err != nil {
		t.Fatal(err)
	}
	frames := map[string]string{"good": "board/good.png", "up": "../x.png", "abs": filepath.Join(outside, "secret.png"),
		"link": "board/link.png", "txt": "board/notes.txt", "viadir": "board/dir/secret.png"}
	sb := &board.Board{BPM: 120}
	i := 0
	for _, id := range []string{"good", "up", "abs", "link", "txt", "viadir"} {
		sb.Shots = append(sb.Shots, board.Shot{ID: id, Beats: [2]float64{float64(i * 4), float64(i*4 + 4)}, What: id, Frame: frames[id]})
		i++
	}
	path := filepath.Join(dir, "storyboard.json")
	sb.Save(path)
	s := &Server{Store: Open(path), Stills: &Stills{Kind: KindStoryboard, Path: path}, Author: "seb"}
	h := s.Handler()

	w := do(t, h, "GET", "/api/item/good/image", nil, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Body.String() != "good frame" {
		t.Fatalf("good frame: %d %s %q", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
	st := getState(t, h)
	for id := range frames {
		if id == "good" {
			continue
		}
		if w := do(t, h, "GET", "/api/item/"+id+"/image", nil, nil); w.Code != 404 {
			t.Errorf("%s: %d %s", id, w.Code, w.Body)
		}
		if st.item(id)["hasImage"] != false {
			t.Errorf("%s should not claim an image", id)
		}
	}
	if w := do(t, h, "GET", "/api/item/nope/image", nil, nil); w.Code != 404 {
		t.Errorf("unknown item: %d", w.Code)
	}

	// There is no video: the routes that would serve one answer 404, and never list a folder.
	for _, p := range []string{"/video", "/video?v=abc", "/video/" + strings.Repeat("a", 64), "/api/waveform/" + strings.Repeat("a", 64)} {
		if w := do(t, h, "GET", p, nil, nil); w.Code != 404 {
			t.Errorf("GET %s: %d %q", p, w.Code, w.Body)
		}
	}
}
