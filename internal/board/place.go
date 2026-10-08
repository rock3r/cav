package board

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Placement is one shot's frame as an image layer in a Cavalry composition.
type Placement struct {
	ID    string  `json:"id"`
	Path  string  `json:"path"`  // absolute path of the frame image
	In    int     `json:"in"`    // first visible frame
	Out   int     `json:"out"`   // last visible frame; api.setOutFrame takes it inclusive
	Scale float64 `json:"scale"` // fits the image inside the comp; 0 when the image size is unknown
}

// ShotFrames returns the first and last visible frame of a shot at fps. The last frame is
// one before the frame where the shot's end beat falls, so a shot that ends where the next
// starts hands off without overlap or gap.
func (sb *Board) ShotFrames(s Shot, fps float64) (in, out int) {
	in = max(0, int(math.Round(sb.Time(s.Beats[0])*fps)))
	out = max(in, int(math.Round(sb.Time(s.Beats[1])*fps))-1)
	return in, out
}

// FitScale is the uniform scale that fits an iw x ih image inside a cw x ch comp,
// letterboxed like the animatic.
func FitScale(iw, ih, cw, ch int) float64 {
	if iw <= 0 || ih <= 0 || cw <= 0 || ch <= 0 {
		return 0
	}
	return math.Min(float64(cw)/float64(iw), float64(ch)/float64(ih))
}

// Placements lists the frames to place in a cw x ch comp at fps whose first frame is
// start; beat 0 lands on start. With only set, it keeps just those shot ids. Every listed
// shot needs a frame image on disk.
func (sb *Board) Placements(ctx context.Context, fps float64, start, cw, ch int, only map[string]bool) ([]Placement, error) {
	if fps <= 0 {
		return nil, fmt.Errorf("the composition frame rate must be positive (got %g)", fps)
	}
	var out []Placement
	for _, s := range sb.Shots {
		if len(only) > 0 && !only[s.ID] {
			continue
		}
		if s.Frame == "" {
			return nil, fmt.Errorf("%s has no frame yet: run cav board frames", s.ID)
		}
		p, err := filepath.Abs(sb.Path(s.Frame))
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("%s: frame %s is missing: run cav board frames", s.ID, s.Frame)
		}
		iw, ih := imageSize(ctx, p)
		in, last := sb.ShotFrames(s, fps)
		out = append(out, Placement{ID: s.ID, Path: p, In: start + in, Out: start + last, Scale: FitScale(iw, ih, cw, ch)})
	}
	for id := range only {
		found := false
		for _, s := range sb.Shots {
			found = found || s.ID == id
		}
		if !found {
			return nil, fmt.Errorf("no shot called %s", id)
		}
	}
	return out, nil
}

// imageSize reads an image's pixel size, with ffprobe for formats Go cannot decode (WebP).
// It returns 0, 0 when the size is unknown, for example for SVG.
func imageSize(ctx context.Context, path string) (int, int) {
	if f, err := os.Open(path); err == nil {
		c, _, err := image.DecodeConfig(f)
		f.Close()
		if err == nil {
			return c.Width, c.Height
		}
	}
	b, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path).Output()
	if err != nil {
		return 0, 0
	}
	w, h, ok := strings.Cut(strings.TrimSpace(string(b)), "x")
	if !ok {
		return 0, 0
	}
	iw, _ := strconv.Atoi(w)
	ih, _ := strconv.Atoi(h)
	return iw, ih
}
