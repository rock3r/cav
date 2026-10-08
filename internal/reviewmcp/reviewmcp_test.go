package reviewmcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	return cs, video, &prepared
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
	text := res.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"c_01 frame 12: the logo lands late", "1 open, 1 resolved", "cav review wait " + video} {
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

	var vid requestOut
	call(t, cs, "review_request", map[string]any{"video": video, "method": "GET", "path": "/video?v=abc"}, &vid)
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
	if !strings.HasPrefix(got, "<style>") || !strings.HasSuffix(got, "<p>hi</p>") || !strings.Contains(got, "/* shim */") {
		t.Errorf("got %q", got)
	}
}
