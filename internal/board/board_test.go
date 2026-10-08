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

func TestPlacements(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	f, _ := os.Create(filepath.Join(dir, "s1.png"))
	png.Encode(f, img)
	f.Close()
	os.WriteFile(filepath.Join(dir, "sb.json"), []byte(`{"bpm":120,"offset":0.25,"shots":[
		{"id":"s1","beats":[0,4],"frame":"s1.png"},{"id":"s2","beats":[4,8]}]}`), 0o644)
	sb, err := Load(filepath.Join(dir, "sb.json"))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := sb.Placements(context.Background(), map[string]bool{"s1": true})
	if err != nil {
		t.Fatal(err)
	}
	// Times include the offset; the native job turns them into frames.
	if len(ps) != 1 || ps[0].Start != 0.25 || ps[0].End != 2.25 || ps[0].Width != 640 || ps[0].Height != 360 || !filepath.IsAbs(ps[0].Path) {
		t.Fatalf("placements: %+v", ps)
	}
	if _, err := sb.Placements(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "s2 has no frame") {
		t.Errorf("a shot without a frame should fail: %v", err)
	}
	if _, err := sb.Placements(context.Background(), map[string]bool{"s9": true}); err == nil {
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
