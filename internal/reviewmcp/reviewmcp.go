// Package reviewmcp is a small MCP server that shows the `cav review` page inside a chat, as
// an MCP App (https://modelcontextprotocol.io/extensions/apps). The host draws the page in a
// sandboxed frame without network access, so the page reaches the review through the host:
// every request it makes becomes a call to the app-only review_request tool, which runs it
// through the same handler as `cav review` and returns the answer inline. The video arrives
// as a data: URL of a small preview copy.
//
// It is meant for a quick look and a few notes. Frame-by-frame review still belongs in the
// browser, with `cav review`.
package reviewmcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rock3r/cav/internal/review"
)

const (
	// PageURI is the UI resource the show_review tool points at.
	PageURI = "ui://cav/review.html"
	// AppMIME is the MCP Apps profile of text/html.
	AppMIME = "text/html;profile=mcp-app"
	// MaxInline is the largest file that review_request sends inline. Hosts pass every
	// answer through their own message channel, so a larger preview would stall the view.
	MaxInline = 24 << 20
)

// Options configure the server.
type Options struct {
	Version string
	// Page is the review page (assets.ReviewPage) and Shim the script that makes it an MCP
	// App (assets.ReviewAppShim).
	Page, Shim []byte
	Author     string
	// CacheDir holds the preview proxies, named <sha256>.mp4.
	CacheDir string
	// Resolve turns the tool's video argument (maybe empty) into a video path.
	Resolve func(video string) (string, error)
	// Prepare probes a video and makes its preview proxy. Tests replace it; nil uses
	// ffprobe and ffmpeg.
	Prepare func(ctx context.Context, video, cacheDir string) (review.Source, string, error)
}

// New builds the MCP server: the page resource, show_review for the model and
// review_request for the page.
func New(o Options) *mcp.Server {
	if o.Prepare == nil {
		o.Prepare = Prepare
	}
	h := &handler{o: o, open: map[string]*session{}}
	s := mcp.NewServer(&mcp.Implementation{Name: "cav", Title: "cav review", Version: o.Version}, &mcp.ServerOptions{
		Instructions: "show_review shows a render's cav review page in the chat, where the person can play it, draw and leave notes. " +
			"It is for a quick look: for frame-by-frame review run `cav review` in a shell. " +
			"Read the notes with review_notes, or with `cav review wait` or `cav review export` when you have a shell.",
	})
	pageMeta := mcp.Meta{"ui": map[string]any{"prefersBorder": true}}
	s.AddResource(&mcp.Resource{URI: PageURI, Name: "cav review", MIMEType: AppMIME, Meta: pageMeta,
		Description: "The cav review page: play a render, step frames, draw and leave notes for the agent."},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
				URI: PageURI, MIMEType: AppMIME, Text: string(AppPage(o.Page, o.Shim)), Meta: pageMeta,
			}}}, nil
		})
	mcp.AddTool(s, &mcp.Tool{
		Name:  "show_review",
		Title: "Show a render's review page",
		Description: "Show the cav review page for a rendered video in the chat, so the person can play it, draw on frames and leave notes. " +
			"Without a video it shows the newest review or MP4 in the renders folder. The result lists the notes so far. " +
			"After the person presses Send to agent, read the notes with review_notes.",
		Meta: mcp.Meta{"ui": map[string]any{"resourceUri": PageURI}, "ui/resourceUri": PageURI},
	}, h.show)
	mcp.AddTool(s, &mcp.Tool{
		Name:  "review_notes",
		Title: "Read a render's review notes",
		Description: "Read the review notes on a video, with the frame each one is about, its text, its drawings and a snapshot image of the frame with the drawing. " +
			"When the person has pressed Send to agent, it returns that send and marks it received, as `cav review wait` does. " +
			"Otherwise it returns every open note. Use it in chats without a shell; with a shell, `cav review wait` works too.",
	}, h.notes)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "review_request",
		Title:       "Review page request",
		Description: "Used by the review page itself: runs one of its HTTP requests against the review of a video opened with show_review.",
		Meta:        mcp.Meta{"ui": map[string]any{"resourceUri": PageURI, "visibility": []string{"app"}}},
	}, h.request)
	return s
}

