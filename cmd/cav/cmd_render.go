package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/operation"
	"github.com/rock3r/cav/internal/sheet"
)

func init() {
	register(command{
		name:    "frame",
		args:    "[frame...] [--scale 50] [-o file.png|dir] [--timeout 5m]",
		summary: "Render single frames to PNG (the current frame if none is given).",
		run:     cmdFrame,
	})
	register(command{
		name:    "sheet",
		args:    "[frames] [--count 12] [--scale 25] [--cols N] [--bpm B] [-o sheet.png] [--timeout 5m]",
		summary: "Render frames and tile them into one labelled contact sheet.",
		run:     cmdSheet,
	})
	longHelp["sheet"] = `
Frames can be:
  (nothing)     12 frames spread evenly over the comp; --count N for another number
  45            just frame 45
  0,30,60,90    a list
  0-600:30      a range with a step (every 30th frame from 0 to 600)
Simulations (Forge Dynamics, particles) only advance when frames render in order:
use a step-1 range such as 0-59:1, or cav frames 0-599 --keep-every 10.
--comp ID|unique-name selects a composition and restores both composition and playhead.
Each tile is labelled "f<frame> <seconds>s". With --bpm, the label also shows the beat
number (b1 is the first beat), so you can check that hits land on the beat.
Frame and sheet commands return operation IDs. After --timeout (default 5m), resume
that operation rather than rendering again. At most 120 frames per command, scale 1..100.
Images are staged beside the output and published after the native job finishes.
The original playhead is restored in finally after normal completion or a recoverable error.
Images replace existing outputs; sheets retain the staged sheet, and --keep retains frame PNGs.
Look at the sheet image after every build step: it is the fastest way to catch
layers that are missing, off-screen, the wrong size or the wrong colour.`
	register(command{
		name:    "onion",
		args:    "[start-end | frames] [--count 8] [--scale 50] [--comp C] [-o onion.png]",
		summary: "Blend frames into one image that shows how things move (onion skin).",
		run:     cmdOnion,
	})
	longHelp["onion"] = `
Renders a few frames and draws them on top of each other. What stays still is drawn once
and dimmed. What moves is drawn once per frame: older frames are fainter and tinted from
blue towards orange, and the last frame is solid in its own colours with a white
outline. A legend under the image names each frame in its tint. Use it to see a motion
path, the spacing of an ease (close ghosts = slow, far apart = fast), overshoot, and the
order of a stagger.
Frames can be:
  (nothing)     8 frames spread evenly over the whole comp; --count N for another number
  0-60          8 frames spread evenly from 0 to 60 (best: one move at a time)
  0,10,20,40    a list; 0-60:5 a range with a step
The output also reports, for each pair of neighbouring frames, the share of pixels that
changed and the area where they changed (JSON with --json). A step with 0 % change is a
hold; one step far larger than its neighbours is a jump or pop.
Simulations only advance when frames render in order: give a step-1 range for them.`
	register(command{
		name:    "render",
		args:    "[-o out.mp4] [--audio track.wav] [--start F --end F] [--scale 100]",
		summary: "Render the active comp to MP4, optionally with audio muxed in by ffmpeg.",
		run:     cmdRender,
	})
	longHelp["render"] = `
Renders the active comp through Cavalry's render queue (an operation-specific item),
checks the frame count with ffprobe, and muxes --audio with ffmpeg (AAC, trimmed to
the video length). A full render can take minutes: cav reports "still running"
(exit code 3) after --timeout and continue the complete command with "cav operation resume <id>".
The timeout includes metadata, render, mux and validation. Outputs are staged beside
the destination and published without overwriting existing files. Staging is retained
as recovery evidence. A refused bridge or changed session returns exit 4 with an unknown
native outcome. operation status shows unvalidated file size/age; neither proves completion.
ffprobe is required. Raw job wait completes only one job.
--save explicitly saves the current named scene before metadata/render submission.
--chunk-frames 600 renders PNG sequences and validates each MP4 segment separately.
It needs ffmpeg and a saved named scene. --max-warmup 10000 bounds replay per chunk:
every new chunk evaluates from comp start, so simulations can recover correctly.
Scene input hashes and segment counts/hashes bind recovery. At most 1000 chunks.
After a native restart, reconcile its old outcome and reopen the same saved scene, then
use operation resume ID --restart-chunk --acknowledge-unknown-outcome. Completed
segments are retained; uncertain attempts are preserved before new native submission.
Ordinary resume never resubmits uncertain work.`
}

