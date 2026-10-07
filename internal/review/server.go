package review

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Server serves the review page, the proxy video and the comment API for one render.
type Server struct {
	Store  *Store
	Source Source // probed from the render, with SHA256 set
	Proxy  string // the all-intra copy the page plays
	Page   []byte
	// PagePath, when set, is read on every request instead of Page (for working on the page).
	PagePath string
	Author   string
	Changed  func() // called after every change (for logging); may be nil
	// Prepare probes a new render and makes its proxy; Watch calls it when the file changes.
	Prepare func(ctx context.Context) (Source, string, error)

	mu        sync.RWMutex
	preparing bool
	lastError string
}

func (s *Server) current() (Source, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Source, s.Proxy
}

func (s *Server) version() string {
	src, _ := s.current()
	if len(src.SHA256) >= 12 {
		return src.SHA256[:12]
	}
	return src.SHA256
}

// Watch reloads the render when its file changes (an agent re-rendered to the same path).
// It waits until the file has stopped growing before it prepares the new proxy.
func (s *Server) Watch(ctx context.Context, every time.Duration) {
	if s.Prepare == nil {
		return
	}
	info, _ := os.Stat(s.Store.Video)
	var lastMod time.Time
	var lastSize int64
	if info != nil {
		lastMod, lastSize = info.ModTime(), info.Size()
	}
	pendingSize := int64(-1)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		info, err := os.Stat(s.Store.Video)
		if err != nil || (info.ModTime().Equal(lastMod) && info.Size() == lastSize) {
			pendingSize = -1
			continue
		}
		if info.Size() != pendingSize {
			pendingSize = info.Size() // still being written: check again next tick
			continue
		}
		s.mu.Lock()
		s.preparing = true
		s.mu.Unlock()
		src, proxy, err := s.Prepare(ctx)
		s.mu.Lock()
		s.preparing = false
		if err != nil {
			s.lastError = err.Error()
		} else {
			s.Source, s.Proxy, s.lastError = src, proxy, ""
		}
		s.mu.Unlock()
		if err == nil {
			s.Store.Update(func(d *Doc) error { d.Source = src; return nil })
			s.changed()
		}
		lastMod, lastSize, pendingSize = info.ModTime(), info.Size(), -1
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		page := s.Page
		if s.PagePath != "" {
			if b, err := os.ReadFile(s.PagePath); err == nil {
				page = b
			}
		}
		w.Write(page)
	})
	mux.HandleFunc("GET /video", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		_, proxy := s.current()
		http.ServeFile(w, r, proxy)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.Store.Load()
		if err != nil {
			httpErr(w, 500, err)
			return
		}
		src, _ := s.current()
		s.mu.RLock()
		preparing, lastErr := s.preparing, s.lastError
		s.mu.RUnlock()
		writeJSON(w, map[string]any{"source": src, "version": s.version(), "author": s.Author, "doc": d,
			"name": filepath.Base(s.Store.Video), "preparing": preparing, "renderError": lastErr})
	})
	mux.HandleFunc("GET /api/snapshot/{id}", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.Store.Load()
		if err != nil {
			httpErr(w, 500, err)
			return
		}
		c := Find(d, r.PathValue("id"))
		if c == nil || c.Snapshot == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, c.Snapshot)
	})
	mux.HandleFunc("POST /api/comments", s.addComment)
	mux.HandleFunc("POST /api/comments/{id}", s.editComment)
	mux.HandleFunc("DELETE /api/comments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var snap string
		_, err := s.Store.Update(func(d *Doc) error {
			for i, c := range d.Comments {
				if c.ID == id {
					snap = c.Snapshot
					d.Comments = append(d.Comments[:i], d.Comments[i+1:]...)
					return nil
				}
			}
			return errNotFound
		})
		if err != nil {
			httpErr(w, statusOf(err), err)
			return
		}
		if snap != "" {
			os.Remove(snap)
		}
		s.changed()
		writeJSON(w, map[string]any{"deleted": id})
	})
	mux.HandleFunc("POST /api/send", func(w http.ResponseWriter, r *http.Request) {
		var sent *Send
		_, err := s.Store.Update(func(d *Doc) error {
			n := len(d.Sends) + 1
			var ids []string
			for i := range d.Comments {
				c := &d.Comments[i]
				if c.Status == "open" && c.Sent == 0 {
					c.Sent = n
					ids = append(ids, c.ID)
				}
			}
			if len(ids) == 0 {
				return errNothing
			}
			d.Sends = append(d.Sends, Send{N: n, At: time.Now().UTC(), Comments: ids})
			sent = &d.Sends[len(d.Sends)-1]
			return nil
		})
		if err != nil {
			httpErr(w, statusOf(err), err)
			return
		}
		s.changed()
		writeJSON(w, sent)
	})
	return guard(mux)
}

