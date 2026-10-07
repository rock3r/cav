package beats

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// The video is compared at this small size: enough to see a cut or a move, cheap to read.
const visW, visH = 160, 90

// VideoFPS reads the frame rate of the first video stream with ffprobe.
func VideoFPS(path string) (float64, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=r_frame_rate", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	a, b, ok := strings.Cut(strings.TrimSpace(strings.Split(string(out), "\n")[0]), "/")
	x, err1 := strconv.ParseFloat(a, 64)
	y, err2 := 1.0, error(nil)
	if ok {
		y, err2 = strconv.ParseFloat(b, 64)
	}
	if err1 != nil || err2 != nil || x <= 0 || y <= 0 {
		return 0, fmt.Errorf("no video frame rate in %s", path)
	}
	return x / y, nil
}

// VisualChange returns, for every video frame, how much it differs from the frame before:
// the mean absolute difference of a small grey copy, 0 (identical) to 1 (black to white).
// Frame 0 has no frame before it and gets 0.
func VisualChange(path string) ([]float64, error) {
	cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-an", "-vf", fmt.Sprintf("scale=%d:%d,format=gray", visW, visH), "-f", "rawvideo", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("ffmpeg is needed to read video: %w", err)
		}
		return nil, err
	}
	r := bufio.NewReaderSize(pipe, 1<<20)
	prev, cur := make([]byte, visW*visH), make([]byte, visW*visH)
	var out []float64
	for {
		if _, err := io.ReadFull(r, cur); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return nil, err
		}
		if len(out) == 0 {
			out = append(out, 0)
		} else {
			s := 0
			for i := range cur {
				d := int(cur[i]) - int(prev[i])
				if d < 0 {
					d = -d
				}
				s += d
			}
			out = append(out, float64(s)/float64(len(cur))/255)
		}
		prev, cur = cur, prev
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// VisualHit is a moment where the picture changes sharply.
type VisualHit struct {
	Frame int     `json:"frame"`
	Kind  string  `json:"kind"` // cut (one frame changes a lot), peak (the fastest frame of a move or burst) or stop (motion lands)
	Size  float64 `json:"size"` // how sharp, in the units of VisualChange
}

// FindHits picks the cuts, peaks and stops in a VisualChange curve.
//   - cut: one frame changes far more than the frames on both sides (a cut or a flash);
//   - peak: the top of a bump in the curve, the fastest frame of a move, burst or slam. That
//     is where a viewer feels the hit; the move itself usually starts earlier, on purpose;
//   - stop: the change falls to near rest within a frame or two (a move lands and holds).
func FindHits(c []float64) []VisualHit {
	if len(c) < 4 {
		return nil
	}
	sorted := append([]float64{}, c...)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]
	// Ignore what is small next to the typical motion of this video, and tiny flicker.
	floor := math.Max(0.004, 3*median)
	at := func(i int) float64 {
		if i < 0 || i >= len(c) {
			return 0
		}
		return c[i]
	}
	lowest := func(a, b int) float64 {
		m := math.Inf(1)
		for i := max(0, a); i <= min(len(c)-1, b); i++ {
			m = math.Min(m, c[i])
		}
		return m
	}
	var hits []VisualHit
	for f := 1; f < len(c); f++ {
		prev, next := at(f-1), at(f+1)
		// Prominence: how far the bump rises above the lower of the valleys on its two sides.
		prom := c[f] - math.Max(lowest(f-12, f-1), lowest(f+1, f+12))
		switch {
		case c[f]-math.Max(prev, next) > floor && c[f] > 4*math.Max(prev, next):
			hits = append(hits, VisualHit{f, "cut", c[f]})
		// A peak rises above the frames just before it: the flat top of a steady move is not one.
		case c[f] >= prev && c[f] > next && prom > floor && prom > 0.5*c[f] && c[f] > 1.15*(at(f-1)+at(f-2)+at(f-3))/3:
			hits = append(hits, VisualHit{f, "peak", prom})
		// A stop comes straight from full speed; the slow tail of an ease-out is not one.
		case prev-c[f] > floor && c[f] < prev/4 && next < prev/4 && prev >= 0.5*math.Max(math.Max(at(f-2), at(f-3)), at(f-4)):
			hits = append(hits, VisualHit{f, "stop", prev - c[f]})
		}
	}
	// Keep the sharpest hit within any 3 frames.
	var out []VisualHit
	for _, h := range hits {
		if n := len(out); n > 0 && h.Frame-out[n-1].Frame <= 3 {
			if h.Size > out[n-1].Size {
				out[n-1] = h
			}
			continue
		}
		out = append(out, h)
	}
	for i := range out {
		out[i].Size = math.Round(out[i].Size*10000) / 10000
	}
	return out
}