func outDir() string {
	if v := os.Getenv("CAV_OUT_DIR"); v != "" {
		return v
	}
	return "renders"
}

// frameRenderJS restores the playhead even when one native render fails.
const frameRenderJS = `
if (api.getActiveComp() !== sample.comp || api.getSceneFilePath() !== sample.scenePath)
  throw new Error('active scene/comp changed; restore the operation scene before resuming');
var cur = api.getFrame(), out = [];
try {
  sample.frames.forEach(function (f) {
    api.setFrame(f);
    var p = sample.dir + '/f_' + f;
    api.renderPNGFrame(p, sample.scale);
    if (!api.filePathExists(p + '.png')) throw new Error('Cavalry did not write ' + p + '.png');
    out.push(p + '.png');
  });
} finally { api.setFrame(cur) }
return out;`

// The operation's stable staging path keeps the native script identical on resume.
func (a *app) renderFrames(frames []int, scale int, dir string, st *sceneState) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	sample, err := json.Marshal(map[string]any{"frames": frames, "dir": filepath.ToSlash(dir), "scale": scale, "comp": st.Comp.ID, "scenePath": st.ScenePath})
	if err != nil {
		return nil, err
	}
	var paths []string
	if err = a.jsCall("var sample = "+string(sample)+";\n"+frameRenderJS, 5*time.Minute, &paths); err != nil {
		return nil, err
	}
	if len(paths) != len(frames) {
		return nil, fmt.Errorf("Cavalry returned %d images for %d frames", len(paths), len(frames))
	}
	for i, p := range paths {
		paths[i] = filepath.FromSlash(p)
		expected := filepath.Join(dir, fmt.Sprintf("f_%d.png", frames[i]))
		if paths[i] != expected {
			return nil, fmt.Errorf("unexpected frame output: %s", paths[i])
		}
		info, err := os.Stat(paths[i])
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("Cavalry did not write a regular image: %s", paths[i])
		}
	}
	return paths, nil
}

func (a *app) publishedImages() (bool, error) {
	if !a.op.Completed["images-published"] {
		return false, nil
	}
	for _, p := range a.op.Outputs {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			return true, fmt.Errorf("published image is missing: %s", p)
		}
	}
	var data map[string]any
	if err := json.Unmarshal(a.op.Data, &data); err != nil {
		return true, err
	}
	a.emit(data, func() {
		for _, p := range a.op.Outputs {
			fmt.Println(p)
		}
	})
	return true, nil
}

