package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/moodboard"
	"github.com/rock3r/cav/internal/services"
)

// siteStub answers for YouTube and Vimeo without the network: every request of the shared
// client goes to one local server, which reads the host the code asked for.
type siteStub struct{ u *url.URL }

func (s siteStub) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("X-Asked-Host", r.URL.Host)
	r2.URL.Scheme, r2.URL.Host = s.u.Scheme, s.u.Host
	return http.DefaultTransport.RoundTrip(r2)
}

func stubSites(t *testing.T, up bool) *[]string {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Header.Get("X-Asked-Host")
		asked = append(asked, host+r.URL.Path)
		switch {
		case !up:
			w.WriteHeader(503)
		case host == "www.youtube.com" && r.URL.Path == "/oembed":
			w.Write([]byte(`{"title":"Launch reel","author_name":"Studio X","author_url":"https://www.youtube.com/@x","thumbnail_url":"https://i.ytimg.com/vi/aqz-KE-bpKQ/hqdefault.jpg"}`))
		case host == "i.ytimg.com":
			var b strings.Builder
			png.Encode(&sbWriter{&b}, solid(8, 8, color.RGBA{200, 80, 40, 255}))
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte(b.String()))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: siteStub{u}}
	t.Cleanup(func() { services.HTTPClient = old })
	return &asked
}

type sbWriter struct{ b *strings.Builder }

func (w *sbWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

func solid(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func writePNG(t *testing.T, path string, c color.Color) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, solid(64, 36, c)); err != nil {
		t.Fatal(err)
	}
}

