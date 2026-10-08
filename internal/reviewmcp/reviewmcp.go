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
			"It is for a quick look: for frame-by-frame review run `cav review` in a shell. Read the notes with `cav review wait` or `cav review export`.",
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
			"After the person presses Send to agent, read the notes with `cav review wait <video>`.",
		Meta: mcp.Meta{"ui": map[string]any{"resourceUri": PageURI}, "ui/resourceUri": PageURI},
	}, h.show)
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
	head := "<style>html.cav-mcp, html.cav-mcp body { height: auto; }\n" +
		"@media (min-width: 901px) { html.cav-mcp .app { height: var(--cav-app-h, 640px); } }</style>\n" +
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

func (h *handler) session(ctx context.Context, video string, create bool) (*session, error) {
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
		return nil, fmt.Errorf("%s was not opened with show_review", video)
	}
	if s != nil {
		if info.ModTime().Equal(s.modTime) && info.Size() == s.size {
			return s, nil
		}
		if !info.ModTime().Equal(s.seenMod) || info.Size() != s.seenSize {
			s.seenMod, s.seenSize, s.failed = info.ModTime(), info.Size(), false // still changing: check again next time
			return s, nil
		}
		if s.failed {
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
	s, err := h.session(ctx, video, true)
	if err != nil {
		return nil, showOut{}, err
	}
	d, err := s.srv.Store.Load()
	if err != nil {
		return nil, showOut{}, err
	}
	src := s.srv.Source
	out := showOut{Video: s.srv.Store.Video, ReviewFile: s.srv.Store.DocPath, Frames: src.Frames, FPS: src.FPS, Width: src.Width, Height: src.Height}
	var b strings.Builder
	for _, c := range d.Comments {
		if c.Status == "open" {
			out.Open++
			where := fmt.Sprintf("frame %d", c.Frame)
			if c.FrameEnd != nil {
				where = fmt.Sprintf("frames %d-%d", c.Frame, *c.FrameEnd)
			}
			fmt.Fprintf(&b, "\n  %s %s: %s", c.ID, where, firstLine(c.Text))
		} else {
			out.Resolved++
		}
	}
	text := fmt.Sprintf("Showing the review page of %s (%d frames at %g fps) in the chat. Notes: %d open, %d resolved.%s\n"+
		"The page plays a small preview. When the person presses Send to agent, read the notes with: cav review wait %s",
		out.Video, src.Frames, src.FPS, out.Open, out.Resolved, b.String(), out.Video)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
}

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
	s, err := h.session(ctx, in.Video, false)
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