// Image commands intentionally replace their outputs, including iterative sheets.
// Copy to a sibling temporary file before rename; retain native staging for recovery.
func publishImage(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("image source is not a regular file: %s", src)
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(dst), ".cav-publish-*.png")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	buf := make([]byte, 64*1024)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, e := in.Read(buf)
		if n > 0 {
			if _, err = out.Write(buf[:n]); err != nil {
				return err
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	if err = out.Chmod(0o644); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Rename(out.Name(), dst)
}

func cmdFrame(a *app, args []string) error {
	fs := flag.NewFlagSet("frame", flag.ContinueOnError)
	comp := fs.String("comp", "", "composition ID or unique name")
	chronological := fs.Bool("chronological", false, "render intervening frames from the comp start")
	keepEvery := fs.Int("keep-every", 1, "retain every Nth requested frame (frames command)")
	maxEvaluated := fs.Int("max-evaluated", 10000, "maximum chronological evaluations (1..100000)")
	scale := fs.Int("scale", 50, "render scale in percent")
	out := fs.String("o", "", "output file (one frame) or folder")
	timeout := fs.Duration("timeout", 5*time.Minute, "total image operation wait budget")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	a.compSelector = *comp
	if *timeout <= 0 || *scale < 1 || *scale > 100 || *keepEvery < 1 || *maxEvaluated < 1 || *maxEvaluated > 100000 {
		return usageErr("invalid timeout or scale (1..100)")
	}
	var frames []int
	if len(pos) > 0 {
		limit := 120
		if *chronological {
			limit = *maxEvaluated
		}
		frames, err = parseFrameListLimit(strings.Join(pos, ","), limit)
		if err != nil {
			return err
		}
		if len(frames) == 0 {
			return usageErr("no frames specified")
		}
	}
	if err = a.beginOperationBudget(*timeout); err != nil {
		return err
	}
	if done, e := a.publishedImages(); done || e != nil {
		return e
	}
	if err = a.checkpoint("waiting-for-metadata"); err != nil {
		return err
	}
	st, err := getScene(a)
	if err != nil {
		return err
	}
	if len(frames) == 0 {
		frames = []int{st.Frame}
	}
	if *chronological {
		frames = sortedFrames(frames)
		kept := make([]int, 0)
		for i, f := range frames {
			if i%*keepEvery == 0 {
				kept = append(kept, f)
			}
		}
		frames = kept
		if len(frames) > 1000 || frames[0] < st.Comp.StartFrame || frames[len(frames)-1] > st.Comp.EndFrame || frames[len(frames)-1]-st.Comp.StartFrame+1 > *maxEvaluated {
			return usageErr("chronological frames exceed comp range, 1000 retained images or --max-evaluated")
		}
	}
	dir, single := a.op.OutputDir, ""
	if *out != "" {
		abs, e := filepath.Abs(*out)
		if e != nil {
			return e
		}
		if strings.HasSuffix(strings.ToLower(abs), ".png") && len(frames) == 1 {
			single = abs
			dir = filepath.Dir(abs)
		} else {
			dir = abs
		}
	}
	outputs := make([]string, len(frames))
	for i, f := range frames {
		outputs[i] = filepath.Join(dir, fmt.Sprintf("f_%d.png", f))
	}
	if single != "" {
		outputs[0] = single
	}
	a.op.IntendedOutputs = outputs
	staging := filepath.Join(dir, ".cav-frame-"+a.op.ID)
	if err = a.checkpoint("rendering-images"); err != nil {
		return err
	}
	var paths []string
	if *chronological {
		paths, err = a.renderChronologicalFrames(frames, *scale, staging, st)
	} else {
		paths, err = a.renderFrames(frames, *scale, staging, st)
	}
	if err != nil {
		return err
	}
	if err = a.checkpoint("publishing-images"); err != nil {
		return err
	}
	for i, p := range paths {
		if err = publishImage(a.ctx, p, outputs[i]); err != nil {
			return err
		}
	}
	a.op.Outputs = outputs
	a.emit(map[string]any{"frames": frames, "files": outputs, "staging": staging}, func() {
		for _, p := range outputs {
			fmt.Println(p)
		}
	})
	a.op.Completed["images-published"] = true
	return a.checkpoint("publication-complete")
}

// parseFrameList understands "12" (count, needs comp range), "0,30,60" and "0-600:30".
func parseFrameList(spec string, compRange *[2]int) ([]int, error) {
	return parseFrameListLimit(spec, 0)
}

func parseFrameListLimit(spec string, limit int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	var out []int
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") && !strings.HasPrefix(part, "-") {
			step := 1
			rng := part
			if i := strings.Index(part, ":"); i >= 0 {
				s, err := strconv.Atoi(part[i+1:])
				if err != nil || s <= 0 {
					return nil, usageErr("bad step in %q", part)
				}
				step, rng = s, part[:i]
			}
			ab := strings.SplitN(rng, "-", 2)
			x, err1 := strconv.Atoi(ab[0])
			y, err2 := strconv.Atoi(ab[1])
			if err1 != nil || err2 != nil || y < x {
				return nil, usageErr("bad range %q (use start-end:step)", part)
			}
			for f := x; f <= y; {
				if limit > 0 && len(out) >= limit {
					return nil, usageErr("too many frames (max %d)", limit)
				}
				out = append(out, f)
				if y-f < step {
					break
				}
				f += step
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, usageErr("bad frame %q", part)
		}
		if limit > 0 && len(out) >= limit {
			return nil, usageErr("too many frames (max %d)", limit)
		}
		out = append(out, n)
	}
	return out, nil
}

