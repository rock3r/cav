package reviewmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rock3r/cav/internal/review"
)

const page = `<!doctype html><html><head><title>cav review</title></head><body><video id="video" src="/video"></video><script>/* page */</script></body></html>`

// connect starts the server with a fake Prepare (no ffmpeg) and returns a client session
// and a counter of the prepares.
func connect(t *testing.T) (*mcp.ClientSession, string, *int) {
	t.Helper()
	dir := t.TempDir()
	video := filepath.Join(dir, "final.mp4")
	if err := os.WriteFile(video, []byte("render v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	cs, prepared := connectTo(t, video)
	return cs, video, prepared
}

// connectTo starts another server process, as a host does, for a video that already exists.
func connectTo(t *testing.T, video string) (*mcp.ClientSession, *int) {
	t.Helper()
	dir := filepath.Dir(video)
	prepared := 0
	srv := New(Options{
		Version: "test", Page: []byte(page), Shim: []byte("/* shim */"), Author: "ann", CacheDir: filepath.Join(dir, "cache"),
		Resolve: func(v string) (string, error) {
			if v == "" {
				return video, nil
			}
			return v, nil
		},
		Prepare: func(ctx context.Context, v, cacheDir string) (review.Source, string, error) {
			prepared++
			if b, _ := os.ReadFile(v); string(b) == "broken" {
				return review.Source{}, "", errors.New("no video stream")
			}
			sha, err := review.Digest(v)
			if err != nil {
				return review.Source{}, "", err
			}
			proxy := filepath.Join(cacheDir, sha+".mp4")
			os.MkdirAll(cacheDir, 0o755)
			if err := os.WriteFile(proxy, []byte("preview of "+v), 0o644); err != nil {
				return review.Source{}, "", err
			}
			return review.Source{Render: v, SHA256: sha, FPS: 25, Frames: 50, Width: 640, Height: 360}, proxy, nil
		},
	})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, &prepared
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil && !res.IsError {
		b, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
	return res
}

func TestToolsPointAtThePage(t *testing.T) {
	cs, _, _ := connect(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tl := range res.Tools {
		tools[tl.Name] = tl
	}
	ui := func(name string) map[string]any {
		tl := tools[name]
		if tl == nil {
			t.Fatalf("no %s tool", name)
		}
		m, _ := tl.Meta["ui"].(map[string]any)
		if m == nil || m["resourceUri"] != PageURI {
			t.Fatalf("%s: _meta.ui = %v, want resourceUri %s", name, tl.Meta["ui"], PageURI)
		}
		return m
	}
	if v := ui("show_review")["visibility"]; v != nil {
		t.Errorf("show_review must be visible to the model, got visibility %v", v)
	}
	if v, _ := ui("review_request")["visibility"].([]any); len(v) != 1 || v[0] != "app" {
		t.Errorf("review_request visibility = %v, want [app]", v)
	}
}

func TestPageResourceIsTheReviewPageWithTheShim(t *testing.T) {
	cs, _, _ := connect(t)
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: PageURI})
	if err != nil {
		t.Fatal(err)
	}
	c := res.Contents[0]
	if c.MIMEType != AppMIME {
		t.Errorf("mime type %q, want %q", c.MIMEType, AppMIME)
	}
	shim, head, pageScript := strings.Index(c.Text, "/* shim */"), strings.Index(c.Text, "</head>"), strings.Index(c.Text, "/* page */")
	if shim < 0 || pageScript < 0 || !(shim < head && head < pageScript) {
		t.Errorf("the shim must sit in <head>, before the page's script:\n%s", c.Text)
	}
}

func TestShowReviewOpensTheVideoAndListsNotes(t *testing.T) {
	cs, video, _ := connect(t)
	store := review.Open(video)
	store.Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments,
			review.Comment{ID: "c_01", Frame: 12, Status: "open", Text: "the logo lands late\nsecond line"},
			review.Comment{ID: "c_02", Frame: 30, Status: "resolved", Text: "done"})
		return nil
	})
	var out showOut
	res := call(t, cs, "show_review", map[string]any{}, &out)
	if res.IsError {
		t.Fatalf("show_review failed: %v", res.Content)
	}
	if out.Video != video || out.Frames != 50 || out.Open != 1 || out.Resolved != 1 {
		t.Errorf("structured result %+v", out)
	}
	// Some hosts pass the model only the structured result, so it carries the notes too.
	if len(out.OpenNotes) != 1 || out.OpenNotes[0].ID != "c_01" || out.OpenNotes[0].Frame != 12 || out.OpenNotes[0].Text != "the logo lands late" {
		t.Errorf("openNotes = %+v", out.OpenNotes)
	}
	if !strings.Contains(out.Next, "cav review wait") || strings.Contains(out.Next, video) {
		t.Errorf("next must name the command without pasting the path into it: %q", out.Next)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"c_01 frame 12: the logo lands late", "1 open, 1 resolved", "Video path: " + video + "\n", "`cav review wait`"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "second line") {
		t.Errorf("the summary should keep only the first line of a note:\n%s", text)
	}
	d, _ := store.Load()
	if d.Source.Frames != 50 || len(d.Versions) != 1 {
		t.Errorf("the review file should record the render: %+v", d.Source)
	}
}

