package board

import (
	"context"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimeFromBPMAndFromGrid(t *testing.T) {
	sb := &Board{BPM: 120, Offset: 0.25, Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 4}}}}
	if got := sb.Time(4); got != 2.25 {
		t.Fatalf("bpm time %g", got)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "grid.json"), []byte(`{"beats":[0.1,0.6,1.0,1.6]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "sb.json"), []byte(`{"grid":"grid.json","shots":[{"id":"s1","beats":[0,2]},{"id":"s2","beats":[2,5]}]}`), 0o644)
	sb, err := Load(filepath.Join(dir, "sb.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sb.Time(1.5); math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("between grid beats: %g", got)
	}
	// Past the last detected beat, the last spacing continues.
	if got := sb.Time(5); math.Abs(got-2.8) > 1e-9 {
		t.Fatalf("after the grid: %g", got)
	}
}

func TestValidate(t *testing.T) {
	bad := []*Board{
		{BPM: 120},
		{BPM: 120, Shots: []Shot{{ID: "s1", Beats: [2]float64{4, 4}}}},
		{BPM: 120, Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 8}}, {ID: "s2", Beats: [2]float64{6, 10}}}},
		{BPM: 120, Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 4}}, {ID: "s1", Beats: [2]float64{4, 8}}}},
		{Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 4}}}},
	}
	for i, b := range bad {
		if b.Validate() == nil {
			t.Errorf("board %d should not validate", i)
		}
	}
}

func TestShotFramesHandOffWithoutOverlap(t *testing.T) {
	// 120 bpm: a beat is 0.5 s, so 30 frames at 60 fps.
	sb := &Board{BPM: 120, Shots: []Shot{{ID: "s1", Beats: [2]float64{0, 4}}, {ID: "s2", Beats: [2]float64{4, 8}}, {ID: "s3", Beats: [2]float64{10, 12}}}}
	want := [][2]int{{0, 119}, {120, 239}, {300, 359}}
	for i, s := range sb.Shots {
		in, out := sb.ShotFrames(s, 60)
		if in != want[i][0] || out != want[i][1] {
			t.Errorf("%s: frames %d-%d, want %d-%d", s.ID, in, out, want[i][0], want[i][1])
		}
	}
	// An offset moves every shot; odd frame rates round to the nearest frame.
	sb.Offset = 0.25
	if in, out := sb.ShotFrames(sb.Shots[1], 25); in != 56 || out != 105 {
		t.Errorf("offset at 25 fps: %d-%d, want 56-105", in, out)
	}
	// A shot shorter than a frame still shows for one frame.
	short := Shot{ID: "x", Beats: [2]float64{0, 0.01}}
	if in, out := sb.ShotFrames(short, 24); out != in {
		t.Errorf("tiny shot: %d-%d", in, out)
	}
}

func TestFitScale(t *testing.T) {
	cases := []struct {
		iw, ih, cw, ch int
		want           float64
	}{
		{1280, 720, 1920, 1080, 1.5},
		{1920, 1080, 1080, 1920, 0.5625}, // landscape frame in a portrait comp: fit the width
		{1024, 1024, 1920, 1080, 1080.0 / 1024},
		{0, 0, 1920, 1080, 0},
	}
	for _, c := range cases {
		if got := FitScale(c.iw, c.ih, c.cw, c.ch); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("FitScale(%d,%d,%d,%d) = %g, want %g", c.iw, c.ih, c.cw, c.ch, got, c.want)
		}
	}
}

func TestPlacements(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	f, _ := os.Create(filepath.Join(dir, "s1.png"))
	png.Encode(f, img)
	f.Close()
	os.WriteFile(filepath.Join(dir, "sb.json"), []byte(`{"bpm":120,"shots":[
		{"id":"s1","beats":[0,4],"frame":"s1.png"},{"id":"s2","beats":[4,8]}]}`), 0o644)
	sb, err := Load(filepath.Join(dir, "sb.json"))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := sb.Placements(context.Background(), 30, 0, 1920, 1080, map[string]bool{"s1": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].In != 0 || ps[0].Out != 59 || ps[0].Scale != 3 || !filepath.IsAbs(ps[0].Path) {
		t.Fatalf("placements: %+v", ps)
	}
	// A composition that starts at frame 100 shifts every shot by 100 frames.
	if ps, err := sb.Placements(context.Background(), 30, 100, 1920, 1080, map[string]bool{"s1": true}); err != nil || ps[0].In != 100 || ps[0].Out != 159 {
		t.Errorf("start frame 100: %+v %v", ps, err)
	}
	if _, err := sb.Placements(context.Background(), 30, 0, 1920, 1080, nil); err == nil || !strings.Contains(err.Error(), "s2 has no frame") {
		t.Errorf("a shot without a frame should fail: %v", err)
	}
	if _, err := sb.Placements(context.Background(), 30, 0, 1920, 1080, map[string]bool{"s9": true}); err == nil {
		t.Error("an unknown shot id should fail")
	}
}

func TestAnimaticPlaysClipsCutToTheShot(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("needs ffmpeg")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if b, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg: %v: %s", err, b)
		}
	}
	// A red still, and a 4-second 24 fps clip with sound, like Veo's, for a 2-second shot
	// and a 6-second shot (the clip is cut, then held).
	run("-f", "lavfi", "-i", "color=c=red:s=320x180", "-frames:v", "1", filepath.Join(dir, "s1.png"))
	run("-f", "lavfi", "-i", "testsrc=s=640x360:r=24:d=4", "-f", "lavfi", "-i", "sine=d=4", "-shortest", "-c:v", "libx264", "-pix_fmt", "yuv420p", filepath.Join(dir, "s2.mp4"))
	sb := &Board{BPM: 120, FPS: 30, Seconds: 10, Width: 320, Height: 180, dir: dir, Shots: []Shot{
		{ID: "s1", Beats: [2]float64{1, 5}, What: "still", Frame: "s1.png"},
		{ID: "s2", Beats: [2]float64{5, 17}, What: "moving", Frame: "s1.png", Clip: "s2.mp4"},
	}}
	out := filepath.Join(dir, "animatic.mp4")
	secs, err := sb.Animatic(context.Background(), "", out, true)
	if err != nil {
		t.Fatal(err)
	}
	// 0.5 s black, s1 2 s, s2 6 s, then 1.5 s black to the piece's 10 s.
	if secs < 9.99 || secs > 10.01 {
		t.Fatalf("length %g", secs)
	}
	b, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,width,height,nb_read_frames", "-count_frames", "-of", "csv=p=0", out).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got != "video,320,180,300" {
		t.Fatalf("streams: %q (want one 320x180 video stream of 300 frames, no clip audio)", got)
	}
	// With music, the music is the only sound.
	run("-f", "lavfi", "-i", "sine=f=220:d=12", filepath.Join(dir, "music.wav"))
	if _, err := sb.Animatic(context.Background(), filepath.Join(dir, "music.wav"), out, false); err != nil {
		t.Fatal(err)
	}
	b, _ = exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", out).Output()
	if got := strings.Fields(string(b)); len(got) != 2 || got[0] != "video" || got[1] != "audio" {
		t.Fatalf("streams with music: %v", got)
	}
}
