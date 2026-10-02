package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/sheet"
)

func init() {
	register(command{
		name:    "frame",
		args:    "[frame...] [--scale 50] [-o file.png|dir]",
		summary: "Render single frames to PNG (the current frame if none is given).",
		run:     cmdFrame,
	})
	register(command{
		name:    "sheet",
		args:    "[frames] [--count 12] [--scale 25] [--cols N] [--bpm B] [-o sheet.png]",
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
use a step-1 range such as 0-59:1 to preview them.
Each tile is labelled "f<frame> <seconds>s". With --bpm, the label also shows the beat
number (b1 is the first beat), so you can check that hits land on the beat.
Look at the sheet image after every build step: it is the fastest way to catch
layers that are missing, off-screen, the wrong size or the wrong colour.`
	register(command{
		name:    "render",
		args:    "[-o out.mp4] [--audio track.wav] [--start F --end F] [--scale 100]",
		summary: "Render the active comp to MP4, optionally with audio muxed in by ffmpeg.",
		run:     cmdRender,
	})
	longHelp["render"] = `
Renders the active comp through Cavalry's render queue (an item named "cav render"),
checks the frame count with ffprobe, and muxes --audio with ffmpeg (AAC, trimmed to
the video length). A full render can take minutes: cav reports "still running"
(exit code 3) after --timeout and you can resume with "cav job wait <id>".`
}

func outDir() string {
	if v := os.Getenv("CAV_OUT_DIR"); v != "" {
		return v
	}
	return "renders"
}

// renderFrames renders the given frames to dir/f_<frame>.png inside one job.
func (a *app) renderFrames(frames []int, scale int, dir string) ([]string, error) {
	abs, _ := filepath.Abs(dir)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	fr, _ := json.Marshal(frames)
	code := fmt.Sprintf(`
var frames = %s, dir = %s, scale = %d, cur = api.getFrame(), out = [];
frames.forEach(function (f) { api.setFrame(f); var p = dir + '/f_' + f; api.renderPNGFrame(p, scale); out.push(p + '.png') });
api.setFrame(cur);
return out;`, fr, jsString(filepath.ToSlash(abs)), scale)
	var paths []string
	timeout := time.Duration(max(2, len(frames))) * 30 * time.Second
	if err := a.jsCall(code, timeout, &paths); err != nil {
		return nil, err
	}
	for i, p := range paths {
		paths[i] = filepath.FromSlash(p)
		if _, err := os.Stat(paths[i]); err != nil {
			return nil, fmt.Errorf("Cavalry did not write %s", paths[i])
		}
	}
	return paths, nil
}

func cmdFrame(a *app, args []string) error {
	fs := flag.NewFlagSet("frame", flag.ContinueOnError)
	scale := fs.Int("scale", 50, "render scale in percent")
	out := fs.String("o", "", "output file (one frame) or folder")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	var frames []int
	for _, p := range pos {
		f, err := parseFrameList(p, nil)
		if err != nil {
			return err
		}
		frames = append(frames, f...)
	}
	if len(frames) == 0 {
		var cur int
		if err := a.jsCall("return api.getFrame()", time.Minute, &cur); err != nil {
			return err
		}
		frames = []int{cur}
	}
	dir := outDir()
	single := ""
	if *out != "" {
		if strings.HasSuffix(strings.ToLower(*out), ".png") && len(frames) == 1 {
			single = *out
			dir = filepath.Dir(*out)
		} else {
			dir = *out
		}
	}
	paths, err := a.renderFrames(frames, *scale, dir)
	if err != nil {
		return err
	}
	if single != "" {
		if err := os.Rename(paths[0], single); err != nil {
			return err
		}
		paths[0], _ = filepath.Abs(single)
	}
	a.emit(map[string]any{"frames": frames, "files": paths}, func() {
		for _, p := range paths {
			fmt.Println(p)
		}
	})
	return nil
}

// parseFrameList understands "12" (count, needs comp range), "0,30,60" and "0-600:30".
func parseFrameList(spec string, compRange *[2]int) ([]int, error) {
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
			for f := x; f <= y; f += step {
				out = append(out, f)
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, usageErr("bad frame %q", part)
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
	scale := fs.Int("scale", 25, "render scale in percent")
	cols := fs.Int("cols", 0, "tiles per row (default: picked from the count)")
	out := fs.String("o", "", "output PNG (default renders/sheet.png)")
	bpm := fs.Float64("bpm", 0, "label tiles with the beat number at this tempo")
	offset := fs.Float64("offset", 0, "with --bpm: time of the first beat in seconds")
	keep := fs.Bool("keep", false, "keep the single frame PNGs")
	count := fs.Int("count", 12, "number of evenly spread frames when no frames are given")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	st, err := getScene(a)
	if err != nil {
		return err
	}
	var frames []int
	if len(pos) == 0 {
		frames = evenFrames(*count, st.Comp.StartFrame, st.Comp.EndFrame)
	} else if frames, err = parseFrameList(strings.Join(pos, ","), nil); err != nil {
		return err
	}
	if len(frames) > 120 {
		return usageErr("%d frames is too many for one sheet (max 120); use a larger step", len(frames))
	}
	if *out == "" {
		*out = filepath.Join(outDir(), "sheet.png")
	}
	tmp := filepath.Join(filepath.Dir(*out), ".frames-"+bridge.NewID())
	paths, err := a.renderFrames(frames, *scale, tmp)
	if err != nil {
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
	w, h, err := sheet.Build(tiles, *out, sheet.Options{Cols: *cols})
	if err != nil {
		return err
	}
	if !*keep {
		_ = os.RemoveAll(tmp)
	}
	abs, _ := filepath.Abs(*out)
	a.emit(map[string]any{"sheet": abs, "frames": frames, "width": w, "height": h}, func() {
		fmt.Printf("%s  (%d frames, %dx%d)\nopen or view this image to review the animation\n", abs, len(frames), w, h)
	})
	return nil
}

func cmdRender(a *app, args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	out := fs.String("o", "", "output MP4 (default renders/<comp>.mp4)")
	audio := fs.String("audio", "", "audio file to mux in")
	start := fs.Int("start", -1, "first frame (default: comp start)")
	end := fs.Int("end", -1, "last frame, inclusive (default: comp end)")
	scale := fs.Int("scale", 100, "resolution scale in percent")
	timeout := fs.Duration("timeout", 30*time.Minute, "report 'still running' after this long")
	if _, err := parseFlags(fs, args); err != nil {
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
	if *out == "" {
		*out = filepath.Join(outDir(), safeName(st.Comp.Name)+".mp4")
	}
	if ext := strings.ToLower(filepath.Ext(*out)); ext != ".mp4" {
		return usageErr("cav render writes MP4 video; for single images use `cav frame <n> -o file.png`, for a review use `cav sheet`")
	}
	absOut, _ := filepath.Abs(*out)
	if err := os.MkdirAll(filepath.Dir(absOut), 0o755); err != nil {
		return err
	}
	if *audio != "" {
		if _, err := os.Stat(*audio); err != nil {
			return usageErr("audio file %s: %v", *audio, err)
		}
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return fail(exitError, "ffmpeg is needed to mux audio", ffmpegFix())
		}
	}
	base := strings.TrimSuffix(filepath.Base(absOut), filepath.Ext(absOut))
	videoBase := base
	if *audio != "" {
		videoBase = base + ".video"
	}
	videoPath := filepath.Join(filepath.Dir(absOut), videoBase+".mp4")
	_ = os.Remove(videoPath)
	code := fmt.Sprintf(`
var comp = api.getActiveComp(), rq = null;
api.getRenderQueueItems().forEach(function (id) { if (api.getNiceName(id) === 'cav render') rq = id });
if (!rq) { rq = api.addRenderQueueItem(comp); api.rename(rq, 'cav render') }
if (api.getCurrentGeneratorType(rq, 'generator') !== 'renderMP4') api.setGenerator(rq, 'generator', 'renderMP4');
api.set(rq, { filePath: %s, fileName: %s, frameRange: [%d, %d], resolutionScale: %d, ensureUniqueNames: false });
var t = Date.now();
api.render(rq);
return { ms: Date.now() - t };`, jsString(filepath.ToSlash(filepath.Dir(absOut))), jsString(videoBase), *start, *end, *scale)
	o, err := a.execJS(code, execOpts{timeout: *timeout, source: "cav render", progress: true})
	if err != nil {
		return err
	}
	if !o.result.OK {
		return scriptErr(o)
	}
	if _, err := os.Stat(videoPath); err != nil {
		return fail(exitError, "Cavalry finished but "+videoPath+" does not exist", "check the render queue in Cavalry (Window > Render Manager)")
	}
	expected := *end - *start + 1
	got := probeFrames(videoPath)
	result := map[string]any{"expectedFrames": expected, "frames": got, "fps": st.Comp.FPS}
	if *audio != "" {
		dur := float64(expected) / st.Comp.FPS
		cmd := exec.Command("ffmpeg", "-y", "-v", "error", "-i", videoPath, "-i", *audio,
			"-map", "0:v:0", "-map", "1:a:0", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
			"-t", fmt.Sprintf("%.3f", dur), absOut)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fail(exitError, "ffmpeg mux failed: "+strings.TrimSpace(stderr.String()), "the video without audio is at "+videoPath)
		}
		_ = os.Remove(videoPath)
		result["audio"] = *audio
	}
	result["file"] = absOut
	a.emit(result, func() {
		fmt.Printf("%s\n%d frames (expected %d) at %g fps", absOut, got, expected, st.Comp.FPS)
		if *audio != "" {
			fmt.Printf(", audio from %s", *audio)
		}
		fmt.Println()
		if got >= 0 && got != expected {
			fmt.Println("warning: the frame count differs from the requested range")
		}
	})
	return nil
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