func TestReviewRequestRunsThePageAPI(t *testing.T) {
	cs, video, _ := connect(t)
	call(t, cs, "show_review", map[string]any{"video": video}, nil)

	var state requestOut
	call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, &state)
	if state.Status != 200 || !strings.Contains(state.Body, `"author":"ann"`) {
		t.Fatalf("GET /api/state = %+v", state)
	}

	var added requestOut
	call(t, cs, "review_request", map[string]any{"video": video, "method": "POST", "path": "/api/comments",
		"body": `{"frame":7,"text":"too fast"}`}, &added)
	if added.Status != 200 {
		t.Fatalf("POST /api/comments = %+v", added)
	}
	d, _ := review.Open(video).Load()
	if len(d.Comments) != 1 || d.Comments[0].Text != "too fast" || d.Comments[0].Author != "ann" {
		t.Errorf("the note should be in the review file: %+v", d.Comments)
	}

	var shown struct{ Version string }
	json.Unmarshal([]byte(state.Body), &shown)
	var vid requestOut
	call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/video?v=" + shown.Version}, &vid)
	if vid.Status != 200 || !strings.HasPrefix(vid.DataURL, "data:video/mp4;base64,") || vid.Body != "" {
		t.Errorf("GET /video should come back as a data URL, got status %d, %.60q", vid.Status, vid.DataURL)
	}
}

func TestReviewRequestOnlyReachesOpenedVideos(t *testing.T) {
	cs, video, _ := connect(t)
	other := filepath.Join(filepath.Dir(video), "other.mp4")
	os.WriteFile(other, []byte("x"), 0o644)
	res := call(t, cs, "review_request", map[string]any{"video": other, "method": "GET", "path": "/api/state"}, nil)
	if !res.IsError {
		t.Fatalf("a video not opened with show_review must be refused: %+v", res.StructuredContent)
	}
	call(t, cs, "show_review", map[string]any{"video": video}, nil)
	res = call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "api/state"}, nil)
	if !res.IsError {
		t.Error("a path without a leading / must be refused")
	}
}

// Claude Desktop runs show_review in one server process and sends the page's requests to
// another: the second one must serve a video that the first opened.
func TestPageRequestsReachAnotherServerProcess(t *testing.T) {
	model, video, _ := connect(t)
	call(t, model, "show_review", map[string]any{"video": video}, nil)
	page, _ := connectTo(t, video)
	var state requestOut
	res := call(t, page, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, &state)
	if res.IsError || state.Status != 200 || !strings.Contains(state.Body, `"frames":50`) {
		t.Fatalf("the second process should serve the opened video: %v %+v", res.Content, state)
	}
	var vid requestOut
	call(t, page, "review_request", map[string]any{"video": video, "method": "GET", "path": "/video"}, &vid)
	if vid.Status != 200 || !strings.HasPrefix(vid.DataURL, "data:video/mp4;base64,") {
		t.Errorf("GET /video from the second process: status %d", vid.Status)
	}
	// A video with no review file is still out of reach for it.
	other := filepath.Join(filepath.Dir(video), "other.mp4")
	os.WriteFile(other, []byte("x"), 0o644)
	if res := call(t, page, "review_request", map[string]any{"video": other, "method": "GET", "path": "/api/state"}, nil); !res.IsError {
		t.Error("a video without a review file must be refused")
	}
}