// AppPage adds the shim and a content-sized layout to the review page. The page fills the
// window height, but the host sizes the frame from the page's own height, so in the frame
// the page takes its natural height and the wide layout a fixed one.
func AppPage(page, shim []byte) []byte {
	// The page stacks the sheet under the stage below 760 pixels. Chat frames are often 600
	// to 800 pixels wide and about 600 tall, so in the chat the two columns hold down to 600
	// pixels, with a narrower sheet, and the whole page takes the frame's height.
	head := "<style>html.cav-mcp, html.cav-mcp body { height: auto; }\n" +
		"@media (min-width: 600px) { html.cav-mcp .app { height: var(--cav-app-h, 600px); } }\n" +
		"@media (min-width: 600px) and (max-width: 759px) {\n" +
		"  html.cav-mcp .app { grid-template-columns: minmax(0, 1fr) 264px; grid-template-rows: auto minmax(0, 1fr); }\n" +
		"  html.cav-mcp .side { border-left: 1px solid var(--rule-3); border-top: 0; height: auto; }\n" +
		// The page sizes the stage from its width when stacked; here it fills the column.
		"  html.cav-mcp .stage-wrap { height: auto !important; }\n" +
		"}\n" +
		"</style>\n" +
		"<script>\n" + string(shim) + "\n</script>\n"
	i := bytes.Index(page, []byte("</head>"))
	if i < 0 {
		return append([]byte(head), page...)
	}
	out := make([]byte, 0, len(page)+len(head))
	out = append(out, page[:i]...)
	out = append(out, head...)
	return append(out, page[i:]...)
}

// Prepare probes a video and makes its preview proxy.
func Prepare(ctx context.Context, video, cacheDir string) (review.Source, string, error) {
	src, err := review.Probe(ctx, video)
	if err != nil {
		return src, "", err
	}
	if src.Frames <= 0 || src.FPS <= 0 {
		return src, "", fmt.Errorf("%s has no readable frames", video)
	}
	if src.SHA256, err = review.Digest(video); err != nil {
		return src, "", err
	}
	src.Render = video
	proxy, err := review.PreviewProxy(ctx, video, src.SHA256, cacheDir)
	return src, proxy, err
}

type handler struct {
	o    Options
	mu   sync.Mutex
	open map[string]*session // by absolute video path
}

// session is one video opened with show_review. It is prepared again when the file changes,
// as `cav review` does when an agent re-renders to the same path.
type session struct {
	srv     *review.Server
	modTime time.Time
	size    int64
	// seen is the changed file as the last request found it. As in review.Server.Watch, a
	// change is prepared only when two requests in a row find the same size and time, so a
	// render that is still being written in place keeps the earlier version on screen.
	seenMod  time.Time
	seenSize int64
	// failed is set when the changed file could not be prepared, so the next requests keep
	// the earlier render instead of preparing the same file again.
	failed bool
}

// session finds the review of a video, and prepares it again when the file changed. With
// activate false it only notes the change: a request such as the page posting a note must
// run against the render the page shows, so a new render goes live only through a request
// whose answer tells the page about it (show_review and GET /api/state).
func (h *handler) session(ctx context.Context, video string, create, activate bool) (*session, error) {
	abs, err := filepath.Abs(video)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s", video)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.open[abs]
	if s == nil && !create {
		// A host may send the page's requests to another server process than the one that
		// ran show_review (Claude Desktop does), so this process may not know the video yet.
		// The review file that show_review (or cav review) wrote next to it shows that it was
		// opened for review; any other file stays out of reach.
		if doc, _ := review.Paths(abs); !fileExists(doc) {
			return nil, fmt.Errorf("%s was not opened with show_review", video)
		}
	}
	if s != nil {
		if info.ModTime().Equal(s.modTime) && info.Size() == s.size {
			return s, nil
		}
		if !info.ModTime().Equal(s.seenMod) || info.Size() != s.seenSize {
			s.seenMod, s.seenSize, s.failed = info.ModTime(), info.Size(), false // still changing: check again next time
			return s, nil
		}
		if s.failed || !activate {
			return s, nil
		}
	}
	src, proxy, err := h.o.Prepare(ctx, abs, h.o.CacheDir)
	if err != nil && s != nil {
		// Keep reviewing the earlier render; the page shows the error until a new render works.
		s.failed = true
		s.srv.SetRenderError(err.Error())
		return s, nil
	}
	if err != nil {
		if s := h.earlier(abs, info, err); s != nil {
			return s, nil
		}
		return nil, err
	}
	store := review.Open(abs)
	if _, err := store.Update(func(d *review.Doc) error { d.Source = src; review.AddVersion(d, src); return nil }); err != nil {
		return nil, err
	}
	s = &session{modTime: info.ModTime(), size: info.Size(),
		srv: &review.Server{Store: store, Source: src, Proxy: proxy, Author: h.o.Author, CacheDir: h.o.CacheDir}}
	h.open[abs] = s
	return s, nil
}