var (
	errNotFound = errors.New("no such comment")
	errNothing  = errors.New("no new open comments to send")
)

func statusOf(err error) int {
	switch {
	case errors.Is(err, errNotFound):
		return 404
	case errors.Is(err, errNothing):
		return 409
	}
	return 400
}

func (s *Server) changed() {
	if s.Changed != nil {
		s.Changed()
	}
}

type commentInput struct {
	Frame    int     `json:"frame"`
	FrameEnd *int    `json:"frameEnd"`
	Text     string  `json:"text"`
	Shapes   []Shape `json:"shapes"`
	Snapshot string  `json:"snapshot"` // data:image/png;base64,...
}

func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	var in commentInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&in); err != nil {
		httpErr(w, 400, err)
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" && len(in.Shapes) == 0 {
		httpErr(w, 400, errors.New("write a note or draw something"))
		return
	}
	if cur, _ := s.current(); in.Frame < 0 || (cur.Frames > 0 && in.Frame >= cur.Frames) {
		httpErr(w, 400, fmt.Errorf("frame %d is outside the video (0-%d)", in.Frame, cur.Frames-1))
		return
	}
	if in.FrameEnd != nil && *in.FrameEnd <= in.Frame {
		in.FrameEnd = nil
	}
	png, err := decodePNG(in.Snapshot)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	var made Comment
	src, _ := s.current()
	_, err = s.Store.Update(func(d *Doc) error {
		d.Source = src
		c := Comment{
			ID: NextID(d), Version: s.version(), Frame: in.Frame, FrameEnd: in.FrameEnd,
			Timecode: Timecode(in.Frame, src.FPS), Fragment: Fragment(in.Frame, in.FrameEnd, src.FPS, in.Shapes),
			Author: s.Author, CreatedAt: time.Now().UTC(), Status: "open", Text: in.Text, Shapes: in.Shapes,
		}
		if png != nil {
			if err := os.MkdirAll(s.Store.SnapDir, 0o755); err != nil {
				return err
			}
			c.Snapshot = filepath.Join(s.Store.SnapDir, fmt.Sprintf("%s_f%d.png", c.ID, c.Frame))
			if err := os.WriteFile(c.Snapshot, png, 0o644); err != nil {
				return err
			}
		}
		d.Comments = append(d.Comments, c)
		made = c
		return nil
	})
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	s.changed()
	writeJSON(w, made)
}

func (s *Server) editComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status *string `json:"status"`
		Text   *string `json:"text"`
		Reply  string  `json:"reply"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		httpErr(w, 400, err)
		return
	}
	var out Comment
	_, err := s.Store.Update(func(d *Doc) error {
		c := Find(d, r.PathValue("id"))
		if c == nil {
			return errNotFound
		}
		if in.Status != nil {
			if err := SetStatus(c, *in.Status, ""); err != nil {
				return err
			}
		}
		if in.Text != nil {
			c.Text = strings.TrimSpace(*in.Text)
		}
		if t := strings.TrimSpace(in.Reply); t != "" {
			c.Replies = append(c.Replies, Reply{Author: s.Author, At: time.Now().UTC(), Text: t})
		}
		out = *c
		return nil
	})
	if err != nil {
		httpErr(w, statusOf(err), err)
		return
	}
	s.changed()
	writeJSON(w, out)
}

// SetStatus opens or resolves a comment. Reopening clears the send, so the next Send
// to agent hands it over again.
func SetStatus(c *Comment, status, note string) error {
	switch status {
	case "resolved":
		now := time.Now().UTC()
		c.Status, c.ResolvedAt, c.ResolvedNote = "resolved", &now, note
	case "open":
		c.Status, c.ResolvedAt, c.ResolvedNote, c.Sent = "open", nil, "", 0
	default:
		return fmt.Errorf("status must be open or resolved, got %q", status)
	}
	return nil
}

func decodePNG(dataURL string) ([]byte, error) {
	if dataURL == "" {
		return nil, nil
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, errors.New("snapshot must be a PNG data URL")
	}
	b, err := base64.StdEncoding.DecodeString(dataURL[len(prefix):])
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	if len(b) < 8 || string(b[1:4]) != "PNG" {
		return nil, errors.New("snapshot is not a PNG")
	}
	return b, nil
}

// guard refuses requests that do not come from the page itself: another site must not be
// able to post comments through the browser (CSRF), or reach the server through a DNS name
// that points at 127.0.0.1 (DNS rebinding).
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "forbidden host", 403)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
				http.Error(w, "forbidden origin", 403)
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") && r.Method != "DELETE" {
				http.Error(w, "send JSON", 415)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func httpErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