func TestReRenderPreparesAgain(t *testing.T) {
	cs, video, prepared := connect(t)
	call(t, cs, "show_review", map[string]any{"video": video}, nil)
	call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, nil)
	if *prepared != 1 {
		t.Fatalf("prepared %d times, want 1 while the file is unchanged", *prepared)
	}
	later := time.Now().Add(time.Minute)
	os.WriteFile(video, []byte("render v2, longer"), 0o644)
	os.Chtimes(video, later, later)
	state := func() {
		call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, nil)
	}
	state()
	if *prepared != 1 {
		t.Fatalf("prepared %d times, want 1 until the new file is seen unchanged twice", *prepared)
	}
	// Still being written in place: it grows between two requests.
	os.WriteFile(video, []byte("render v2, longer, still growing"), 0o644)
	os.Chtimes(video, later, later)
	state()
	if *prepared != 1 {
		t.Fatalf("prepared %d times, want 1 while the file is still growing", *prepared)
	}
	state()
	if *prepared != 2 {
		t.Errorf("prepared %d times, want 2 once the re-render stopped changing", *prepared)
	}
	d, _ := review.Open(video).Load()
	if len(d.Versions) != 2 {
		t.Errorf("both renders should be recorded, got %d", len(d.Versions))
	}
}

// A note posted while a re-render waits to go live stays on the render the page shows: only
// GET /api/state, whose answer tells the page, makes the new render current (issue #26).
func TestNoteDuringReRenderStaysOnTheShownRender(t *testing.T) {
	cs, video, prepared := connect(t)
	call(t, cs, "show_review", map[string]any{"video": video}, nil)
	var state requestOut
	call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, &state)
	var shown struct{ Version string }
	json.Unmarshal([]byte(state.Body), &shown)

	later := time.Now().Add(time.Minute)
	os.WriteFile(video, []byte("render v2, longer"), 0o644)
	os.Chtimes(video, later, later)
	call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, nil) // sees the change

	var added requestOut
	call(t, cs, "review_request", map[string]any{"video": video, "method": "POST", "path": "/api/comments",
		"body": `{"frame":3,"text":"on what I see"}`}, &added)
	if added.Status != 200 || *prepared != 1 {
		t.Fatalf("a POST must not make the re-render current: status %d, prepared %d", added.Status, *prepared)
	}
	var c review.Comment
	json.Unmarshal([]byte(added.Body), &c)
	if c.Version != shown.Version {
		t.Errorf("note on version %s, want the shown %s", c.Version, shown.Version)
	}

	call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, &state)
	var now struct{ Version string }
	json.Unmarshal([]byte(state.Body), &now)
	if *prepared != 2 || now.Version == shown.Version {
		t.Fatalf("GET /api/state should make the re-render current: prepared %d, version %s", *prepared, now.Version)
	}

	// A page whose state is still cached posts against the earlier render, by name.
	call(t, cs, "review_request", map[string]any{"video": video, "method": "POST", "path": "/api/comments",
		"body": `{"frame":4,"text":"still the old one","version":"` + shown.Version + `"}`}, &added)
	json.Unmarshal([]byte(added.Body), &c)
	if added.Status != 200 || c.Version != shown.Version {
		t.Errorf("a note naming the shown render should keep it: %d %+v", added.Status, c)
	}
}

