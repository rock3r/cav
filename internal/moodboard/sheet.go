package moodboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/sheet"
)

// Duration reads the length of a clip in seconds with ffprobe.
func Duration(ctx context.Context, clip string) (float64, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", clip).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", clip, err)
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s has no length", clip)
	}
	return d, nil
}

// Fresh says whether out exists and is newer than in.
func Fresh(in, out string) bool {
	a, err := os.Stat(in)
	if err != nil {
		return false
	}
	b, err := os.Stat(out)
	return err == nil && !b.ModTime().Before(a.ModTime())
}

// ContactSheet writes a PNG of n frames of a clip, taken at even times and labelled with
// their time, so the agent can see the clip. It returns the clip's length. The sheet is made
// again only when it is missing or older than the clip.
func ContactSheet(ctx context.Context, clip, out string, n int) (float64, error) {
	seconds, err := Duration(ctx, clip)
	if err != nil {
		return 0, err
	}
	if Fresh(clip, out) {
		return seconds, nil
	}
	if n < 1 {
		n = 6
	}
	tmp, err := os.MkdirTemp("", "cav-clip")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)
	const w, h = 320, 180
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=0x16181d,format=rgba", w, h, w, h)
	var tiles []sheet.Tile
	for i := 0; i < n; i++ {
		t := (float64(i) + 0.5) * seconds / float64(n)
		// A seek to the very end finds no frame in a short clip: stay a little before it.
		if last := seconds - 0.25; t > last && last > 0 {
			t = last
		}
		p := filepath.Join(tmp, fmt.Sprintf("%02d.png", i))
		b, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-ss", fmt.Sprintf("%.3f", t), "-i", clip,
			"-vf", vf, "-frames:v", "1", p).CombinedOutput()
		if err != nil {
			return 0, fmt.Errorf("ffmpeg could not read %s: %v: %s", clip, err, strings.TrimSpace(string(b)))
		}
		tiles = append(tiles, sheet.Tile{Path: p, Label: fmt.Sprintf("%.1fs", t)})
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, err
	}
	cols := 3
	if n < 3 {
		cols = n
	}
	if _, _, err := sheet.Build(tiles, out, sheet.Options{Cols: cols, Gap: 6, Scale: 2}); err != nil {
		return 0, err
	}
	return seconds, nil
}