func readMood(t *testing.T, path string) *moodboard.Board {
	t.Helper()
	b, err := moodboard.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRefAddVideoEmbedsWithoutDownloading(t *testing.T) {
	root := inNewProject(t)
	asked := stubSites(t, true)
	root = library.Root()
	var err error
	out, _ := captureStreams(t, func() {
		err = sourcesAdd(&app{json: true}, "image", root, "moodboard",
			[]string{"https://youtu.be/aqz-KE-bpKQ?si=x", "--take", "the pace of the cuts", "--by", "brief"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var res struct{ ID, Kind, Thumb string }
	if json.Unmarshal([]byte(out), &res) != nil || res.ID != "yt-aqz-KE-bpKQ" || res.Kind != "embed" {
		t.Fatalf("answer %s", out)
	}
	// Only the oEmbed address and the image host were asked. The video itself never was.
	if got := strings.Join(*asked, " "); got != "www.youtube.com/oembed i.ytimg.com/vi/aqz-KE-bpKQ/hqdefault.jpg" {
		t.Errorf("asked %s", got)
	}
	m, _ := library.Load(root)
	if len(m.Assets) != 1 {
		t.Fatalf("manifest %+v", m.Assets)
	}
	e := m.Assets[0]
	if e.Kind != "embed" || e.Licence != "reference-only" || e.Source != "youtube" || e.URL != "https://www.youtube.com/watch?v=aqz-KE-bpKQ" ||
		e.Path != "moodboard/.thumbs/yt-aqz-KE-bpKQ.jpg" || e.Author != "Studio X" {
		t.Errorf("entry %+v", e)
	}
	if _, err := os.Stat(e.Path); err != nil {
		t.Errorf("no thumbnail: %v", err)
	}
	p, _ := library.LoadProject(root)
	if probs := library.Check(root, m, p); len(probs) != 0 {
		t.Errorf("credits check should be clean: %+v", probs)
	}
	b := readMood(t, "moodboard.json")
	it := b.Find("yt-aqz-KE-bpKQ")
	if it == nil || it.Kind != "embed" || it.Take != "the pace of the cuts" || it.AddedBy != "brief" || it.Licence != "reference-only" ||
		it.Title != "Launch reel" || it.Thumb != "moodboard/.thumbs/yt-aqz-KE-bpKQ.jpg" {
		t.Fatalf("item %+v", it)
	}
	// Adding it again keeps one item.
	captureStreams(t, func() {
		err = sourcesAdd(&app{json: true}, "image", root, "moodboard", []string{"https://www.youtube.com/watch?v=aqz-KE-bpKQ"})
	})
	if err != nil || len(readMood(t, "moodboard.json").Items) != 1 {
		t.Fatalf("twice: %v", err)
	}
	if got := readMood(t, "moodboard.json").Items[0]; got.Take != "the pace of the cuts" || got.AddedBy != "brief" {
		t.Errorf("adding it again lost the take: %+v", got)
	}
}

func TestRefAddRefusesOtherLinksAndWorksOffline(t *testing.T) {
	inNewProject(t)
	root := library.Root()
	asked := stubSites(t, false)
	err := sourcesAdd(&app{json: true}, "image", root, "moodboard", []string{"https://example.com/video.mp4", "--licence", "CC0-1.0"})
	if err == nil || !strings.Contains(err.Error(), "YouTube or Vimeo") || len(*asked) != 0 {
		t.Fatalf("another link: %v, asked %v", err, *asked)
	}
	if err := sourcesAdd(&app{json: true}, "audio", root, "sfx", []string{"https://youtu.be/aqz-KE-bpKQ"}); err == nil || len(*asked) != 0 {
		t.Fatalf("sfx add must not take a link: %v", err)
	}
	// The site is down: the video is still added, without a thumbnail.
	var e error
	_, errOut := captureStreams(t, func() {
		e = sourcesAdd(&app{json: true}, "image", root, "moodboard", []string{"https://vimeo.com/123456789", "--title", "Film by hand"})
	})
	if e != nil {
		t.Fatal(e)
	}
	b := readMood(t, "moodboard.json")
	it := b.Find("vimeo-123456789")
	if it == nil || it.Kind != "embed" || it.Thumb != "" || it.Title != "Film by hand" || it.Licence != "reference-only" {
		t.Fatalf("item %+v", it)
	}
	_ = errOut
	if m, _ := library.Load(root); len(m.Assets) != 0 {
		t.Errorf("no thumbnail, so nothing to record: %+v", m.Assets)
	}
	// A later sync keeps it.
	if b2, ch, err := moodboard.Sync("moodboard.json", "moodboard", nil, root); err != nil || b2.Find("vimeo-123456789") == nil || len(ch.Missing) != 0 {
		t.Fatalf("sync: %v %+v", err, ch)
	}
}

func TestBoardMoodTakeAndDrop(t *testing.T) {
	inNewProject(t)
	writePNG(t, "moodboard/Neon.png", color.RGBA{255, 0, 120, 255})
	writePNG(t, "moodboard/dusk.png", color.RGBA{40, 20, 90, 255})
	a := &app{json: true}

	err := boardMood(a, []string{"take", "nope", "the glow"})
	if err == nil || !strings.Contains(err.Error(), "neon, dusk") && !strings.Contains(err.Error(), "dusk, neon") {
		t.Fatalf("unknown id should list the ids: %v", err)
	}
	if err := boardMood(a, []string{"drop", "nope"}); err == nil {
		t.Fatal("dropping an unknown id should fail")
	}
	if err := boardMood(a, []string{"take", "neon"}); err == nil {
		t.Fatal("take needs words")
	}
	captureStreams(t, func() { err = boardMood(a, []string{"take", "neon", "the", "hot pink glow"}) })
	if err != nil {
		t.Fatal(err)
	}
	captureStreams(t, func() { err = boardMood(a, []string{"drop", "dusk"}) })
	if err != nil {
		t.Fatal(err)
	}
	b := readMood(t, "moodboard.json")
	if b.Find("neon").Take != "the hot pink glow" || b.Find("dusk").Status != moodboard.StatusDropped {
		t.Fatalf("%+v", b.Items)
	}
}

func TestBoardMoodWritesThePictureAndKeepsTakesAndDrops(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	inNewProject(t)
	writePNG(t, "moodboard/neon.png", color.RGBA{255, 0, 120, 255})
	writePNG(t, "moodboard/dusk.png", color.RGBA{40, 20, 90, 255})
	writePNG(t, "moodboard/sand.png", color.RGBA{220, 200, 140, 255})
	clip := "moodboard/reel.mp4"
	if b, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=10", "-t", "2", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot make a clip: %v %s", err, b)
	}
	a := &app{json: true}
	var err error
	out, _ := captureStreams(t, func() { err = boardMood(a, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		File   string
		Items  int
		Counts map[string]int
		Added  []string
		NoTake []string
	}
	if json.Unmarshal([]byte(out), &res) != nil || res.Items != 4 || res.Counts["image"] != 3 || res.Counts["clip"] != 1 ||
		len(res.Added) != 4 || len(res.NoTake) != 4 {
		t.Fatalf("answer %s", out)
	}
	for _, f := range []string{res.File, "moodboard/.thumbs/reel.png", "moodboard.json"} {
		if st, err := os.Stat(f); err != nil || st.Size() == 0 {
			t.Errorf("%s: %v", f, err)
		}
	}
	b := readMood(t, "moodboard.json")
	if it := b.Find("reel"); it.Kind != "clip" || it.Seconds < 1.9 || it.Seconds > 2.1 {
		t.Errorf("clip %+v", it)
	}

	captureStreams(t, func() {
		err = boardMood(a, []string{"take", "reel", "the cut rhythm"})
		if err == nil {
			err = boardMood(a, []string{"drop", "sand"})
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	// Run it again: the take and the drop stay, and the dropped file stays out of the picture.
	out, _ = captureStreams(t, func() { err = boardMood(a, nil) })
	if err != nil {
		t.Fatal(err)
	}
	res.Added, res.NoTake = nil, nil
	if json.Unmarshal([]byte(out), &res) != nil || res.Items != 3 || len(res.Added) != 0 || len(res.NoTake) != 2 {
		t.Fatalf("second answer %s", out)
	}
	b = readMood(t, "moodboard.json")
	if b.Find("reel").Take != "the cut rhythm" || b.Find("sand").Status != moodboard.StatusDropped {
		t.Fatalf("%+v", b.Items)
	}
}