func TestBrokenReRenderKeepsTheEarlierOne(t *testing.T) {
	cs, video, prepared := connect(t)
	call(t, cs, "show_review", map[string]any{"video": video}, nil)
	later := time.Now().Add(time.Minute)
	os.WriteFile(video, []byte("broken"), 0o644)
	os.Chtimes(video, later, later)
	state := func() requestOut {
		var out requestOut
		res := call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, &out)
		if res.IsError {
			t.Fatalf("review_request failed: %v", res.Content)
		}
		return out
	}
	state()        // sees the change
	out := state() // prepares it, which fails
	if *prepared != 2 || out.Status != 200 || !strings.Contains(out.Body, `"renderError":"no video stream"`) {
		t.Fatalf("prepared %d, want 2 with the earlier render still served and the error shown: %+v", *prepared, out)
	}
	state()
	if *prepared != 2 {
		t.Errorf("prepared %d times: an unchanged broken file must not be prepared again", *prepared)
	}
	os.WriteFile(video, []byte("fixed render"), 0o644)
	os.Chtimes(video, later.Add(time.Minute), later.Add(time.Minute))
	state()
	if out := state(); *prepared != 3 || !strings.Contains(out.Body, `"renderError":""`) {
		t.Errorf("a fixed render should be prepared and clear the error: prepared %d, %s", *prepared, out.Body)
	}
}

// A server process that never opened the video, and finds it broken, serves the last render
// in the review file with the error shown, as a process that had the session does (issue #33).
func TestNewProcessKeepsTheEarlierRenderOfABrokenFile(t *testing.T) {
	model, video, _ := connect(t)
	call(t, model, "show_review", map[string]any{"video": video}, nil)
	later := time.Now().Add(time.Minute)
	os.WriteFile(video, []byte("broken"), 0o644)
	os.Chtimes(video, later, later)

	page, prepared := connectTo(t, video)
	state := func() requestOut {
		var out requestOut
		res := call(t, page, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, &out)
		if res.IsError {
			t.Fatalf("review_request failed: %v", res.Content)
		}
		return out
	}
	out := state()
	if out.Status != 200 || !strings.Contains(out.Body, `"frames":50`) || !strings.Contains(out.Body, `"renderError":"no video stream"`) {
		t.Fatalf("the earlier render should be served with the error shown: %+v", out)
	}
	var vid requestOut
	call(t, page, "review_request", map[string]any{"video": video, "method": "GET", "path": "/video"}, &vid)
	if vid.Status != 200 || !strings.HasPrefix(vid.DataURL, "data:video/mp4;base64,") {
		t.Errorf("GET /video should play the earlier preview: status %d", vid.Status)
	}
	state()
	if *prepared != 1 {
		t.Errorf("prepared %d times: an unchanged broken file must not be prepared again", *prepared)
	}
	os.WriteFile(video, []byte("fixed render"), 0o644)
	os.Chtimes(video, later.Add(time.Minute), later.Add(time.Minute))
	state()
	if out := state(); *prepared != 2 || !strings.Contains(out.Body, `"renderError":""`) {
		t.Errorf("a fixed render should be prepared and clear the error: prepared %d, %s", *prepared, out.Body)
	}

	// Without the earlier preview there is nothing to show, so the request still fails.
	os.WriteFile(video, []byte("broken"), 0o644)
	os.Chtimes(video, later.Add(2*time.Minute), later.Add(2*time.Minute))
	cache := filepath.Join(filepath.Dir(video), "cache")
	os.RemoveAll(cache)
	fresh, _ := connectTo(t, video)
	if res := call(t, fresh, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, nil); !res.IsError {
		t.Error("a broken file with no earlier preview must fail the request")
	}

	// A digest in the review file that is not one must not name a file outside the cache.
	os.MkdirAll(cache, 0o755)
	os.WriteFile(filepath.Join(filepath.Dir(video), "secret.mp4"), []byte("not a preview"), 0o644)
	review.Open(video).Update(func(d *review.Doc) error { d.Source.SHA256 = "../secret"; return nil })
	fresh, _ = connectTo(t, video)
	if res := call(t, fresh, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, nil); !res.IsError {
		t.Error("a review file whose digest leaves the cache must not be served")
	}

	// A review file that cannot be read fails the request too, without stopping the server.
	doc, _ := review.Paths(video)
	os.WriteFile(doc, []byte("{not json"), 0o644)
	fresh, _ = connectTo(t, video)
	if res := call(t, fresh, "review_request", map[string]any{"video": video, "method": "GET", "path": "/api/state"}, nil); !res.IsError {
		t.Error("an unreadable review file must fail the request")
	}
}

