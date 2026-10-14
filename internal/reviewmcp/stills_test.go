package reviewmcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/review"
)

// pageNonce reads the nonce the way the host does: from the page resource it shows the person.
func pageNonce(t *testing.T, cs *mcp.ClientSession) string {
	t.Helper()
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: PageURI})
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`name="cav-nonce" content="([0-9a-f]{32,})"`).FindStringSubmatch(res.Contents[0].Text)
	if m == nil {
		t.Fatalf("the page resource should carry a nonce:\n%.300s", res.Contents[0].Text)
	}
	return m[1]
}

func TestAgentCannotApproveThroughReviewRequest(t *testing.T) {
	path := storyboardFile(t)
	cs, _ := connectTo(t, path)
	nonce := pageNonce(t, cs)
	var shown showOut
	shownRes := call(t, cs, "show_review", map[string]any{"video": path}, &shown)
	items := shas(t, cs, path)
	body := func(n string) string {
		b, _ := json.Marshal(map[string]any{"items": items, "nonce": n})
		return string(b)
	}
	for name, n := range map[string]string{"no nonce": "", "guessed nonce": "0123456789abcdef0123456789abcdef"} {
		if r := request(t, cs, path, "POST", "/api/approve", body(n)); r.Status != 403 {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if r := request(t, cs, path, "POST", "/api/approve", `{"withdraw":true}`); r.Status != 403 {
		t.Errorf("withdraw: %+v", r)
	}
	if d, _ := review.Open(path).Load(); d.Approved != nil || len(d.Sends) != 0 {
		t.Fatalf("the agent approved: %+v", d)
	}

	// The nonce is in the page and nowhere an agent reads: not in a tool result, not in the
	// state, not in the notes, not in the review file.
	var all strings.Builder
	for _, c := range shownRes.Content {
		all.WriteString(c.(*mcp.TextContent).Text)
	}
	b, _ := json.Marshal(shownRes.StructuredContent)
	all.Write(b)
	all.WriteString(request(t, cs, path, "GET", "/api/state", "").Body)
	notes := call(t, cs, "review_notes", map[string]any{"video": path}, nil)
	for _, c := range notes.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			all.WriteString(tc.Text)
		}
	}
	b, _ = json.Marshal(notes.StructuredContent)
	all.Write(b)
	doc, _ := os.ReadFile(review.Open(path).DocPath)
	all.Write(doc)
	if strings.Contains(all.String(), nonce) {
		t.Error("the nonce leaked into something an agent can read")
	}
	if r := request(t, cs, path, "POST", "/api/approve", body(nonce)); r.Status != 200 {
		t.Errorf("the page, which has the nonce, can approve: %+v", r)
	}
}

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
	body, _ := json.Marshal(map[string]any{"items": shas(t, cs, path), "nonce": pageNonce(t, cs)})
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