// earlier rebuilds the session of a video that this process has not opened yet but cannot
// prepare now, for example because it is being rewritten in place. It uses the last render in
// the review file and its preview, so the page keeps the earlier render and shows the error,
// as a process that already had the session does. It returns nil when there is no usable
// earlier preview.
func (h *handler) earlier(abs string, info os.FileInfo, prepErr error) *session {
	store := review.Open(abs)
	if !fileExists(store.DocPath) {
		return nil
	}
	d, err := store.Load()
	if err != nil {
		return nil
	}
	// The review file sits in the project, so its digest must be one before it names a file.
	sha := d.Source.SHA256
	if len(sha) != 64 || strings.Trim(sha, "0123456789abcdef") != "" || d.Source.Frames <= 0 || d.Source.FPS <= 0 {
		return nil
	}
	proxy := filepath.Join(h.o.CacheDir, sha+".mp4")
	if !fileExists(proxy) {
		return nil
	}
	s := &session{modTime: info.ModTime(), size: info.Size(), seenMod: info.ModTime(), seenSize: info.Size(), failed: true,
		srv: &review.Server{Store: store, Source: d.Source, Proxy: proxy, Author: h.o.Author, CacheDir: h.o.CacheDir}}
	s.srv.SetRenderError(prepErr.Error())
	h.open[abs] = s
	return s
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

type showIn struct {
	Video string `json:"video,omitempty" jsonschema:"path of the rendered video; default: the newest review or MP4 in the renders folder"`
}

type showOut struct {
	Video      string  `json:"video"`
	ReviewFile string  `json:"reviewFile"`
	Frames     int     `json:"frames"`
	FPS        float64 `json:"fps"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Open       int     `json:"open"`
	Resolved   int     `json:"resolved"`
	// OpenNotes and Next repeat the text result. Some hosts give the model only the
	// structured result, and then this is all it learns from the call.
	OpenNotes []noteSummary `json:"openNotes"`
	Next      string        `json:"next"`
}

type noteSummary struct {
	ID    string `json:"id"`
	Frame int    `json:"frame"`
	// FrameEnd is set for a note on a range of frames.
	FrameEnd *int   `json:"frameEnd,omitempty"`
	Text     string `json:"text"`
}

func (h *handler) show(ctx context.Context, req *mcp.CallToolRequest, in showIn) (*mcp.CallToolResult, showOut, error) {
	video := in.Video
	if h.o.Resolve != nil {
		v, err := h.o.Resolve(video)
		if err != nil {
			return nil, showOut{}, err
		}
		video = v
	}
	if video == "" {
		return nil, showOut{}, errors.New("give the video to show")
	}
	s, err := h.session(ctx, video, true, true)
	if err != nil {
		return nil, showOut{}, err
	}
	d, err := s.srv.Store.Load()
	if err != nil {
		return nil, showOut{}, err
	}
	src := s.srv.Source
	out := showOut{Video: s.srv.Store.Video, ReviewFile: s.srv.Store.DocPath, Frames: src.Frames, FPS: src.FPS, Width: src.Width, Height: src.Height,
		OpenNotes: []noteSummary{}}
	var b strings.Builder
	for _, c := range d.Comments {
		if c.Status == "open" {
			out.Open++
			where := fmt.Sprintf("frame %d", c.Frame)
			if c.FrameEnd != nil {
				where = fmt.Sprintf("frames %d-%d", c.Frame, *c.FrameEnd)
			}
			fmt.Fprintf(&b, "\n  %s %s: %s", c.ID, where, firstLine(c.Text))
			out.OpenNotes = append(out.OpenNotes, noteSummary{ID: c.ID, Frame: c.Frame, FrameEnd: c.FrameEnd, Text: firstLine(c.Text)})
		} else {
			out.Resolved++
		}
	}
	// The path is named as data, never pasted into a command: no one quoting is safe in
	// every shell the agent may use.
	out.Next = "Hosts that draw MCP Apps show the page in the chat; others show only this result. " +
		"When the person presses Send to agent, call review_notes with this video path. " +
		"With a shell, `cav review wait` with the path as its argument, quoted for your shell, works too; " +
		"`cav review` with the same path opens the full review in a browser."
	text := fmt.Sprintf("Showing the review page of this video (%d frames at %g fps). Notes: %d open, %d resolved.%s\nVideo path: %s\n%s",
		src.Frames, src.FPS, out.Open, out.Resolved, b.String(), out.Video, out.Next)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
}

type notesIn struct {
	Video string `json:"video,omitempty" jsonschema:"path of the rendered video; default: the newest review or MP4 in the renders folder"`
}

type notesOut struct {
	Video string `json:"video"`
	// Send is the number of the send this call received, or 0 when nothing was sent and the
	// result lists every open note instead.
	Send  int        `json:"send"`
	Notes []noteFull `json:"notes"`
	Next  string     `json:"next"`
}

type noteFull struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"`
	Frame    int            `json:"frame"`
	FrameEnd *int           `json:"frameEnd,omitempty"`
	Timecode string         `json:"timecode"`
	Text     string         `json:"text"`
	Shapes   []string       `json:"shapes,omitempty"`
	Fragment string         `json:"fragment,omitempty"`
	Snapshot string         `json:"snapshot,omitempty"`
	Replies  []review.Reply `json:"replies,omitempty"`
	// OnOlderRender is set for a note made on an earlier render of this file.
	OnOlderRender bool `json:"onOlderRender,omitempty"`
}

// maxSnapshots and maxSnapshotBytes cap the images one review_notes call returns. Hosts
// pass the whole answer through their message channel (see MaxInline), and a browser review
// can hold full-size snapshots.
const (
	maxSnapshots     = 8
	maxSnapshotBytes = 8 << 20
)

// The page accepts long notes and replies, so their text is capped too. The whole answer
// stays under MaxInline (24 MB): the text appears twice (content and structuredContent), at
// most maxNotes × (maxNoteText + maxReplies × maxReplyText) = 50 × 14 KB = 700 KB each, and
// the snapshots grow by a third in base64, so 8 MB of them take under 11 MB.
const (
	maxNotes     = 50
	maxNoteText  = 4000
	maxReplies   = 10
	maxReplyText = 1000
)

// clip shortens s to at most n bytes, on a rune boundary, and marks the cut.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + " […]"
}

