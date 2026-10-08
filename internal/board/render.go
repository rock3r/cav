package board

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rock3r/cav/internal/sheet"
)

// Sheet tiles every shot's frame, labelled with its beats and times, into one PNG.
func (sb *Board) Sheet(ctx context.Context, out string, cols int) error {
	tmp, err := os.MkdirTemp("", "cav-board")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var tiles []sheet.Tile
	for _, s := range sb.Shots {
		if s.Frame == "" {
			return fmt.Errorf("%s has no frame yet: run cav board frames", s.ID)
		}
		p := filepath.Join(tmp, s.ID+".png")
		if err := Normalize(ctx, sb.Path(s.Frame), p, 640, 360); err != nil {
			return err
		}
		label := sb.Label(s) + "  " + s.What
		if len(label) > 52 { // a 640 px tile fits 53 characters at label scale 2
			label = label[:49] + "..."
		}
		tiles = append(tiles, sheet.Tile{Path: p, Label: label})
	}
	if cols == 0 {
		cols = 3
	}
	_, _, err = sheet.Build(tiles, out, sheet.Options{Cols: cols, Gap: 10, Scale: 2})
	return err
}

// Animatic holds each frame for its shot's length, cuts on the shot boundaries and adds
// the music when given. Captions name the shot, bar and time. A shot with a clip plays the
// clip instead, cut to the shot's length.
func (sb *Board) Animatic(ctx context.Context, audio, out string, captions bool) (float64, error) {
	tmp, err := os.MkdirTemp("", "cav-animatic")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)
	for _, s := range sb.Shots {
		if s.Clip != "" {
			return sb.animaticWithClips(ctx, tmp, audio, out, captions)
		}
	}
	var list strings.Builder
	var total float64
	last := ""
	// Frames before the first shot and gaps between shots show black.
	black := filepath.Join(tmp, "black.png")
	if err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i",
		fmt.Sprintf("color=c=0x000000:s=%dx%d", sb.Width, sb.Height), "-frames:v", "1", black).Run(); err != nil {
		return 0, fmt.Errorf("ffmpeg: %w", err)
	}
	add := func(path string, d float64) {
		if d <= 0 {
			return
		}
		fmt.Fprintf(&list, "file '%s'\nduration %.6f\n", strings.ReplaceAll(path, "'", `'\''`), d)
		total += d
		last = path
	}
	cursor := 0.0
	for _, s := range sb.Shots {
		if s.Frame == "" {
			return 0, fmt.Errorf("%s has no frame yet: run cav board frames", s.ID)
		}
		start, end := sb.Time(s.Beats[0]), sb.Time(s.Beats[1])
		add(black, start-cursor)
		p := filepath.Join(tmp, s.ID+".png")
		if err := Normalize(ctx, sb.Path(s.Frame), p, sb.Width, sb.Height); err != nil {
			return 0, err
		}
		if captions {
			bar := int(s.Beats[0])/4 + 1
			if err := Caption(p, fmt.Sprintf("%s  bar %d  %.2fs  %s", s.ID, bar, start, s.What)); err != nil {
				return 0, err
			}
		}
		add(p, end-start)
		cursor = end
	}
	if sb.Seconds > cursor {
		add(black, sb.Seconds-cursor)
	}
	// The concat demuxer needs the last file again, without a duration.
	fmt.Fprintf(&list, "file '%s'\n", strings.ReplaceAll(last, "'", `'\''`))
	listPath := filepath.Join(tmp, "list.txt")
	if err := os.WriteFile(listPath, []byte(list.String()), 0o644); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, err
	}
	args := []string{"-v", "error", "-y", "-f", "concat", "-safe", "0", "-i", listPath}
	if audio != "" {
		args = append(args, "-i", audio)
	}
	args = append(args, "-vf", fmt.Sprintf("fps=%g,format=yuv420p", sb.FPS), "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-t", fmt.Sprintf("%.6f", total))
	if audio != "" {
		args = append(args, "-map", "0:v:0", "-map", "1:a:0", "-c:a", "aac", "-b:a", "192k")
	}
	args = append(args, "-movflags", "+faststart", out)
	if b, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return total, nil
}

