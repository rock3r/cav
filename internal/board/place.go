package board

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Placement is one shot's frame, to become an image layer in a Cavalry composition. The
// frame numbers and the fit scale depend on the composition, so the native job works them
// out (assets/board/place.js).
type Placement struct {
	ID     string  `json:"id"`
	Path   string  `json:"path"`   // absolute path of the frame image
	Start  float64 `json:"start"`  // seconds from beat 0
	End    float64 `json:"end"`    // seconds from beat 0
	Width  int     `json:"width"`  // image size in pixels; 0 when unknown
	Height int     `json:"height"` // image size in pixels; 0 when unknown
}

// Placements lists the frames to place. With only set, it keeps just those shot ids.
// Every listed shot needs a frame image on disk.
func (sb *Board) Placements(ctx context.Context, only map[string]bool) ([]Placement, error) {
	ids := map[string]bool{}
	for _, s := range sb.Shots {
		ids[s.ID] = true
	}
	for id := range only {
		if !ids[id] {
			return nil, fmt.Errorf("no shot called %s", id)
		}
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
		w, h := imageSize(ctx, p)
		out = append(out, Placement{ID: s.ID, Path: p, Start: sb.Time(s.Beats[0]), End: sb.Time(s.Beats[1]), Width: w, Height: h})
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