// notes is review_notes: `cav review wait` without the wait, for chats without a shell. It
// receives the pending send, or lists the open notes when there is none, and returns each
// note's snapshot as an image so that the agent sees what the reviewer drew.
func (h *handler) notes(ctx context.Context, req *mcp.CallToolRequest, in notesIn) (*mcp.CallToolResult, notesOut, error) {
	video := in.Video
	if h.o.Resolve != nil {
		v, err := h.o.Resolve(video)
		if err != nil {
			return nil, notesOut{}, err
		}
		video = v
	}
	abs, err := filepath.Abs(video)
	if err != nil {
		return nil, notesOut{}, err
	}
	store := h.store(abs)
	if !fileExists(store.DocPath) {
		return nil, notesOut{}, fmt.Errorf("%s has no review yet: show it with show_review first", video)
	}

	// Build the answer and claim the send in one update, under the store's lock: a note the
	// reviewer edits, replies to, resolves or deletes meanwhile then cannot reach the agent in
	// an earlier version while the send is marked received. The send is marked only once the
	// answer is complete, so an answer that fails on the way leaves it pending.
	var out notesOut
	var content []mcp.Content
	_, err = store.Update(func(d *review.Doc) error {
		var claim bool
		out, content, claim = answer(store, d, abs)
		if !claim {
			return errNoClaim
		}
		now := time.Now().UTC()
		review.Pending(d).DeliveredAt = &now
		return nil
	})
	if err != nil && !errors.Is(err, errNoClaim) {
		return nil, notesOut{}, err
	}
	return &mcp.CallToolResult{Content: content}, out, nil
}

// errNoClaim stops the update in notes without saving when there is no send to claim.
var errNoClaim = errors.New("nothing to claim")