func evenFrames(n, start, end int) []int {
	if n <= 1 {
		return []int{start}
	}
	var out []int
	for i := 0; i < n; i++ {
		f := start + int(math.Round(float64(i)*float64(end-start)/float64(n-1)))
		if len(out) == 0 || out[len(out)-1] != f {
			out = append(out, f)
		}
	}
	return out
}

func cmdSheet(a *app, args []string) error {
	fs := flag.NewFlagSet("sheet", flag.ContinueOnError)
	comp := fs.String("comp", "", "composition ID or unique name")
	scale := fs.Int("scale", 25, "render scale in percent")
	cols := fs.Int("cols", 0, "tiles per row (default: picked from the count)")
	out := fs.String("o", "", "output PNG (default renders/sheet.png)")
	bpm := fs.Float64("bpm", 0, "label tiles with the beat number at this tempo")
	offset := fs.Float64("offset", 0, "with --bpm: time of the first beat in seconds")
	keep := fs.Bool("keep", false, "keep the single frame PNGs")
	count := fs.Int("count", 12, "number of evenly spread frames when no frames are given")
	timeout := fs.Duration("timeout", 5*time.Minute, "total image operation wait budget")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	a.compSelector = *comp
	if *timeout <= 0 || *scale < 1 || *scale > 100 || *count < 1 || *count > 120 || *cols < 0 || *cols > 120 || *bpm < 0 || math.IsNaN(*bpm) || math.IsInf(*bpm, 0) || math.IsNaN(*offset) || math.IsInf(*offset, 0) {
		return usageErr("invalid sheet arguments (scale 1..100, count 1..120, cols 0..120, positive timeout)")
	}
	var frames []int
	if len(pos) > 0 {
		frames, err = parseFrameListLimit(strings.Join(pos, ","), 120)
		if err != nil {
			return err
		}
		if len(frames) == 0 {
			return usageErr("no frames specified")
		}
	}
	if err = a.beginOperationBudget(*timeout); err != nil {
		return err
	}
	if done, e := a.publishedImages(); done || e != nil {
		return e
	}
	if err = a.checkpoint("waiting-for-metadata"); err != nil {
		return err
	}
	st, err := getScene(a)
	if err != nil {
		return err
	}
	if st.Comp.EndFrame < st.Comp.StartFrame || st.Comp.FPS <= 0 || math.IsNaN(st.Comp.FPS) || math.IsInf(st.Comp.FPS, 0) {
		return fmt.Errorf("invalid comp frame range or FPS")
	}
	if len(pos) == 0 {
		frames = evenFrames(*count, st.Comp.StartFrame, st.Comp.EndFrame)
	}
	if *out == "" {
		*out = filepath.Join(a.op.OutputDir, "sheet.png")
	}
	abs, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	staging := filepath.Join(filepath.Dir(abs), ".cav-sheet-"+a.op.ID)
	tmp := filepath.Join(staging, "frames")
	a.op.IntendedOutputs = []string{abs}
	if err = a.checkpoint("rendering-images"); err != nil {
		return err
	}
	paths, err := a.renderFrames(frames, *scale, tmp, st)
	if err != nil {
		return err
	}
	if err = a.checkpoint("building-sheet"); err != nil {
		return err
	}
	if err = a.ctx.Err(); err != nil {
		return err
	}
	tiles := make([]sheet.Tile, len(paths))
	for i, p := range paths {
		f := frames[i]
		label := fmt.Sprintf("f%d %.2fs", f, float64(f)/st.Comp.FPS)
		if *bpm > 0 {
			beat := (float64(f)/st.Comp.FPS-*offset)*(*bpm)/60 + 1
			label += fmt.Sprintf(" b%.1f", beat)
		}
		tiles[i] = sheet.Tile{Path: p, Label: label}
	}
	built := filepath.Join(staging, "sheet.png")
	w, h, err := sheet.Build(tiles, built, sheet.Options{Cols: *cols})
	if err != nil {
		return err
	}
	if err = a.checkpoint("publishing-images"); err != nil {
		return err
	}
	if err = publishImage(a.ctx, built, abs); err != nil {
		return err
	}
	a.op.Outputs = []string{abs}
	a.emit(map[string]any{"sheet": abs, "frames": frames, "width": w, "height": h, "staging": staging}, func() {
		fmt.Printf("%s  (%d frames, %dx%d)\nopen or view this image to review the animation\n", abs, len(frames), w, h)
	})
	a.op.Completed["images-published"] = true
	if err = a.checkpoint("publication-complete"); err != nil {
		return err
	}
	if !*keep {
		return os.RemoveAll(tmp)
	}
	return nil
}

