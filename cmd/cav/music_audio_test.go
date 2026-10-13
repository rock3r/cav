package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/listen"
)

func needFFmpeg(t *testing.T) {
	t.Helper()
	for _, b := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skip(b + " is not installed")
		}
	}
}

// tone writes a sine wave of the given length to path. ffmpeg's sine peaks at -18 dBFS;
// gain is added to that. WAV is written as float, so peaks above 0 dBFS stay.
func tone(t *testing.T, path string, seconds float64, gain string) {
	t.Helper()
	args := []string{"-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=" + strconv.FormatFloat(seconds, 'f', -1, 64),
		"-af", "volume=" + gain}
	if filepath.Ext(path) == ".wav" {
		args = append(args, "-c:a", "pcm_f32le")
	}
	args = append(args, path)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, b)
	}
}

func TestPrepareRefForStableAudio(t *testing.T) {
	needFFmpeg(t)
	ctx, dir := context.Background(), t.TempDir()
	short := filepath.Join(dir, "short.wav")
	tone(t, short, 3, "-6dB")
	if _, _, err := prepareRef(ctx, "stability", short, dir); err == nil || !strings.Contains(err.Error(), "6 to 190 s") {
		t.Fatalf("a 3 s reference must be refused: %v", err)
	}
	if got, note, err := prepareRef(ctx, "acestep", short, dir); err != nil || got != short || note != "" {
		t.Fatalf("ACE-Step gets the file as it is: %q %q %v", got, note, err)
	}
	m4a := filepath.Join(dir, "pixel.m4a")
	tone(t, m4a, 8, "-6dB")
	got, note, err := prepareRef(ctx, "stability", m4a, dir)
	if err != nil || filepath.Ext(got) != ".wav" || !strings.Contains(note, "WAV") {
		t.Fatalf("an m4a goes up as WAV: %q %q %v", got, note, err)
	}
	big := filepath.Join(dir, "big.wav") // 150 s of 96 kHz float 5.1: about 345 MB
	if b, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=96000:duration=150",
		"-ac", "6", "-c:a", "pcm_f32le", big).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, b)
	}
	got, note, err = prepareRef(ctx, "stability", big, dir)
	if st, _ := os.Stat(got); err != nil || !strings.Contains(note, "50 MB") || st == nil || st.Size() > stabilityRefBytes {
		t.Fatalf("a WAV over the request limit is made smaller: %q %q %v", got, note, err)
	}
	long := filepath.Join(dir, "long.mp3")
	tone(t, long, 200, "-6dB")
	got, note, err = prepareRef(ctx, "stability", long, dir)
	if err != nil || !strings.Contains(note, "first 190 s") {
		t.Fatalf("a long reference is cut: %q %v", note, err)
	}
	if d, _ := audioSeconds(ctx, got); d > 190.1 {
		t.Fatalf("sent %.1f s", d)
	}
}

func TestAddHeadroomLowersOnlyHotTakes(t *testing.T) {
	needFFmpeg(t)
	ctx, dir := context.Background(), t.TempDir()
	quiet := filepath.Join(dir, "quiet.wav")
	tone(t, quiet, 2, "-6dB")
	if gain, err := addHeadroom(ctx, quiet); err != nil || gain != 0 {
		t.Fatalf("a take with headroom stays as it is: %.2f %v", gain, err)
	}
	for _, name := range []string{"hot.wav", "hot.mp3"} {
		hot := filepath.Join(dir, name)
		tone(t, hot, 2, "20dB") // +2 dBFS
		gain, err := addHeadroom(ctx, hot)
		if err != nil || gain >= 0 {
			t.Fatalf("%s: a clipping take is lowered: %.2f %v", name, gain, err)
		}
		l, err := listen.Measure(ctx, hot)
		if err != nil || l.TruePeak > headroomPeak {
			t.Fatalf("%s: true peak after: %.2f %v", name, l.TruePeak, err)
		}
	}
}

func TestCutAndFadeEndsTheTakeOnTime(t *testing.T) {
	needFFmpeg(t)
	ctx, dir := context.Background(), t.TempDir()
	take := filepath.Join(dir, "take.wav")
	tone(t, take, 30, "0dB")
	if err := cutAndFade(ctx, take, 24, takeFade); err != nil {
		t.Fatal(err)
	}
	if d, err := audioSeconds(ctx, take); err != nil || d < 23.99 || d > 24.01 {
		t.Fatalf("the take lasts %.3f s: %v", d, err)
	}
	// peak is the loudest sample from `from` to one second later, in dBFS.
	peak := func(from float64) float64 {
		t.Helper()
		out, err := exec.Command("ffmpeg", "-v", "info", "-ss", strconv.FormatFloat(from, 'f', 2, 64), "-t", "1", "-i", take,
			"-af", "volumedetect", "-f", "null", "-").CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg: %v: %s", err, out)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if _, v, ok := strings.Cut(line, "max_volume: "); ok {
				if f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), " dB"), 64); err == nil {
					return f
				}
			}
		}
		t.Fatalf("no max_volume in %s", out)
		return 0
	}
	// The tone peaks at -18 dBFS until the fade starts, 2 s before the end. In the last
	// second it is at most half as loud (about -24 dBFS); a cut with no fade stays at -18.
	if p := peak(20); p < -18.5 {
		t.Errorf("before the fade the peak is %.1f dBFS", p)
	}
	if p := peak(23); p > -23 {
		t.Errorf("in the last second the peak is %.1f dBFS: no fade", p)
	}
}

func TestRefNamesAreDistinct(t *testing.T) {
	got := refNames([]string{"a/pixel.wav", "b/pixel.mp3", "c/Pixel!.wav", "bell.wav"})
	want := []string{"pixel", "pixel-2", "pixel-3", "bell"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestPrepareRefKeepsSameNamedReferencesApart(t *testing.T) {
	needFFmpeg(t)
	ctx, dir := context.Background(), t.TempDir()
	a, b := filepath.Join(dir, "a", "pixel.m4a"), filepath.Join(dir, "b", "pixel.m4a")
	for _, p := range []string{a, b} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		tone(t, p, 7, "-6dB")
	}
	ga, _, err1 := prepareRef(ctx, "stability", a, dir)
	gb, _, err2 := prepareRef(ctx, "stability", b, dir)
	if err1 != nil || err2 != nil || ga == gb {
		t.Fatalf("two references with one name must not share a file: %q %q %v %v", ga, gb, err1, err2)
	}
}
