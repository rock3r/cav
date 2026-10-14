package reviewmcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/review"
)

// storyboardFile writes a storyboard of two shots with a frame each.
func storyboardFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "board"), 0o755)
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

// request runs one of the page's requests through review_request.
func request(t *testing.T, cs *mcp.ClientSession, path, method, api, body string) requestOut {
	t.Helper()
	var out requestOut
	call(t, cs, "review_request", map[string]any{"video": path, "method": method, "path": api, "body": body}, &out)
	return out
}

// shas reads the hash of every item from the page's state.
func shas(t *testing.T, cs *mcp.ClientSession, path string) map[string]string {
	t.Helper()
	st := request(t, cs, path, "GET", "/api/state", "")
	var s struct{ Items []struct{ ID, SHA string } }
	if err := json.Unmarshal([]byte(st.Body), &s); err != nil {
		t.Fatal(err, st.Body)
	}
	m := map[string]string{}
	for _, it := range s.Items {
		m[it.ID] = it.SHA
	}
	return m
}

func TestShowReviewOpensAStoryboard(t *testing.T) {
	path := storyboardFile(t)
	cs, prepared := connectTo(t, path)
	review.Open(path).Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments, review.Comment{ID: "c_01", Status: "open", Item: "s2", Verdict: "redo", Text: "bigger tagline"})
		return nil
	})
	var out showOut
	res := call(t, cs, "show_review", map[string]any{"video": path}, &out)
	if res.IsError {
		t.Fatalf("show_review: %v", res.Content)
	}
	if *prepared != 0 {
		t.Errorf("a storyboard has no video to prepare, Prepare ran %d times", *prepared)
	}
	if out.Kind != "storyboard" || out.Items != 2 || out.Approved || out.Open != 1 || out.Video != path || len(out.OpenNotes) != 1 || out.OpenNotes[0].Item != "s2" {
		t.Errorf("result %+v", out)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"Showing the review page of storyboard.json (2 shots)", "c_01 s2 (redo): bigger tagline", "Approved: no."} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	d, _ := review.Open(path).Load()
	if d.Kind != "storyboard" {
		t.Errorf("the review file should say what it is about: %+v", d)
	}
	// The page can reach it, and a stills file that was never shown is out of reach.
	if st := request(t, cs, path, "GET", "/api/state", ""); st.Status != 200 || !strings.Contains(st.Body, `"kind":"storyboard"`) {
		t.Errorf("state %+v", st)
	}
	other := storyboardFile(t)
	res = call(t, cs, "review_request", map[string]any{"video": other, "method": "GET", "path": "/api/state"}, nil)
	if !res.IsError {
		t.Error("review_request must not open a storyboard that show_review has not")
	}
	if *prepared != 0 {
		t.Errorf("Prepare ran %d times", *prepared)
	}
}

func TestReviewRequestServesItemImageAsDataURL(t *testing.T) {
	path := storyboardFile(t)
	cs, _ := connectTo(t, path)
	call(t, cs, "show_review", map[string]any{"video": path}, nil)
	img := request(t, cs, path, "GET", "/api/item/s1/image", "")
	if img.Status != 200 || img.ContentType != "image/png" || !strings.HasPrefix(img.DataURL, "data:image/png;base64,") || img.Body != "" {
		t.Errorf("image %d %s %.40q", img.Status, img.ContentType, img.DataURL)
	}
	if v := request(t, cs, path, "GET", "/video", ""); v.Status != 404 {
		t.Errorf("a storyboard has no video: %d", v.Status)
	}
}

func TestReviewNotesReportsItemsAndApproval(t *testing.T) {
	path := storyboardFile(t)
	cs, _ := connectTo(t, path)
	call(t, cs, "show_review", map[string]any{"video": path}, nil)

	if r := request(t, cs, path, "POST", "/api/comments", `{"item":"s1","verdict":"redo","text":"the logo is off centre"}`); r.Status != 200 {
		t.Fatalf("note: %+v", r)
	}
	if r := request(t, cs, path, "POST", "/api/send", `{}`); r.Status != 200 {
		t.Fatalf("send: %+v", r)
	}
	// The frame changes after the note was made.
	os.WriteFile(filepath.Join(filepath.Dir(path), "board", "s1.png"), []byte("new frame"), 0o644)

	var notes notesOut
	res := call(t, cs, "review_notes", map[string]any{"video": path}, &notes)
	text := res.Content[0].(*mcp.TextContent).Text
	if notes.Kind != "storyboard" || notes.Send != 1 || notes.Approved || len(notes.Notes) != 1 {
		t.Fatalf("notes %+v", notes)
	}
	n := notes.Notes[0]
	if n.Item != "s1" || n.Verdict != "redo" || !n.ItemChanged || n.OnOlderRender || n.Text != "the logo is off centre" {
		t.Errorf("note %+v", n)
	}
	for _, want := range []string{"c_01 open, s1, redo [item changed since]", "the logo is off centre", "changing the item in its file"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}

	// The person resolves the note, approves, and the approval is one send of its own.
	request(t, cs, path, "POST", "/api/comments/c_01", `{"status":"resolved"}`)
	request(t, cs, path, "POST", "/api/comments", `{"item":"s1","verdict":"approve"}`)
	body, _ := json.Marshal(map[string]any{"items": shas(t, cs, path)})
	if r := request(t, cs, path, "POST", "/api/approve", string(body)); r.Status != 200 {
		t.Fatalf("approve: %+v", r)
	}
	notes = notesOut{}
	res = call(t, cs, "review_notes", map[string]any{"video": path}, &notes)
	text = res.Content[0].(*mcp.TextContent).Text
	if notes.Send != 2 || !notes.Approved || len(notes.Notes) != 0 || !strings.Contains(text, "the reviewer approved it, by ann") || !strings.Contains(notes.Next, "next stage") {
		t.Fatalf("approval: %+v\n%s", notes, text)
	}
	// It was received: asking again finds nothing sent.
	notes = notesOut{}
	call(t, cs, "review_notes", map[string]any{"video": path}, &notes)
	if notes.Send != 0 || notes.Approved {
		t.Errorf("the approval should be received once: %+v", notes)
	}
}

func TestSendStateSeesApproval(t *testing.T) {
	approved := func() *review.Doc {
		return &review.Doc{Approved: &review.Approval{By: "ann", Items: map[string]string{"s1": "a"}},
			Sends: []review.Send{{N: 1, Comments: []string{}, Approved: true}}}
	}
	read := sendState(approved(), 1)
	if read == "" {
		t.Fatal("the pending send should have a state")
	}
	removed := approved()
	removed.Approved = nil
	if sendState(removed, 1) == read {
		t.Error("an approval taken away must change the state, or review_notes would claim a stale answer")
	}
	changed := approved()
	changed.Approved.Items["s1"] = "b"
	if sendState(changed, 1) == read {
		t.Error("an approval for other hashes must change the state")
	}
}