// answer builds the review_notes answer from d: the pending send, or the open notes when
// there is none. claim reports whether the whole send fits, so that it may be marked received.
func answer(store *review.Store, d *review.Doc, abs string) (out notesOut, content []mcp.Content, claim bool) {
	out = notesOut{Video: abs, Notes: []noteFull{}}
	cur := d.Source.SHA256
	if len(cur) > 12 {
		cur = cur[:12]
	}
	want := map[string]bool{}
	if s := review.Pending(d); s != nil {
		out.Send = s.N
		for _, id := range s.Comments {
			want[id] = true
		}
	}
	truncated := 0
	for _, c := range d.Comments {
		if !((out.Send > 0 && want[c.ID]) || (out.Send == 0 && c.Status == "open")) {
			continue
		}
		if len(out.Notes) == maxNotes {
			truncated++
			continue
		}
		// Every string comes from the review file, which can be edited by hand, so each is
		// capped: otherwise one field could push the answer past MaxInline.
		n := noteFull{ID: clip(c.ID, 64), Status: clip(c.Status, 16), Frame: c.Frame, FrameEnd: c.FrameEnd,
			Timecode: clip(c.Timecode, 32), Text: clip(c.Text, maxNoteText), Fragment: clip(c.Fragment, 80),
			OnOlderRender: cur != "" && c.Version != cur}
		for i, r := range c.Replies {
			if i == maxReplies {
				break
			}
			r.Text = clip(r.Text, maxReplyText)
			r.Author = clip(r.Author, 80)
			n.Replies = append(n.Replies, r)
		}
		// The kinds of drawing, once each: a note can hold any number of shapes.
		seen := map[string]bool{}
		for _, sh := range c.Shapes {
			if t := clip(sh.Type, 16); !seen[t] && len(n.Shapes) < 8 {
				seen[t] = true
				n.Shapes = append(n.Shapes, t)
			}
		}
		n.Snapshot = snapshotPath(store, c.Snapshot)
		out.Notes = append(out.Notes, n)
	}

	// A send cut short stays pending, so nothing in it is lost.
	claim = out.Send > 0 && truncated == 0
	var b strings.Builder
	switch {
	case out.Send > 0 && !claim:
		fmt.Fprintf(&b, "Send #%d on %s has %d notes; here are the first %d. The send stays pending: read all of it with `cav review export` in a shell.\n",
			out.Send, abs, len(out.Notes)+truncated, len(out.Notes))
	case out.Send > 0:
		fmt.Fprintf(&b, "Send #%d on %s: %d note(s). It is now marked received.\n", out.Send, abs, len(out.Notes))
	default:
		fmt.Fprintf(&b, "Nothing was sent from the review of %s. Its open notes: %d.\n", abs, len(out.Notes))
		if truncated > 0 {
			fmt.Fprintf(&b, "Only the first %d are listed; %d more are open.\n", len(out.Notes), truncated)
		}
	}
	content = []mcp.Content{nil} // the text goes first, once it is complete
	images, budget := 0, maxSnapshotBytes
	for _, n := range out.Notes {
		where := fmt.Sprintf("frame %d (%s)", n.Frame, n.Timecode)
		if n.FrameEnd != nil {
			where = fmt.Sprintf("frames %d-%d (from %s)", n.Frame, *n.FrameEnd, n.Timecode)
		}
		older := ""
		if n.OnOlderRender {
			older = " [made on an older render]"
		}
		fmt.Fprintf(&b, "\n%s %s, %s%s\n", n.ID, n.Status, where, older)
		if n.Text != "" {
			fmt.Fprintf(&b, "  %s\n", strings.ReplaceAll(n.Text, "\n", "\n  "))
		}
		if len(n.Shapes) > 0 {
			fmt.Fprintf(&b, "  drawing: %s (%s)\n", strings.Join(n.Shapes, ", "), n.Fragment)
		}
		for _, r := range n.Replies {
			fmt.Fprintf(&b, "  reply from %s: %s\n", r.Author, r.Text)
		}
		if n.Snapshot == "" {
			continue
		}
		info, err := os.Lstat(n.Snapshot)
		switch {
		case err != nil:
		case images >= maxSnapshots || info.Size() > int64(budget):
			fmt.Fprintf(&b, "  snapshot: %s (not attached: the answer is at its size limit)\n", n.Snapshot)
		default:
			if png, err := os.ReadFile(n.Snapshot); err == nil && len(png) <= budget {
				fmt.Fprintf(&b, "  snapshot: image %d below\n", images+1)
				content = append(content, &mcp.ImageContent{Data: png, MIMEType: "image/png"})
				images++
				budget -= len(png)
			}
		}
	}
	out.Next = "Fix the notes, re-render to the same file, then resolve each one with a short note on what changed " +
		"(`cav review resolve <id> --note ...` with a shell, or Resolve in the review page)."
	fmt.Fprintf(&b, "\n%s", out.Next)
	content[0] = &mcp.TextContent{Text: b.String()}
	return out, content, claim
}