func cmdOnion(a *app, args []string) error {
	fs := flag.NewFlagSet("onion", flag.ContinueOnError)
	comp := fs.String("comp", "", "composition ID or unique name")
	scale := fs.Int("scale", 50, "render scale in percent")
	out := fs.String("o", "", "output PNG (default renders/onion.png)")
	count := fs.Int("count", 8, "frames spread over the comp or over a start-end range")
	keep := fs.Bool("keep", false, "keep the single frame PNGs")
	timeout := fs.Duration("timeout", 5*time.Minute, "total image operation wait budget")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	a.compSelector = *comp
	if *count < 2 || *count > 24 || *scale < 1 || *scale > 100 || *timeout <= 0 {
		return usageErr("invalid onion arguments (count 2..24, scale 1..100, positive timeout)")
	}
	// "start-end" alone spreads --count frames over the range; anything else is a frame list.
	spec := strings.Join(pos, ",")
	spread := spec == "" || (strings.Count(spec, "-") == 1 && !strings.ContainsAny(spec, ",:") && !strings.HasPrefix(spec, "-"))
	var frames []int
	if spec != "" {
		if frames, err = parseFrameListLimit(spec, 100000); err != nil {
			return err
		}
		if len(frames) == 0 {
			return usageErr("no frames specified")
		}
	}
	if err = a.beginOperationBudget(*timeout); err != nil {
		return err
	}
	if done, e := a.publishedImages(); done || e != nil {
		return e
	}
	if err = a.checkpoint("waiting-for-metadata"); err != nil {
		return err
	}
	st, err := getScene(a)
	if err != nil {
		return err
	}
	if st.Comp.EndFrame < st.Comp.StartFrame || st.Comp.FPS <= 0 {
		return fmt.Errorf("invalid comp frame range or FPS")
	}
	switch {
	case spec == "":
		frames = evenFrames(*count, st.Comp.StartFrame, st.Comp.EndFrame)
	case spread:
		frames = evenFrames(*count, frames[0], frames[len(frames)-1])
	}
	if len(frames) < 2 || len(frames) > 24 {
		return usageErr("an onion needs 2 to 24 frames; got %d", len(frames))
	}
	if *out == "" {
		*out = filepath.Join(a.op.OutputDir, "onion.png")
	}
	abs, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	staging := filepath.Join(filepath.Dir(abs), ".cav-onion-"+a.op.ID)
	tmp := filepath.Join(staging, "frames")
	a.op.IntendedOutputs = []string{abs}
	if err = a.checkpoint("rendering-images"); err != nil {
		return err
	}
	paths, err := a.renderFrames(frames, *scale, tmp, st)
	if err != nil {
		return err
	}
	if err = a.checkpoint("building-sheet"); err != nil {
		return err
	}
	built := filepath.Join(staging, "onion.png")
	w, h, steps, err := sheet.Onion(paths, frames, built)
	if err != nil {
		return err
	}
	if err = a.checkpoint("publishing-images"); err != nil {
		return err
	}
	if err = publishImage(a.ctx, built, abs); err != nil {
		return err
	}
	a.op.Outputs = []string{abs}
	type step struct {
		From    int     `json:"from"`
		To      int     `json:"to"`
		Changed float64 `json:"changedPercent"`
		Box     []int   `json:"box,omitempty"`
	}
	js := make([]step, len(steps))
	for i, s := range steps {
		js[i] = step{From: s.From, To: s.To, Changed: math.Round(s.Changed*1000) / 10}
		if s.Changed > 0 {
			js[i].Box = s.Box[:]
		}
	}
	a.emit(map[string]any{"onion": abs, "frames": frames, "width": w, "height": h, "steps": js, "staging": staging}, func() {
		fmt.Printf("%s  (%d frames, %dx%d)\nchange between frames:", abs, len(frames), w, h)
		for _, s := range js {
			fmt.Printf("  %d>%d %.1f%%", s.From, s.To, s.Changed)
		}
		fmt.Println("\nopen or view this image: faint blue = early, solid with a white outline = last frame")
	})
	a.op.Completed["images-published"] = true
	if err = a.checkpoint("publication-complete"); err != nil {
		return err
	}
	if !*keep {
		return os.RemoveAll(tmp)
	}
	return nil
}
func cmdRender(a *app, args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	save := fs.Bool("save", false, "save the current named scene before submission")
	chunks := fs.Int("chunk-frames", 0, "render validated PNG/MP4 chunks (0 disables; 1..10000)")
	maxWarmup := fs.Int("max-warmup", 10000, "maximum replay frames per chunk")
	out := fs.String("o", "", "output MP4 (default renders/<comp>.mp4)")
	audio := fs.String("audio", "", "audio file to mux in")
	start := fs.Int("start", -1, "first frame")
	end := fs.Int("end", -1, "last frame, inclusive")
	scale := fs.Int("scale", 100, "resolution scale in percent")
	timeout := fs.Duration("timeout", 30*time.Minute, "total operation wait budget")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 || *timeout <= 0 || *scale < 1 || *scale > 100 || *chunks < 0 || *chunks > 10000 || *maxWarmup < 0 || *maxWarmup > 100000 {
		return usageErr("invalid render arguments, timeout or scale (1..100)")
	}
	if err := a.beginOperationBudget(*timeout); err != nil {
		return err
	}
	if *out != "" {
		p, e := filepath.Abs(*out)
		if e != nil {
			return e
		}
		a.op.IntendedOutputs = []string{p}
	}
	if *audio != "" {
		p, e := filepath.Abs(*audio)
		if e != nil {
			return e
		}
		digest, e := fileDigest(a.ctx, p)
		if e != nil {
			return e
		}
		if a.op.Inputs == nil {
			a.op.Inputs = map[string]string{}
		}
		if previous, ok := a.op.Inputs[p]; ok && previous != digest {
			return fmt.Errorf("audio input changed since the operation started: %s", p)
		}
		a.op.Inputs[p] = digest
	}
	if *save {
		if err = a.checkpoint("saving-scene"); err != nil {
			return err
		}
		var saved bool
		if err = a.jsCall("if(!api.getSceneFilePath())throw new Error('render --save requires a named scene'); return api.saveScene();", *timeout, &saved); err != nil {
			return err
		}
		if !saved {
			return fmt.Errorf("scene save failed; render not submitted")
		}
	}
	if err = a.checkpoint("waiting-for-metadata"); err != nil {
		return err
	}
	st, err := getScene(a)
	if err != nil {
		return err
	}
	if *start < 0 {
		*start = st.Comp.StartFrame
	}
	if *end < 0 {
		*end = st.Comp.EndFrame
	}
	if *end < *start || st.Comp.FPS <= 0 {
		return usageErr("invalid frame range or comp fps")
	}
	if *out == "" {
		dir := a.op.OutputDir
		if dir == "" {
			dir = outDir()
		}
		*out = filepath.Join(dir, safeName(st.Comp.Name)+".mp4")
	}
	if strings.ToLower(filepath.Ext(*out)) != ".mp4" {
		return usageErr("cav render writes MP4; use cav frame for images")
	}
	absOut, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if *audio != "" {
		p, e := filepath.Abs(*audio)
		if e != nil {
			return e
		}
		*audio = p
		if _, e = os.Stat(p); e != nil {
			return e
		}
		if _, e = exec.LookPath("ffmpeg"); e != nil {
			return fail(exitError, "ffmpeg is needed to mux audio", ffmpegFix())
		}
	}
	if err = os.MkdirAll(filepath.Dir(absOut), 0755); err != nil {
		return err
	}
	stage := filepath.Join(filepath.Dir(absOut), ".cav-"+a.op.ID)
	if len(a.op.Outputs) == 0 {
		if _, e := os.Lstat(absOut); !os.IsNotExist(e) {
			return fmt.Errorf("output already exists or cannot be inspected: %s", absOut)
		}
		records, e := filepath.Glob(filepath.Join(config.Home(), "operations", "*.json"))
		if e != nil {
			return e
		}
		for _, p := range records {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			var r operation.Record
			if e = json.Unmarshal(b, &r); e != nil {
				return e
			}
			if r.ID != a.op.ID {
				for _, o := range r.Outputs {
					if o == absOut && r.Status != "complete" && r.Status != "abandoned" {
						return fmt.Errorf("output reserved by operation %s; resume or inspect it", r.ID)
					}
				}
			}
		}
		a.op.Outputs = []string{absOut}
		a.op.IntendedOutputs = []string{absOut}
		if err = a.checkpoint("rendering"); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(stage, 0700); err != nil {
		return err
	}
	videoPath := filepath.Join(stage, "video.mp4")
	if a.op.Render == nil {
		a.op.Render = &operation.Render{Stage: stage, ExpectedFrames: *end - *start + 1, FPS: st.Comp.FPS,
			ChunkSize: *chunks, ScenePath: st.ScenePath, Comp: st.Comp.ID, Chunks: []operation.Chunk{}}
	}
	if *chunks > 0 {
		if err = a.renderChunks(st, chunkOptions{Start: *start, End: *end, Scale: *scale, MaxWarmup: *maxWarmup}); err != nil {
			return err
		}
	} else {
		code := fmt.Sprintf(`
if (api.getActiveComp() !== %s || api.getSceneFilePath() !== %s) throw new Error('active scene/comp changed; restore the operation scene before rendering');
var rq = api.addRenderQueueItem(api.getActiveComp());
api.rename(rq, %s);
api.setGenerator(rq, 'generator', 'renderMP4');
api.set(rq, { filePath: %s, fileName: 'video', frameRange: [%d, %d], resolutionScale: %d, ensureUniqueNames: false });
var t = Date.now();
api.render(rq);
return { ms: Date.now() - t };`, jsString(st.Comp.ID), jsString(st.ScenePath), jsString("cav render "+a.op.ID), jsString(filepath.ToSlash(stage)), *start, *end, *scale)
		if err = a.checkpoint("rendering"); err != nil {
			return err
		}
		o, err := a.execJS(code, execOpts{timeout: *timeout, source: "cav render", progress: true})
		if err != nil {
			return err
		}
		if !o.result.OK {
			return scriptErr(o)
		}
	}
	expected := *end - *start + 1
	finalStage := videoPath
	if *audio != "" {
		if err = a.checkpoint("postprocessing"); err != nil {
			return err
		}
		finalStage = filepath.Join(stage, "mux.mp4")
		// Only a partial, operation-owned mux may be replaced. The native video is retained.
		if !a.op.Completed["mux"] {
			if _, e := os.Stat(finalStage); os.IsNotExist(e) {
				digest, e := fileDigest(a.ctx, *audio)
				if e != nil {
					return e
				}
				if a.op.Inputs[*audio] != digest {
					return fmt.Errorf("audio input changed before mux")
				}
				partial := filepath.Join(stage, "mux.partial.mp4")
				cmd := exec.CommandContext(a.ctx, "ffmpeg", "-y", "-v", "error", "-i", videoPath, "-i", *audio, "-map", "0:v:0", "-map", "1:a:0", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-t", fmt.Sprintf("%.3f", float64(expected)/st.Comp.FPS), partial)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err = cmd.Run(); err != nil {
					if cause := a.ctx.Err(); cause != nil {
						return fmt.Errorf("ffmpeg mux interrupted: %w", cause)
					}
					return fmt.Errorf("ffmpeg mux failed: %s: %w", strings.TrimSpace(stderr.String()), err)
				}
				got, e := probeFramesContext(a.ctx, partial)
				if e != nil {
					return e
				}
				if got != expected {
					return fmt.Errorf("mux has %d frames; expected %d", got, expected)
				}
				if err = os.Link(partial, finalStage); err != nil {
					return err
				}
			} else if e != nil {
				return e
			}
			// A complete mux published before a crash is retained even if its checkpoint was lost.
			a.op.Completed["mux"] = true
			if err = a.checkpoint("postprocessing"); err != nil {
				return err
			}
		}

	}
	if err = a.checkpoint("validation"); err != nil {
		return err
	}
	if err = a.ctx.Err(); err != nil {
		return err
	}
	got, err := probeFramesContext(a.ctx, finalStage)
	if err != nil {
		return fmt.Errorf("video validation failed (staged video retained): %w", err)
	}
	if got != expected {
		return fmt.Errorf("video has %d frames; expected %d (staged video retained)", got, expected)
	}
	if *chunks > 0 {
		if err = a.bindSceneInput(st.ScenePath); err != nil {
			return err
		}
	}
	// Hard-link publication is atomic and never overwrites another command's output.
	// Staging shares the destination filesystem. Retain the source link as recovery evidence.
	if err = os.Link(finalStage, absOut); err != nil {
		src, se := os.Stat(finalStage)
		dst, de := os.Stat(absOut)
		if se != nil || de != nil || !os.SameFile(src, dst) {
			return fmt.Errorf("cannot publish without overwriting %s: %w", absOut, err)
		}
	}
	result := map[string]any{"file": absOut, "frames": got, "expectedFrames": expected, "fps": st.Comp.FPS, "staging": stage}
	if *audio != "" {
		result["audio"] = *audio
	}
	a.emit(result, func() { fmt.Printf("%s\n%d frames at %g fps\n", absOut, got, st.Comp.FPS) })
	return nil
}

func probeFramesContext(ctx context.Context, path string) (int, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-count_packets", "-select_streams", "v:0", "-show_entries", "stream=nb_read_packets", "-of", "csv=p=0", path).Output()
	if err != nil {
		if cause := ctx.Err(); cause != nil {
			return 0, cause
		}
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.Split(string(out), "\n")[0]))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid ffprobe frame count %q", out)
	}
	return n, nil
}

func probeFrames(path string) int {
	out, err := exec.Command("ffprobe", "-v", "error", "-count_packets", "-select_streams", "v:0",
		"-show_entries", "stream=nb_read_packets", "-of", "csv=p=0", path).Output()
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.Split(string(out), "\n")[0]))
	if err != nil {
		return -1
	}
	return n
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == ' ' {
			return '_'
		}
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return -1
		}
		return r
	}, s)
	if s == "" {
		s = "render"
	}
	return s
}

func fileDigest(ctx context.Context, path string) (string, error) {
	out, err := runInputWorker(ctx, "hash", path)
	if err != nil {
		return "", err
	}
	digest := strings.TrimSpace(string(out))
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("invalid input digest")
	}
	return digest, nil
}