func TestDoRefusesFilesTooLargeForTheChat(t *testing.T) {
	// Served in chunks, as http.ServeFile does: the writer must stop the copy at the limit
	// instead of buffering the whole file.
	written := 0
	big := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		chunk := make([]byte, 1<<20)
		for i := 0; i < 4*(MaxInline>>20); i++ {
			n, err := w.Write(chunk)
			written += n
			if err != nil {
				return
			}
		}
	})
	out := Do(big, "GET", "/video", "")
	if out.Status != http.StatusRequestEntityTooLarge || out.DataURL != "" || !strings.Contains(out.Body, "cav review") {
		t.Errorf("got status %d, body %q", out.Status, out.Body)
	}
	if written > MaxInline {
		t.Errorf("the writer took %d bytes, more than the %d limit", written, MaxInline)
	}
}

func TestDoServesAFileUpToTheLimit(t *testing.T) {
	f := filepath.Join(t.TempDir(), "p.mp4")
	os.WriteFile(f, []byte("tiny video"), 0o644)
	out := Do(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, f) }), "GET", "/video", "")
	if out.Status != 200 || out.DataURL != "data:video/mp4;base64,dGlueSB2aWRlbw==" {
		t.Errorf("got %+v", out)
	}
}

func TestAppPageWithoutHead(t *testing.T) {
	got := string(AppPage([]byte("<p>hi</p>"), []byte("/* shim */")))
	if !strings.HasPrefix(got, `<meta name="color-scheme"`) || !strings.HasSuffix(got, "<p>hi</p>") || !strings.Contains(got, "/* shim */") {
		t.Errorf("got %q", got)
	}
}

func TestReviewNotesReceivesTheSendWithSnapshots(t *testing.T) {
	cs, video, _ := connect(t)
	if res := call(t, cs, "review_notes", map[string]any{"video": video}, nil); !res.IsError {
		t.Fatal("a video without a review must be refused")
	}
	call(t, cs, "show_review", map[string]any{"video": video}, nil)

	// One note with a snapshot in the review's own folder, one whose recorded snapshot path
	// points outside it, and a send of both.
	store := review.Open(video)
	png := []byte("\x89PNG\r\n\x1a\nfake")
	os.MkdirAll(store.SnapDir, 0o755)
	os.WriteFile(filepath.Join(store.SnapDir, "c_01_f12.png"), png, 0o644)
	outside := filepath.Join(t.TempDir(), "c_02_f30.png")
	os.WriteFile(outside, png, 0o644)
	// A symlink with the right name in the right folder, pointing at a file elsewhere.
	secret := filepath.Join(t.TempDir(), "secret.png")
	os.WriteFile(secret, []byte("\x89PNGsecret"), 0o644)
	if err := os.Symlink(secret, filepath.Join(store.SnapDir, "c_02_f30.png")); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
	store.Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments,
			review.Comment{ID: "c_01", Frame: 12, Timecode: "00:00:00:12", Status: "open", Text: "logo lands late",
				Shapes: []review.Shape{{Type: "arrow"}}, Snapshot: "elsewhere/c_01_f12.png", Version: d.Source.SHA256[:12]},
			review.Comment{ID: "c_02", Frame: 30, Status: "open", Text: "too fast", Snapshot: outside},
			review.Comment{ID: "c_03", Frame: 40, Status: "resolved", Text: "done"})
		d.Sends = append(d.Sends, review.Send{N: 1, At: time.Now(), Comments: []string{"c_01", "c_02"}})
		return nil
	})

	var out notesOut
	res := call(t, cs, "review_notes", map[string]any{"video": video}, &out)
	if res.IsError || out.Send != 1 || len(out.Notes) != 2 {
		t.Fatalf("got %v %+v", res.Content, out)
	}
	snapDir, _ := filepath.EvalSymlinks(store.SnapDir)
	if out.Notes[0].Snapshot != filepath.Join(snapDir, "c_01_f12.png") || out.Notes[0].Shapes[0] != "arrow" || out.Notes[0].OnOlderRender {
		t.Errorf("note c_01: %+v", out.Notes[0])
	}
	// c_02's recorded file sits outside the review's folder, and the file with its name in
	// the folder is a symlink to elsewhere: neither is read.
	if out.Notes[1].Snapshot != "" {
		t.Errorf("a snapshot outside the review's folder, or a symlink, must not be read: %q", out.Notes[1].Snapshot)
	}
	images := 0
	for _, c := range res.Content {
		if img, ok := c.(*mcp.ImageContent); ok && img.MIMEType == "image/png" && string(img.Data) == string(png) {
			images++
		}
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if images != 1 || !strings.Contains(text, "Send #1") || !strings.Contains(text, "logo lands late") {
		t.Errorf("want the text first and c_01's snapshot as the one image, got %d image(s):\n%s", images, text)
	}
	d, _ := store.Load()
	if d.Sends[0].DeliveredAt == nil {
		t.Error("the send must be marked received, as cav review wait does")
	}

	// Nothing pending now: the open notes come back, and nothing is marked.
	out = notesOut{}
	call(t, cs, "review_notes", map[string]any{"video": video}, &out)
	if out.Send != 0 || len(out.Notes) != 2 || out.Notes[0].ID != "c_01" {
		t.Errorf("without a send it should list the open notes: %+v", out)
	}
}