// store returns the review store of a video, shared with its open session when there is
// one: a Store serializes its read-modify-write with its own lock, so two stores on the same
// file in one process could each overwrite the other's change.
func (h *handler) store(abs string) *review.Store {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s := h.open[abs]; s != nil {
		return s.srv.Store
	}
	return review.Open(abs)
}

// snapshotPath finds a note's snapshot; see review.Store.SnapshotFile.
func snapshotPath(store *review.Store, p string) string { return store.SnapshotFile(p) }

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if s == "" {
		return "(drawing only)"
	}
	if r := []rune(s); len(r) > 100 {
		return string(r[:100]) + "…"
	}
	return s
}

type requestIn struct {
	Video  string `json:"video" jsonschema:"the video, as show_review returned it"`
	Method string `json:"method" jsonschema:"GET, POST or DELETE"`
	Path   string `json:"path" jsonschema:"the page's request path, such as /api/state"`
	Body   string `json:"body,omitempty" jsonschema:"the JSON request body"`
}

type requestOut struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	// Body holds a text answer (JSON), DataURL a binary one (video, image).
	Body    string `json:"body,omitempty"`
	DataURL string `json:"dataUrl,omitempty"`
}

func (h *handler) request(ctx context.Context, req *mcp.CallToolRequest, in requestIn) (*mcp.CallToolResult, requestOut, error) {
	if !strings.HasPrefix(in.Path, "/") {
		return nil, requestOut{}, fmt.Errorf("path must start with /, got %q", in.Path)
	}
	activate := (in.Method == "" || in.Method == http.MethodGet) && in.Path == "/api/state"
	s, err := h.session(ctx, in.Video, false, activate)
	if err != nil {
		return nil, requestOut{}, err
	}
	out := Do(s.srv.Handler(), in.Method, in.Path, in.Body)
	summary := fmt.Sprintf("%s %s: %d", in.Method, in.Path, out.Status)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: summary}}}, out, nil
}

// Do runs one request through the review handler in-process and packs the answer for the
// page. The request looks like the page's own (host 127.0.0.1, JSON body), so the handler's
// CSRF guard treats it as it treats the browser page.
func Do(hd http.Handler, method, path, body string) requestOut {
	if method == "" {
		method = http.MethodGet
	}
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	if method != http.MethodGet && method != http.MethodHead {
		r.Header.Set("Content-Type", "application/json")
	}
	w := &capWriter{header: http.Header{}, code: http.StatusOK}
	hd.ServeHTTP(w, r)
	ct := w.header.Get("Content-Type")
	out := requestOut{Status: w.code, ContentType: ct}
	switch {
	case w.over:
		out.Status, out.ContentType = http.StatusRequestEntityTooLarge, "application/json"
		out.Body = fmt.Sprintf(`{"error":"the preview is more than the %d MB the chat view takes: review it in the browser with cav review"}`, MaxInline>>20)
	case isText(ct):
		out.Body = w.body.String()
	default:
		out.DataURL = "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(w.body.Bytes())
	}
	return out
}

// capWriter keeps at most MaxInline bytes of a response. A longer one fails the write, which
// stops http.ServeFile, so a long render is never read into memory whole.
type capWriter struct {
	header http.Header
	code   int
	wrote  bool
	body   bytes.Buffer
	over   bool
}

var errTooLarge = errors.New("response larger than the inline limit")

func (w *capWriter) Header() http.Header { return w.header }

func (w *capWriter) WriteHeader(code int) {
	if !w.wrote {
		w.code, w.wrote = code, true
	}
}

func (w *capWriter) Write(b []byte) (int, error) {
	w.wrote = true
	if w.over || w.body.Len()+len(b) > MaxInline {
		w.over = true
		w.body.Reset()
		return 0, errTooLarge
	}
	return w.body.Write(b)
}

func isText(ct string) bool {
	return ct == "" || strings.HasPrefix(ct, "application/json") || strings.HasPrefix(ct, "text/")
}
