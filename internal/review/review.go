// Package review stores the notes a reviewer leaves on a rendered video, in a sidecar
// JSON file that an agent can read back: <video>.review.json next to the video, plus one
// PNG per comment with the drawing burned in.
package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const Schema = "cav-review/1"

type Doc struct {
	Schema   string    `json:"schema"`
	Source   Source    `json:"source"`
	Comments []Comment `json:"comments"`
	Sends    []Send    `json:"sends,omitempty"`
	// Server is the running review page, while `cav review` serves it; tools such as the
	// Claude Code plugin read it to link to the page.
	Server *ServerInfo `json:"server,omitempty"`
	// Versions lists every render of this file the review has seen, oldest first, so the
	// page can compare the current render with an earlier one.
	Versions []Version `json:"versions,omitempty"`
}

type Version struct {
	SHA256 string    `json:"sha256"`
	Frames int       `json:"frames"`
	FPS    float64   `json:"fps"`
	Width  int       `json:"width"`
	Height int       `json:"height"`
	At     time.Time `json:"at"`
}

// AddVersion records a render once, by content.
func AddVersion(d *Doc, src Source) {
	for _, v := range d.Versions {
		if v.SHA256 == src.SHA256 {
			return
		}
	}
	d.Versions = append(d.Versions, Version{SHA256: src.SHA256, Frames: src.Frames, FPS: src.FPS, Width: src.Width, Height: src.Height, At: time.Now().UTC()})
}

type ServerInfo struct {
	URL     string    `json:"url"`
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
}

type Source struct {
	Render string  `json:"render"`
	SHA256 string  `json:"sha256"`
	FPS    float64 `json:"fps"`
	Frames int     `json:"frames"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
}

type Shape struct {
	Type  string       `json:"type"` // pen, arrow, rect, ellipse
	Pts   [][2]float64 `json:"pts"`  // 0..1 fractions of the frame, y down
	Color string       `json:"color,omitempty"`
}

type Reply struct {
	Author string    `json:"author"`
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
}

type Comment struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"` // the render's sha256, first 12 hex digits
	Frame     int       `json:"frame"`
	FrameEnd  *int      `json:"frameEnd,omitempty"`
	Timecode  string    `json:"timecode"`
	Fragment  string    `json:"fragment"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	Status    string    `json:"status"` // open, resolved
	Text      string    `json:"text"`
	Shapes    []Shape   `json:"shapes,omitempty"`
	Snapshot  string    `json:"snapshot,omitempty"`
	Replies   []Reply   `json:"replies,omitempty"`
	// Sent is the number of the send that handed this comment to the agent (0: not sent).
	Sent         int        `json:"sent,omitempty"`
	ResolvedAt   *time.Time `json:"resolvedAt,omitempty"`
	ResolvedNote string     `json:"resolvedNote,omitempty"`
}

// Send is one press of "Send to agent".
type Send struct {
	N           int        `json:"n"`
	At          time.Time  `json:"at"`
	Comments    []string   `json:"comments"`
	DeliveredAt *time.Time `json:"deliveredAt,omitempty"`
}

// Paths derives the sidecar locations from the video path.
func Paths(video string) (doc, snapDir string) {
	ext := filepath.Ext(video)
	base := strings.TrimSuffix(video, ext)
	return base + ".review.json", filepath.Join(filepath.Dir(video), "review", filepath.Base(base))
}

// Store guards one review file. The server and the CLI both write it, so every change
// re-reads the file first and writes it back atomically.
type Store struct {
	Video   string
	DocPath string
	SnapDir string
	mu      sync.Mutex
}

func Open(video string) *Store {
	d, s := Paths(video)
	return &Store{Video: video, DocPath: d, SnapDir: s}
}

// Load reads the review file. A missing file gives an empty document.
func (s *Store) Load() (*Doc, error) {
	b, err := os.ReadFile(s.DocPath)
	if errors.Is(err, os.ErrNotExist) {
		return &Doc{Schema: Schema, Comments: []Comment{}}, nil
	}
	if err != nil {
		return nil, err
	}
	d := &Doc{}
	if err := json.Unmarshal(b, d); err != nil {
		return nil, fmt.Errorf("%s: %w", s.DocPath, err)
	}
	if d.Comments == nil {
		d.Comments = []Comment{}
	}
	return d, nil
}

func (s *Store) save(d *Doc) error {
	d.Schema = Schema
	sort.SliceStable(d.Comments, func(i, j int) bool { return d.Comments[i].Frame < d.Comments[j].Frame })
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.DocPath + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.DocPath)
}

// Update applies fn to the current document and saves it.
func (s *Store) Update(fn func(d *Doc) error) (*Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.Load()
	if err != nil {
		return nil, err
	}
	if err := fn(d); err != nil {
		return nil, err
	}
	return d, s.save(d)
}

// NextID returns c_01, c_02, ... after the highest existing id.
func NextID(d *Doc) string {
	n := 0
	for _, c := range d.Comments {
		var k int
		if _, err := fmt.Sscanf(c.ID, "c_%d", &k); err == nil && k > n {
			n = k
		}
	}
	return fmt.Sprintf("c_%02d", n+1)
}

// Find returns a pointer to the comment with the id.
func Find(d *Doc, id string) *Comment {
	for i := range d.Comments {
		if d.Comments[i].ID == id {
			return &d.Comments[i]
		}
	}
	return nil
}

// Timecode formats a frame as HH:MM:SS:FF.
func Timecode(frame int, fps float64) string {
	if fps <= 0 {
		return ""
	}
	f := int(math.Round(fps))
	secs := frame / f
	return fmt.Sprintf("%02d:%02d:%02d:%02d", secs/3600, secs/60%60, secs%60, frame%f)
}

// Fragment builds a W3C media fragment: the time range, and the box around the shapes.
func Fragment(frame int, frameEnd *int, fps float64, shapes []Shape) string {
	if fps <= 0 {
		return ""
	}
	t0 := float64(frame) / fps
	t1 := t0 + 1/fps
	if frameEnd != nil {
		t1 = float64(*frameEnd+1) / fps
	}
	s := fmt.Sprintf("#t=%.4f,%.4f", t0, t1)
	minX, minY, maxX, maxY := 1.0, 1.0, 0.0, 0.0
	for _, sh := range shapes {
		for _, p := range sh.Pts {
			minX, minY = math.Min(minX, p[0]), math.Min(minY, p[1])
			maxX, maxY = math.Max(maxX, p[0]), math.Max(maxY, p[1])
		}
	}
	if maxX > minX && maxY > minY {
		clamp := func(v float64) float64 { return math.Max(0, math.Min(100, v*100)) }
		s += fmt.Sprintf("&xywh=percent:%.0f,%.0f,%.0f,%.0f", clamp(minX), clamp(minY), clamp(maxX-minX), clamp(maxY-minY))
	}
	return s
}

// Digest returns the sha256 of a file.
func Digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Pending returns the newest send not yet delivered to the agent, or nil.
func Pending(d *Doc) *Send {
	for i := range d.Sends {
		if d.Sends[i].DeliveredAt == nil {
			return &d.Sends[i]
		}
	}
	return nil
}
