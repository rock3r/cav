package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rock3r/cav/assets"
)

func init() {
	longHelp["seams"] = `Compare f-1/f at in/out and opacity-key boundaries in the selected comp.
--video uses an existing full-comp video: video frame zero maps to comp start.
Without a video, simulations evaluate chronologically with a --max-evaluated budget.
--allow-cuts 60,120 annotates intended cuts without hiding their measured differences.
--threshold (default .05) selects the changed-pixel fraction requiring review.
--tolerance (default 8) ignores small per-channel pixel differences.
Bounding boxes use sampled-image pixels, with exclusive right/bottom endpoints.
--max-boundaries defaults to 60, maximum 500. Metadata omissions are explicit.
Differences are review signals. Referenced comp boundaries require their own --comp.
After timeout, resume the operation ID. ffmpeg/ffprobe are required for --video.`
	register(command{name: "seams", args: "[--video file.mp4] [--comp ID|name] [--allow-cuts F,...] [--max-boundaries 60]",
		summary: "Compare rendered frame pairs at section/opacity boundaries for review.", run: cmdSeams})
}

type seamResult struct {
	Frame           int     `json:"frame"`
	ChangedFraction float64 `json:"changedFraction"`
	MeanDifference  float64 `json:"meanDifference"`
	Box             *[4]int `json:"box,omitempty"` // left, top, right, bottom in sampled pixels
	IntendedCut     bool    `json:"intendedCut"`
	Review          bool    `json:"review"`
	Before          string  `json:"before"`
	After           string  `json:"after"`
}

func cmdSeams(a *app, args []string) error {
	fs := flag.NewFlagSet("seams", flag.ContinueOnError)
	video := fs.String("video", "", "finished video (frame zero maps to comp start)")
	comp := fs.String("comp", "", "composition ID or unique name")
	cuts := fs.String("allow-cuts", "", "intended cut frames")
	maxBoundaries := fs.Int("max-boundaries", 60, "maximum pairs (1..500)")
	maxEvaluated := fs.Int("max-evaluated", 10000, "maximum native simulation evaluations")
	scale := fs.Int("scale", 25, "native render scale percent")
	threshold := fs.Float64("threshold", 0.05, "changed-pixel fraction requiring review (0..1)")
	tolerance := fs.Int("tolerance", 8, "per-channel pixel difference tolerance (0..255)")
	timeout := fs.Duration("timeout", 10*time.Minute, "total operation budget")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 ||
		*maxBoundaries < 1 ||
		*maxBoundaries > 500 ||
		*maxEvaluated < 1 ||
		*maxEvaluated > 100000 ||
		*scale < 1 ||
		*scale > 100 ||
		*threshold < 0 ||
		*threshold > 1 ||
		math.IsNaN(*threshold) ||
		*tolerance < 0 ||
		*tolerance > 255 ||
		*timeout <= 0 {
		return usageErr("invalid seam limits")
	}
	a.compSelector = *comp
	allowed := map[int]bool{}
	if *cuts != "" {
		frames, e := parseFrameListLimit(*cuts, 1000)
		if e != nil {
			return e
		}
		for _, f := range frames {
			allowed[f] = true
		}
	}
	if err = a.beginOperationBudget(*timeout); err != nil {
		return err
	}
	code, _ := assets.Diagnostics.ReadFile("diagnostics/structure.js")
	var d structureData
	if err = a.jsCall("var limits={layers:10000,edges:10000,scan:100000,comps:64,keys:2000,ms:3000};\n"+string(code), *timeout, &d); err != nil {
		return err
	}
	boundaries := []int{}
	for _, l := range d.Layers {
		if l.Comp == d.Comp {
			for _, f := range l.Boundaries {
				if f > d.Start && f <= d.End {
					boundaries = append(boundaries, f)
				}
			}
		}
	}
	boundaries = sortedFrames(boundaries)
	omitted := max(0, len(boundaries)-*maxBoundaries)
	if omitted > 0 {
		boundaries = boundaries[:*maxBoundaries]
	}
	samples := []int{}
	for _, f := range boundaries {
		samples = append(samples, f-1, f)
	}
	samples = sortedFrames(samples)
	dir := filepath.Join(a.op.OutputDir, ".cav-seams-"+a.op.ID)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	paths := map[int]string{}
	source := "native"
	if *video != "" {
		source = "video"
	}
	var abs string
	if *video != "" {
		var e error
		abs, e = filepath.Abs(*video)
		if e != nil {
			return e
		}
		digest, e := fileDigest(a.ctx, abs)
		if e != nil {
			return e
		}
		if a.op.Inputs == nil {
			a.op.Inputs = map[string]string{}
		}
		if prev, ok := a.op.Inputs[abs]; ok && prev != digest {
			return fmt.Errorf("seam video changed")
		}
		a.op.Inputs[abs] = digest
		if err = a.checkpoint("decoding-seam-frames"); err != nil {
			return err
		}
		count, e := probeFramesContext(a.ctx, abs)
		if e != nil {
			return e
		}
		if count != d.End-d.Start+1 {
			return fmt.Errorf("video has %d frames; comp has %d; provide a finished full-comp video", count, d.End-d.Start+1)
		}
	}
	if len(samples) > 0 {
		if *video != "" {
			source = "video"
			terms := []string{}
			for _, f := range samples {
				terms = append(terms, "eq(n,"+strconv.Itoa(f-d.Start)+")")
			}
			filter := "select='" + strings.Join(terms, "+") + "'"
			output := filepath.Join(dir, "video_%04d.png")
			cmd := exec.CommandContext(a.ctx, "ffmpeg", "-y", "-v", "error", "-i", abs, "-vf", filter, "-fps_mode", "passthrough", output)
			if b, e := cmd.CombinedOutput(); e != nil {
				return fmt.Errorf("seam decode failed: %w: %s", e, b)
			}
			for i, f := range samples {
				paths[f] = filepath.Join(dir, fmt.Sprintf("video_%04d.png", i+1))
			}
		} else {
			st, e := getScene(a)
			if e != nil {
				return e
			}
			if st.Comp.ID != d.Comp || st.ScenePath != d.ScenePath {
				return fmt.Errorf("seam scene changed")
			}
			if err = a.checkpoint("rendering-seam-frames"); err != nil {
				return err
			}
			var images []string
			if needsChronological(d) {
				if samples[len(samples)-1]-d.Start+1 > *maxEvaluated {
					return usageErr("seams need more than --max-evaluated frames; use a finished --video")
				}
				images, err = a.renderChronologicalFrames(samples, *scale, dir, st)
			} else {
				images, err = a.renderFrames(samples, *scale, dir, st)
			}
			if err != nil {
				return err
			}
			for i, f := range samples {
				paths[f] = images[i]
			}
		}
	}
	results := make([]seamResult, 0, len(boundaries))
	for _, f := range boundaries {
		if err = a.ctx.Err(); err != nil {
			return err
		}
		r, e := compareSeam(a.ctx, paths[f-1], paths[f], *tolerance)
		if e != nil {
			return e
		}
		r.Frame = f
		r.Before = paths[f-1]
		r.After = paths[f]
		r.IntendedCut = allowed[f]
		r.Review = !r.IntendedCut && r.ChangedFraction >= *threshold
		results = append(results, r)
	}
	a.emit(map[string]any{
		"seams": results, "source": source, "omittedBoundaries": omitted,
		"failures": d.Failures, "skipped": d.Skipped,
		"complete": omitted == 0 && len(d.Failures) == 0 && len(d.Skipped) == 0,
		"scope":    "active comp boundaries; inspect referenced comps with --comp", "staging": dir,
	}, func() {
		for _, r := range results {
			fmt.Printf("frame %d: %.1f%% changed, box %v, intended cut=%t, review=%t\n", r.Frame, 100*r.ChangedFraction, r.Box, r.IntendedCut, r.Review)
		}
		fmt.Printf("%d boundaries inspected, %d omitted; source=%s; differences are review signals\n", len(results), omitted, source)
	})
	return nil
}