func TestReviewNotesRefusesASymlinkedSnapshotFolderAndCapsText(t *testing.T) {
	cs, video, _ := connect(t)
	call(t, cs, "show_review", map[string]any{"video": video}, nil)
	store := review.Open(video)
	// review/final is a symlink to a folder outside the project holding a matching file.
	elsewhere := t.TempDir()
	os.WriteFile(filepath.Join(elsewhere, "c_01_f0.png"), []byte("\x89PNGsecret"), 0o644)
	os.MkdirAll(filepath.Dir(store.SnapDir), 0o755)
	if err := os.Symlink(elsewhere, store.SnapDir); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
	store.Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments, review.Comment{ID: "c_01", Status: "open", Text: strings.Repeat("é", maxNoteText),
			Snapshot: "c_01_f0.png", Replies: make([]review.Reply, maxReplies+5)})
		return nil
	})
	var out notesOut
	res := call(t, cs, "review_notes", map[string]any{"video": video}, &out)
	if res.IsError || len(out.Notes) != 1 {
		t.Fatalf("got %v %+v", res.Content, out)
	}
	n := out.Notes[0]
	if n.Snapshot != "" {
		t.Errorf("a snapshot through a symlinked folder must not be read: %q", n.Snapshot)
	}
	if len(n.Text) > maxNoteText+len(" […]") || !strings.HasSuffix(n.Text, "[…]") || !utf8.ValidString(n.Text) {
		t.Errorf("note text not capped on a rune boundary: %d bytes", len(n.Text))
	}
	if len(n.Replies) != maxReplies {
		t.Errorf("replies not capped: %d", len(n.Replies))
	}
}

func TestReviewNotesLeavesALongSendPendingAndCapsShapes(t *testing.T) {
	cs, video, _ := connect(t)
	call(t, cs, "show_review", map[string]any{"video": video}, nil)
	store := review.Open(video)
	var ids []string
	store.Update(func(d *review.Doc) error {
		for i := 0; i < maxNotes+3; i++ {
			id := fmt.Sprintf("c_%03d", i+1)
			ids = append(ids, id)
			d.Comments = append(d.Comments, review.Comment{ID: id, Frame: i, Status: "open", Text: "n",
				Shapes: make([]review.Shape, 500)})
		}
		d.Sends = append(d.Sends, review.Send{N: 1, At: time.Now(), Comments: ids})
		return nil
	})
	var out notesOut
	res := call(t, cs, "review_notes", map[string]any{"video": video}, &out)
	if res.IsError || out.Send != 1 || len(out.Notes) != maxNotes {
		t.Fatalf("got %d notes, send %d: %v", len(out.Notes), out.Send, res.IsError)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "stays pending") {
		t.Errorf("a send cut short must say it stays pending:\n%.300s", text)
	}
	if d, _ := store.Load(); d.Sends[0].DeliveredAt != nil {
		t.Error("a send cut short must stay pending")
	}
	if len(out.Notes[0].Shapes) != 1 {
		t.Errorf("500 shapes of one kind should list that kind once, got %d", len(out.Notes[0].Shapes))
	}
}

