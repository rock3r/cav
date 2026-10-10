package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/listen"
)

// Stable Audio's audio-to-audio takes MP3 or WAV, 6 to 190 seconds long (Stability's
// OpenAPI spec, read on 2026-10-09).
const stabilityRefMin, stabilityRefMax = 6.0, 190.0

// prepareRef makes a reference track fit the service. For Stable Audio, cav converts other
// formats (m4a, flac, aiff...) to WAV and keeps the first 190 seconds of a longer track; a
// track shorter than 6 seconds is an error. Other services get the file as it is. It
// returns the file to send and a note when cav changed it.
func prepareRef(ctx context.Context, service, path, tmp string) (string, string, error) {
	if service != "stability" {
		return path, "", nil
	}
	dur, err := audioSeconds(ctx, path)
	if err != nil {
		return path, "", nil // without ffprobe, send it as it is; the service says what is wrong
	}
	if dur < stabilityRefMin {
		return "", "", fmt.Errorf("%s is %.1f s long; Stable Audio needs a reference of %g to %g s", path, dur, stabilityRefMin, stabilityRefMax)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if dur <= stabilityRefMax && (ext == ".mp3" || ext == ".wav") {
		return path, "", nil
	}
	out := filepath.Join(tmp, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".wav")
	args := []string{"-v", "error", "-y", "-i", path, "-vn"}
	note := "converted to WAV for Stable Audio"
	if dur > stabilityRefMax {
		args = append(args, "-t", strconv.FormatFloat(stabilityRefMax, 'f', 0, 64))
		note = fmt.Sprintf("Stable Audio takes at most %g s, so cav sent the first %g s", stabilityRefMax, stabilityRefMax)
	}
	if b, err := exec.CommandContext(ctx, "ffmpeg", append(args, "-c:a", "pcm_s16le", out)...).CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("converting %s: %v: %s", path, err, strings.TrimSpace(string(b)))
	}
	return out, note, nil
}

func audioSeconds(ctx context.Context, path string) (float64, error) {
	b, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
}

// Takes from every music service came back with true peaks above 0 dBFS in tests on
// 2026-10-09, so they can clip once encoded again. cav lowers the gain of such a take to
// headroomTarget (below the -1 dBTP that cav listen asks for, to leave room for the MP3
// encoder). It changes the level only: no compression or limiting.
const headroomPeak, headroomTarget = -1.0, -1.5

// addHeadroom lowers the gain of a take whose true peak is above -1 dBTP and returns the
// change in dB (0 when the take was fine). It needs ffmpeg.
func addHeadroom(ctx context.Context, path string) (float64, error) {
	l, err := listen.Measure(ctx, path)
	if err != nil {
		return 0, err
	}
	if l.TruePeak <= headroomPeak {
		return 0, nil
	}
	gain := headroomTarget - l.TruePeak
	ext := strings.ToLower(filepath.Ext(path))
	codec := []string{"-c:a", "libmp3lame", "-b:a", "320k"}
	if ext == ".wav" {
		codec = []string{"-c:a", "pcm_s24le"}
	}
	tmp := strings.TrimSuffix(path, filepath.Ext(path)) + ".level" + ext
	args := append([]string{"-v", "error", "-y", "-i", path, "-vn", "-af", fmt.Sprintf("volume=%.2fdB", gain)}, codec...)
	if b, err := exec.CommandContext(ctx, "ffmpeg", append(args, tmp)...).CombinedOutput(); err != nil {
		os.Remove(tmp)
		return 0, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return gain, os.Rename(tmp, path)
}
