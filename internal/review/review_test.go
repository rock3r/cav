package review

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTimecodeAndFragment(t *testing.T) {
	if got := Timecode(112, 60); got != "00:00:01:52" {
		t.Fatalf("timecode %s", got)
	}
	if got := Timecode(3600*30+5, 29.97); got != "01:00:00:05" {
		t.Fatalf("29.97 timecode %s", got)
	}
	end := 130
	got := Fragment(112, &end, 60, []Shape{{Type: "rect", Pts: [][2]float64{{0.4, 0.2}, {0.65, 0.35}}}})
	if got != "#t=1.8667,2.1833&xywh=percent:40,20,25,15" {
		t.Fatalf("fragment %s", got)
	}
}

func pngDataURL(t *testing.T) string {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func newTestServer(t *testing.T) (*Server, http.Handler) {
	dir := t.TempDir()
	video := filepath.Join(dir, "v3.mp4")
	os.WriteFile(video, []byte("not really a video"), 0o644)
	s := &Server{Store: Open(video), Source: Source{Render: video, SHA256: "abcdef0123456789", FPS: 30, Frames: 120, Width: 64, Height: 36},
		Proxy: video, Page: []byte("<html>"), Author: "seb"}
	return s, s.Handler()
}

func do(t *testing.T, h http.Handler, method, path string, body any, hdr map[string]string) *httptest.ResponseRecorder {
	var r *bytes.Reader
	if body != nil {
		j, _ := json.Marshal(body)
		r = bytes.NewReader(j)
	} else {
		r = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "http://127.0.0.1:8790"+path, r)
	req.Host = "127.0.0.1:8790"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestCommentSendDeliverLoop(t *testing.T) {
	s, h := newTestServer(t)
	w := do(t, h, "POST", "/api/comments", map[string]any{"frame": 57, "frameEnd": 70, "text": "fade too slow", "snapshot": pngDataURL(t),
		"shapes": []Shape{{Type: "arrow", Pts: [][2]float64{{0.1, 0.1}, {0.2, 0.3}}}}}, nil)
	if w.Code != 200 {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	var c Comment
	json.Unmarshal(w.Body.Bytes(), &c)
	if c.ID != "c_01" || c.Timecode != "00:00:01:27" || c.Version != "abcdef012345" || c.Snapshot == "" {
		t.Fatalf("comment %+v", c)
	}
	if _, err := os.Stat(c.Snapshot); err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
	if w := do(t, h, "POST", "/api/comments", map[string]any{"frame": 500, "text": "x"}, nil); w.Code != 400 {
		t.Fatalf("frame outside the video accepted: %d", w.Code)
	}

	if w := do(t, h, "POST", "/api/send", map[string]any{}, nil); w.Code != 200 {
		t.Fatalf("send: %d %s", w.Code, w.Body)
	}
	if w := do(t, h, "POST", "/api/send", map[string]any{}, nil); w.Code != 409 {
		t.Fatalf("second send with nothing new: %d", w.Code)
	}
	d, _ := s.Store.Load()
	p := Pending(d)
	if p == nil || p.N != 1 || len(p.Comments) != 1 || d.Comments[0].Sent != 1 {
		t.Fatalf("pending send %+v / %+v", p, d.Comments)
	}
	// Reopening clears the send, so the next send hands it over again.
	if w := do(t, h, "POST", "/api/comments/c_01", map[string]any{"status": "resolved"}, nil); w.Code != 200 {
		t.Fatalf("resolve: %d", w.Code)
	}
	if w := do(t, h, "POST", "/api/comments/c_01", map[string]any{"status": "open"}, nil); w.Code != 200 {
		t.Fatalf("reopen: %d", w.Code)
	}
	d, _ = s.Store.Load()
	if d.Comments[0].Sent != 0 || d.Comments[0].Status != "open" {
		t.Fatalf("reopen did not clear the send: %+v", d.Comments[0])
	}
	if w := do(t, h, "DELETE", "/api/comments/c_01", nil, nil); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if _, err := os.Stat(c.Snapshot); !os.IsNotExist(err) {
		t.Fatal("snapshot left behind after delete")
	}
}

func TestGuardRefusesCrossSiteRequests(t *testing.T) {
	_, h := newTestServer(t)
	if w := do(t, h, "POST", "/api/send", map[string]any{}, map[string]string{"Origin": "https://evil.example"}); w.Code != 403 {
		t.Fatalf("foreign origin: %d", w.Code)
	}
	req := httptest.NewRequest("GET", "/api/state", nil)
	req.Host = "evil.example"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("foreign host: %d", w.Code)
	}
	req = httptest.NewRequest("POST", "/api/send", strings.NewReader("a=b"))
	req.Host = "127.0.0.1:8790"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 415 {
		t.Fatalf("form post: %d", w.Code)
	}
}

func TestWatchReloadsAChangedRender(t *testing.T) {
	s, _ := newTestServer(t)
	calls := 0
	s.Prepare = func(ctx context.Context) (Source, string, error) {
		calls++
		return Source{Render: s.Store.Video, SHA256: "ffff00001111222233", FPS: 30, Frames: 240, Width: 64, Height: 36}, s.Store.Video, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Watch(ctx, 20*time.Millisecond); close(done) }()
	defer func() { cancel(); <-done }()
	time.Sleep(60 * time.Millisecond)
	os.WriteFile(s.Store.Video, []byte("a new render, longer than before"), 0o644)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if d, _ := s.Store.Load(); d.Source.Frames == 240 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	d, _ := s.Store.Load()
	if s.version() != "ffff00001111" || calls != 1 || d.Source.Frames != 240 {
		t.Fatalf("version %s, %d prepares, review file %+v", s.version(), calls, d.Source)
	}
}

func TestVersionsAndPeaks(t *testing.T) {
	s, h := newTestServer(t)
	s.CacheDir = t.TempDir()
	old := strings.Repeat("a", 64)
	os.WriteFile(filepath.Join(s.CacheDir, old+".mp4"), []byte("old proxy"), 0o644)
	s.Store.Update(func(d *Doc) error {
		AddVersion(d, Source{SHA256: old, Frames: 10, FPS: 30})
		AddVersion(d, Source{SHA256: old, Frames: 10, FPS: 30})
		return nil
	})
	d, _ := s.Store.Load()
	if len(d.Versions) != 1 {
		t.Fatalf("a version is recorded once: %d", len(d.Versions))
	}
	if w := do(t, h, "GET", "/video/"+old, nil, nil); w.Code != 200 || w.Body.String() != "old proxy" {
		t.Fatalf("recorded version: %d %q", w.Code, w.Body.String())
	}
	// The page asks for the version /api/state named; a newer current render must not answer.
	if w := do(t, h, "GET", "/video?v="+old[:12], nil, nil); w.Code != 200 || w.Body.String() != "old proxy" {
		t.Fatalf("/video?v=<recorded version>: %d %q", w.Code, w.Body.String())
	}
	if w := do(t, h, "GET", "/video?v=abcdef012345", nil, nil); w.Code != 200 || w.Body.String() != "not really a video" {
		t.Fatalf("/video?v=<current version>: %d %q", w.Code, w.Body.String())
	}
	if w := do(t, h, "GET", "/video?v=bbbbbbbbbbbb", nil, nil); w.Code != 404 {
		t.Fatalf("/video?v=<unknown version>: %d, want 404", w.Code)
	}
	for _, bad := range []string{strings.Repeat("b", 64), "..%2F..%2Fetc", "abc"} {
		if w := do(t, h, "GET", "/video/"+bad, nil, nil); w.Code != 404 {
			t.Fatalf("%s: got %d, want 404", bad, w.Code)
		}
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	wav := filepath.Join(t.TempDir(), "tone.wav")
	if err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-af", "volume=0.5", wav).Run(); err != nil {
		t.Fatal(err)
	}
	peaks, secs, err := Peaks(context.Background(), wav, 100)
	if err != nil || len(peaks) != 100 || secs < 0.99 || secs > 1.01 {
		t.Fatalf("peaks %d, %.2fs, %v", len(peaks), secs, err)
	}
	// ffmpeg's sine source plays at 1/8 of full scale; at half volume it peaks at 0.0625.
	if peaks[50] < 0.055 || peaks[50] > 0.07 {
		t.Fatalf("peak %v, want about 0.0625", peaks[50])
	}
}

func TestPreviewProxyIsSmallAndKeepsEveryFrame(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "wide.mp4")
	if err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=1920x1080:rate=25:duration=1",
		"-pix_fmt", "yuv420p", video).Run(); err != nil {
		t.Skipf("ffmpeg cannot make a test video: %v", err)
	}
	ctx := context.Background()
	p, err := PreviewProxy(ctx, video, "abc", filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "abc.mp4" {
		t.Errorf("preview at %s, want <sha>.mp4 so earlier versions resolve", p)
	}
	src, err := Probe(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if src.Width != PreviewWidth || src.Height != 360 || src.Frames != 25 {
		t.Errorf("preview is %dx%d with %d frames, want %dx360 with 25", src.Width, src.Height, src.Frames, PreviewWidth)
	}
}

// A note is recorded against the render the page showed, even when a re-render went live
// between the page's last look at /api/state and the note (issue #26).
func TestNoteKeepsTheVersionThePageShowed(t *testing.T) {
	s, h := newTestServer(t)
	s.Store.Update(func(d *Doc) error { AddVersion(d, s.Source); return nil })
	newer := Source{Render: s.Store.Video, SHA256: "ffff00001111222233", FPS: 25, Frames: 50, Width: 64, Height: 36}
	s.mu.Lock()
	s.Source = newer
	s.mu.Unlock()
	s.Store.Update(func(d *Doc) error { AddVersion(d, newer); return nil })

	// Frame 100 is past the end of the new render, but inside the 120 frames the page showed.
	w := do(t, h, "POST", "/api/comments", map[string]any{"frame": 100, "text": "on the old one", "version": "abcdef012345"}, nil)
	if w.Code != 200 {
		t.Fatalf("add on the shown render: %d %s", w.Code, w.Body)
	}
	var c Comment
	json.Unmarshal(w.Body.Bytes(), &c)
	if c.Version != "abcdef012345" || c.Timecode != "00:00:03:10" {
		t.Errorf("the note should be on the shown render at 30 fps: %+v", c)
	}

	w = do(t, h, "POST", "/api/comments", map[string]any{"frame": 10, "text": "no version"}, nil)
	json.Unmarshal(w.Body.Bytes(), &c)
	if w.Code != 200 || c.Version != "ffff00001111" {
		t.Errorf("a note without a version should be on the current render: %d %+v", w.Code, c)
	}

	if w := do(t, h, "POST", "/api/comments", map[string]any{"frame": 1, "text": "x", "version": "000000000000"}, nil); w.Code != 409 {
		t.Errorf("a version the review never saw: %d, want 409", w.Code)
	}
}

func TestSnapshotIsFoundByNameInTheReviewFolder(t *testing.T) {
	s, h := newTestServer(t)
	os.MkdirAll(s.Store.SnapDir, 0o755)
	os.WriteFile(filepath.Join(s.Store.SnapDir, "c_01_f3.png"), []byte("\x89PNGsnap"), 0o644)
	outside := filepath.Join(t.TempDir(), "c_02_f5.png")
	os.WriteFile(outside, []byte("\x89PNGsecret"), 0o644)
	s.Store.Update(func(d *Doc) error {
		d.Comments = append(d.Comments,
			// Recorded relative to the folder another `cav review` ran in.
			Comment{ID: "c_01", Frame: 3, Status: "open", Snapshot: "renders/review/v3/c_01_f3.png"},
			// Names a file outside the review's folder.
			Comment{ID: "c_02", Frame: 5, Status: "open", Snapshot: outside})
		return nil
	})
	if w := do(t, h, "GET", "/api/snapshot/c_01", nil, nil); w.Code != 200 || w.Body.String() != "\x89PNGsnap" {
		t.Errorf("a relative snapshot path should be found in the review folder: %d %q", w.Code, w.Body.String())
	}
	if w := do(t, h, "GET", "/api/snapshot/c_02", nil, nil); w.Code != 404 {
		t.Errorf("a snapshot outside the review folder must not be served: %d", w.Code)
	}
}