func TestReviewNotesCapsEveryFieldOfAHandEditedFile(t *testing.T) {
	cs, video, _ := connect(t)
	call(t, cs, "show_review", map[string]any{"video": video}, nil)
	store := review.Open(video)
	huge := strings.Repeat("x", 1<<20)
	id := "c_" + huge
	store.Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments, review.Comment{ID: id, Status: "open" + huge, Timecode: huge, Text: "n"})
		d.Sends = append(d.Sends, review.Send{N: 1, At: time.Now(), Comments: []string{id}})
		return nil
	})
	var out notesOut
	res := call(t, cs, "review_notes", map[string]any{"video": video}, &out)
	if res.IsError || out.Send != 1 || len(out.Notes) != 1 {
		t.Fatalf("got %v %+v", res.IsError, out.Send)
	}
	if n := out.Notes[0]; len(n.ID) > 100 || len(n.Status) > 100 || len(n.Timecode) > 100 {
		t.Errorf("fields not capped: id %d, status %d, timecode %d bytes", len(n.ID), len(n.Status), len(n.Timecode))
	}
	if text := res.Content[0].(*mcp.TextContent).Text; len(text) > 10000 {
		t.Errorf("the text answer holds %d bytes", len(text))
	}
}

func TestReviewNotesHandsASendOutOnce(t *testing.T) {
	// No show_review first: a fresh server process reading an existing review file has no
	// session, and its calls must still share one store.
	cs, video, _ := connect(t)
	review.Open(video).Update(func(d *review.Doc) error {
		d.Comments = append(d.Comments, review.Comment{ID: "c_01", Status: "open", Text: "n"})
		d.Sends = append(d.Sends, review.Send{N: 1, At: time.Now(), Comments: []string{"c_01"}})
		return nil
	})
	const calls = 8
	sends := make(chan int, calls)
	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Not call: t.Fatal must not run outside the test's goroutine.
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "review_notes", Arguments: map[string]any{"video": video}})
			var out notesOut
			if err == nil && !res.IsError {
				b, _ := json.Marshal(res.StructuredContent)
				json.Unmarshal(b, &out)
			}
			sends <- out.Send
		}()
	}
	wg.Wait()
	close(sends)
	got := 0
	for s := range sends {
		got += s
	}
	if got != 1 {
		t.Errorf("send #1 handed out %d times", got)
	}
}

func TestSendStateSeesEveryChangeToTheSend(t *testing.T) {
	doc := func() *review.Doc {
		return &review.Doc{Comments: []review.Comment{{ID: "c_01", Status: "open", Text: "a"}, {ID: "c_02", Status: "open", Text: "b"}},
			Sends: []review.Send{{N: 1, Comments: []string{"c_01"}}}}
	}
	read := sendState(doc(), 1)
	same := doc()
	same.Comments[1].Text = "edited" // not in the send
	if sendState(same, 1) != read {
		t.Error("an edit to a note outside the send must not count")
	}
	for name, edit := range map[string]func(d *review.Doc){
		"edit":    func(d *review.Doc) { d.Comments[0].Text = "edited" },
		"reply":   func(d *review.Doc) { d.Comments[0].Replies = []review.Reply{{Text: "r"}} },
		"resolve": func(d *review.Doc) { d.Comments[0].Status = "resolved" },
		"delete":  func(d *review.Doc) { d.Comments = d.Comments[1:] },
		"taken":   func(d *review.Doc) { now := time.Now(); d.Sends[0].DeliveredAt = &now },
		"render":  func(d *review.Doc) { d.Source.SHA256 = "new" },
	} {
		d := doc()
		edit(d)
		if sendState(d, 1) == read {
			t.Errorf("%s: the change was not seen", name)
		}
	}
}