func readSeamImage(ctx context.Context, path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	config, _, err := image.DecodeConfig(seamReader{ctx: ctx, r: f})
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16000000 {
		return nil, fmt.Errorf("seam image exceeds pixel limit")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	im, _, err := image.Decode(seamReader{ctx: ctx, r: f})
	return im, err
}
func compareSeam(ctx context.Context, before, after string, tolerance int) (seamResult, error) {
	a, err := readSeamImage(ctx, before)
	if err != nil {
		return seamResult{}, err
	}
	b, err := readSeamImage(ctx, after)
	if err != nil {
		return seamResult{}, err
	}
	bounds := a.Bounds()
	if bounds != b.Bounds() {
		return seamResult{}, fmt.Errorf("seam dimensions differ")
	}
	r := seamResult{}
	box := [4]int{bounds.Max.X, bounds.Max.Y, bounds.Min.X, bounds.Min.Y}
	changed, total, sum := 0, 0, uint64(0)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return seamResult{}, err
		}
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			diffs := []int{absInt(int(ar>>8) - int(br>>8)), absInt(int(ag>>8) - int(bg>>8)), absInt(int(ab>>8) - int(bb>>8)), absInt(int(aa>>8) - int(ba>>8))}
			hit := false
			for _, d := range diffs {
				sum += uint64(d)
				if d > tolerance {
					hit = true
				}
			}
			total++
			if hit {
				changed++
				box[0] = min(box[0], x)
				box[1] = min(box[1], y)
				box[2] = max(box[2], x+1)
				box[3] = max(box[3], y+1)
			}
		}
	}
	if changed > 0 {
		r.Box = &box
	}
	r.ChangedFraction = float64(changed) / float64(total)
	r.MeanDifference = float64(sum) / float64(total*4*255)
	return r, nil
}
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

type seamReader struct {
	ctx context.Context
	r   io.Reader
}

func (r seamReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