// animaticWithClips builds the animatic in one ffmpeg filter graph: each segment (black gap,
// held frame or clip) is scaled to the board size, cut or held to its length, and the
// segments are joined in order. Clip audio is dropped; the music, when given, is the only
// sound.
func (sb *Board) animaticWithClips(ctx context.Context, tmp, audio, out string, captions bool) (float64, error) {
	var args, chains []string
	n, total := 0, 0.0
	fit := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=0x16181d,setsar=1,fps=%g", sb.Width, sb.Height, sb.Width, sb.Height, sb.FPS)
	segment := func(d float64, input []string, overlay string) {
		if d <= 0 {
			return
		}
		// Count frames from the start of the piece, so rounding never drifts the cuts.
		frames := int(math.Round((total+d)*sb.FPS)) - int(math.Round(total*sb.FPS))
		args = append(args, input...)
		v := n
		n++
		chain := fmt.Sprintf("[%d:v]%s", v, fit)
		if overlay != "" {
			// The caption layer is a looped still: repeat its last frame so it never ends the
			// shot early.
			args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.6f", d+1), "-i", overlay)
			chain += fmt.Sprintf("[b%d];[b%d][%d:v]overlay=0:0:eof_action=repeat", v, v, n)
			n++
		}
		chain += fmt.Sprintf(",tpad=stop_mode=clone:stop_duration=%.6f,trim=end_frame=%d,setpts=PTS-STARTPTS", d+1, frames)
		chains = append(chains, chain+fmt.Sprintf(",format=yuv420p[s%d]", len(chains)))
		total += d
	}
	black := func(d float64) {
		segment(d, []string{"-f", "lavfi", "-t", fmt.Sprintf("%.6f", d), "-i", fmt.Sprintf("color=c=black:s=%dx%d:r=%g", sb.Width, sb.Height, sb.FPS)}, "")
	}
	cursor := 0.0
	for _, s := range sb.Shots {
		start, end := sb.Time(s.Beats[0]), sb.Time(s.Beats[1])
		black(start - cursor)
		d := end - start
		caption := ""
		if captions {
			caption = filepath.Join(tmp, s.ID+"-caption.png")
			bar := int(s.Beats[0])/4 + 1
			if err := CaptionLayer(caption, sb.Width, sb.Height, fmt.Sprintf("%s  bar %d  %.2fs  %s", s.ID, bar, start, s.What)); err != nil {
				return 0, err
			}
		}
		switch {
		case s.Clip != "":
			if _, err := os.Stat(sb.Path(s.Clip)); err != nil {
				return 0, fmt.Errorf("%s: clip %s: %w", s.ID, s.Clip, err)
			}
			segment(d, []string{"-i", sb.Path(s.Clip)}, caption)
		case s.Frame != "":
			p := filepath.Join(tmp, s.ID+".png")
			if err := Normalize(ctx, sb.Path(s.Frame), p, sb.Width, sb.Height); err != nil {
				return 0, err
			}
			segment(d, []string{"-loop", "1", "-t", fmt.Sprintf("%.6f", d), "-i", p}, caption)
		default:
			return 0, fmt.Errorf("%s has no frame yet: run cav board frames", s.ID)
		}
		cursor = end
	}
	if sb.Seconds > cursor {
		black(sb.Seconds - cursor)
	}
	var join strings.Builder
	for i := range chains {
		fmt.Fprintf(&join, "[s%d]", i)
	}
	// Restamp the joined frames at the board's rate: segments from different sources keep
	// uneven timestamps through concat, and -t would then drop the last frames.
	fmt.Fprintf(&join, "concat=n=%d:v=1:a=0,setpts=N/(%g*TB)[v]", len(chains), sb.FPS)
	graph := strings.Join(append(chains, join.String()), ";")
	if audio != "" {
		args = append(args, "-i", audio)
	}
	args = append(args, "-filter_complex", graph, "-map", "[v]")
	if audio != "" {
		args = append(args, "-map", fmt.Sprintf("%d:a:0", n), "-c:a", "aac", "-b:a", "192k")
	}
	args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-t", fmt.Sprintf("%.6f", total), "-movflags", "+faststart", out)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, err
	}
	args = append([]string{"-v", "error", "-y"}, args...)
	if b, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return total, nil
}

var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".bmp": true, ".tif": true, ".tiff": true}

// MoodTile is one image of a mood board and its caption.
type MoodTile struct {
	Path    string
	Caption string
}

// MoodImages lists the images in a folder, sorted by name.
func MoodImages(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && imageExt[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// Mood lays the images out as one collage PNG with a caption under each.
func Mood(ctx context.Context, tiles []MoodTile, out string, cols int) (int, int, error) {
	if len(tiles) == 0 {
		return 0, 0, fmt.Errorf("no images to lay out")
	}
	tmp, err := os.MkdirTemp("", "cav-mood")
	if err != nil {
		return 0, 0, err
	}
	defer os.RemoveAll(tmp)
	var st []sheet.Tile
	for i, t := range tiles {
		p := filepath.Join(tmp, fmt.Sprintf("%03d.png", i))
		if err := Normalize(ctx, t.Path, p, 480, 320); err != nil {
			return 0, 0, err
		}
		label := t.Caption
		if len(label) > 39 { // a 480 px tile fits 40 characters at label scale 2
			label = label[:36] + "..."
		}
		st = append(st, sheet.Tile{Path: p, Label: label})
	}
	if cols == 0 {
		cols = 4
		if len(tiles) <= 6 {
			cols = 3
		}
	}
	return sheet.Build(st, out, sheet.Options{Cols: cols, Gap: 8, Scale: 2})
}
