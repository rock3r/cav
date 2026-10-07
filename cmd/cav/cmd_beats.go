package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rock3r/cav/internal/beats"
)

func init() {
	register(command{
		name:    "beats",
		args:    "<audio> [--fps 60] [--bpm hint] [-o grid.json] | --bpm B --seconds S [--offset O]",
		summary: "Find the tempo and beats of a track and print a frame-based beat grid.",
		run:     cmdBeats,
	})
	longHelp["beats"] = `
With an audio file, cav decodes it with ffmpeg and detects the tempo, the beats and
the downbeats (first beat of each 4/4 bar). Give --bpm when you know the tempo: it
limits the search to ±8 %.
Without an audio file, --bpm and --seconds make an exact grid.

The output lists every beat as a time and as a frame at --fps. Key your animation
to these frames: cuts and hits on downbeats, smaller accents on beats. In a script,
load the grid with: var grid = JSON.parse(api.readFromFile('/abs/path/grid.json'))
or pass the numbers in directly.
Check the result: detection can pick half or double the real tempo. If the BPM looks
wrong, run again with --bpm <expected>.

With an audio file it also prints the shape of the track: sections (where the music
changes), events (rise = a sudden jump in loudness, fall = a drop-out, silence = a gap)
and the strongest accents with their band (low, mid, high). --json has the full lists
and barEnergy (0 = quietest bar, 1 = loudest). cav spectrogram draws all of this.`
}

func init() {
	register(command{
		name:    "spectrogram",
		args:    "<audio> [--fps 60] [--bpm hint] [--from S --to S] [--width 1600] [-o spectrogram.png]",
		summary: "Draw a track as a picture: spectrogram, loudness, beats, sections and accents.",
		run:     cmdSpectrogram,
	})
	longHelp["spectrogram"] = `
You cannot hear a track, but you can look at it. This draws one image with all lanes on
one time axis (seconds and frames at --fps):
  top     spectrogram: low notes at the bottom, high at the top, brighter = louder.
          White lines are bars (numbered above), short ticks are beats, yellow lines
          are section boundaries (s2, s3, ...).
  db      loudness. Red shading = silence, green triangle = sudden rise, red = fall.
  low     onsets in three lanes: low (kick, bass), mid (snare, stabs, voice) and hi
  mid     (hats, cymbals). Taller = stronger, each lane scaled on its own. A white dot
  hi      marks one of the strongest accents of the track.
How to read it: a bright band that starts at a bar line is a new instrument; a wide
bright column is a hit or a drop; a dark gap is a break; a rising streak is a riser that
lands where it ends. Use --from/--to (seconds) to zoom into a part.
The same numbers are in cav beats <audio> --json.`
}

func init() {
	register(command{
		name:    "sync",
		args:    "<video.mp4> [--audio track] [--bpm hint] [--from S --to S] [-o sync.png]",
		summary: "Check that the cuts and hits of a rendered video land on the music.",
		run:     cmdSync,
	})
	longHelp["sync"] = `
Reads a rendered video and its music (the video's own sound, or --audio), and lines up
the moments where the picture changes sharply with the beats:
  cut     one frame changes a lot (a cut or a flash)
  peak    the fastest frame of a move, burst or slam: where a viewer feels the hit
  stop    motion lands and rests (the end of a move)
Each visual hit is reported with its distance in frames to the nearest beat (minus =
early). Hits within a tolerance (about 35 ms) of a beat or half beat are on the grid.
It also lists the strong musical moments (section starts, sudden rises, silences and
the strongest kicks and stabs) that no visual hit answers.
The picture is the cav spectrogram view with one more lane, "pic": how much the picture
changes per frame. Hits are marked green on the beat grid and red off it.
A cut or flash should be on the beat. A pop or slam should peak on it (the move starts
earlier). A move that lands on the beat shows as a stop on it. Treat the report as
evidence, not a verdict: the grid itself can be a frame or two off on long tracks.`
}

type beatGrid struct {
	BPM            float64   `json:"bpm"`
	FPS            float64   `json:"fps"`
	Duration       float64   `json:"duration"`
	FramesPerBeat  float64   `json:"framesPerBeat"`
	Offset         float64   `json:"offset"`
	Confidence     float64   `json:"confidence"`
	Beats          []float64 `json:"beats"`
	BeatFrames     []int     `json:"beatFrames"`
	Downbeats      []float64 `json:"downbeats"`
	DownbeatFrames []int     `json:"downbeatFrames"`
	Source         string    `json:"source"`
	// Only with an audio file:
	Sections  []sectionOut `json:"sections,omitempty"`
	Events    []eventOut   `json:"events,omitempty"`
	Accents   []accentOut  `json:"accents,omitempty"`
	BarEnergy []float64    `json:"barEnergy,omitempty"`
}

type sectionOut struct {
	beats.Section
	StartFrame int `json:"startFrame"`
	EndFrame   int `json:"endFrame"`
}
type eventOut struct {
	beats.Event
	Frame    int `json:"frame"`
	EndFrame int `json:"endFrame,omitempty"`
}
type accentOut struct {
	beats.Accent
	Frame int `json:"frame"`
}

// describe adds the sections, events and accents of the track to the grid.
func (g *beatGrid) describe(st beats.Structure) {
	fr := func(t float64) int { return int(math.Round(t * g.FPS)) }
	for _, s := range st.Sections {
		g.Sections = append(g.Sections, sectionOut{s, fr(s.Start), fr(s.End)})
	}
	for _, e := range st.Events {
		o := eventOut{Event: e, Frame: fr(e.Start)}
		if e.End > 0 {
			o.EndFrame = fr(e.End)
		}
		g.Events = append(g.Events, o)
	}
	for _, a := range st.Accents {
		g.Accents = append(g.Accents, accentOut{a, fr(a.Time)})
	}
	g.BarEnergy = st.BarEnergy
}

func cmdBeats(a *app, args []string) error {
	fs := flag.NewFlagSet("beats", flag.ContinueOnError)
	fps := fs.Float64("fps", 60, "frames per second of the comp")
	bpm := fs.Float64("bpm", 0, "tempo: a hint with audio, exact without")
	seconds := fs.Float64("seconds", 0, "grid length when there is no audio")
	offset := fs.Float64("offset", 0, "time of the first beat (grid without audio)")
	out := fs.String("o", "", "also write the grid to this JSON file")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	var r beats.Result
	var structure *beats.Structure
	src := "grid"
	switch {
	case len(pos) == 1:
		samples, err := beats.Decode(pos[0])
		if err != nil {
			return fail(exitError, err.Error(), ffmpegFix())
		}
		r = beats.Analyze(samples, *bpm)
		src = pos[0]
		if r.BPM == 0 {
			return fail(exitError, "no steady beat found", "pass the tempo with --bpm")
		}
		st := beats.NewProfile(samples).Describe(r)
		structure = &st
	case *bpm > 0 && *seconds > 0:
		r = beats.Grid(*bpm, *offset, *seconds)
	default:
		return usageErr("usage: cav beats <audio> | cav beats --bpm 120 --seconds 16")
	}
	g := beatGrid{BPM: r.BPM, FPS: *fps, Duration: r.Duration, FramesPerBeat: math.Round(60/r.BPM**fps*100) / 100,
		Confidence: r.Confidence, Beats: r.Beats, Downbeats: r.Downbeats, Source: src}
	if len(r.Beats) > 0 {
		g.Offset = r.Beats[0]
	}
	for _, t := range r.Beats {
		g.BeatFrames = append(g.BeatFrames, int(math.Round(t**fps)))
	}
	for _, t := range r.Downbeats {
		g.DownbeatFrames = append(g.DownbeatFrames, int(math.Round(t**fps)))
	}
	if structure != nil {
		g.describe(*structure)
	}
	if *out != "" {
		b, _ := json.MarshalIndent(g, "", "  ")
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			return err
		}
	}
	data := map[string]any{}
	b, _ := json.Marshal(g)
	_ = json.Unmarshal(b, &data)
	if *out != "" {
		data["file"] = *out
	}
	a.emit(data, func() {
		fmt.Printf("%.2f BPM (confidence %.2f), %.1f frames per beat at %g fps, %.2f s\n", g.BPM, g.Confidence, g.FramesPerBeat, g.FPS, g.Duration)
		fmt.Printf("first beat at %.3f s (frame %d); %d beats, %d bars\n", g.Offset, first(g.BeatFrames), len(g.Beats), len(g.Downbeats))
		fmt.Printf("downbeat frames: %s\n", clip(fmt.Sprint(g.DownbeatFrames), 300))
		fmt.Printf("beat frames: %s\n", clip(fmt.Sprint(g.BeatFrames), 400))
		if len(g.Sections) > 0 {
			fmt.Println("sections (frames, loudness):")
			for _, s := range g.Sections {
				fmt.Printf("  %5d-%-5d bar %-3d %-6s %6.1f dBFS", s.StartFrame, s.EndFrame, s.Bar, s.Level, s.Loudness)
				if s.Change != 0 {
					fmt.Printf("  %+.1f dB", s.Change)
				}
				fmt.Println()
			}
		}
		for _, e := range g.Events {
			if e.Kind == "silence" {
				fmt.Printf("silence  frames %d-%d\n", e.Frame, e.EndFrame)
			} else {
				fmt.Printf("%-8s frame %d (%+.1f dB)\n", e.Kind, e.Frame, e.DB)
			}
		}
		// Strong kicks, snares and stabs between the beats are syncopations worth a hit of
		// their own; off-beat hi-hats are only texture.
		var off []string
		for _, a := range g.Accents {
			if !a.OnGrid && a.Band != "high" && a.Strength >= 0.5 {
				off = append(off, fmt.Sprintf("%d(%s)", a.Frame, a.Band))
			}
		}
		if len(g.Accents) > 0 {
			fmt.Printf("%d strong accents (--json lists them); strong low/mid accents off the grid: %s\n", len(g.Accents), clip(strings.Join(off, " "), 300))
		}
		if *out != "" {
			fmt.Printf("grid written to %s\n", *out)
		}
		if g.Confidence < 0.3 && src != "grid" {
			fmt.Println("warning: low confidence; check the tempo, or pass --bpm")
		}
	})
	return nil
}

func first(x []int) int {
	if len(x) == 0 {
		return 0
	}
	return x[0]
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + " …]"
	}
	return s
}

func cmdSpectrogram(a *app, args []string) error {
	fs := flag.NewFlagSet("spectrogram", flag.ContinueOnError)
	fps := fs.Float64("fps", 60, "frames per second of the comp, for frame numbers")
	bpm := fs.Float64("bpm", 0, "tempo hint")
	from := fs.Float64("from", 0, "start of the picture in seconds")
	to := fs.Float64("to", 0, "end of the picture in seconds (default: end of the track)")
	width := fs.Int("width", 1600, "image width in pixels")
	out := fs.String("o", "", "output PNG (default renders/spectrogram.png)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *width < 300 || *width > 8000 || *fps <= 0 {
		return usageErr("usage: cav spectrogram <audio> [--from S --to S] [--width 300..8000]")
	}
	samples, err := beats.Decode(pos[0])
	if err != nil {
		return fail(exitError, err.Error(), ffmpegFix())
	}
	r := beats.Analyze(samples, *bpm)
	prof := beats.NewProfile(samples)
	st := prof.Describe(r)
	if *out == "" {
		*out = filepath.Join(outDir(), "spectrogram.png")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	w, h, err := beats.Picture(prof, beats.View{Start: *from, End: *to, Width: *width, FPS: *fps, Grid: r, Structure: &st}, *out)
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(*out)
	a.emit(map[string]any{"image": abs, "width": w, "height": h, "bpm": r.BPM, "duration": prof.Duration, "sections": len(st.Sections)}, func() {
		fmt.Printf("%s  (%dx%d, %.2f BPM, %.1f s, %d sections)\nopen or view this image; cav help spectrogram explains the lanes\n", abs, w, h, r.BPM, prof.Duration, len(st.Sections))
	})
	return nil
}

type syncHit struct {
	beats.VisualHit
	Time   float64 `json:"time"`
	Beat   float64 `json:"beat"`   // position on the grid, 0 = first beat
	Offset int     `json:"offset"` // frames from the nearest beat or half beat; minus = early
	OnGrid bool    `json:"onGrid"`
}

type syncMoment struct {
	Kind  string  `json:"kind"` // section, rise, silence, accent
	Time  float64 `json:"time"`
	Frame int     `json:"frame"`
	Note  string  `json:"note,omitempty"`
}

func cmdSync(a *app, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	audio := fs.String("audio", "", "music file (default: the video's own sound)")
	bpm := fs.Float64("bpm", 0, "tempo hint")
	from := fs.Float64("from", 0, "start of the picture in seconds")
	to := fs.Float64("to", 0, "end of the picture in seconds")
	width := fs.Int("width", 1600, "image width in pixels")
	out := fs.String("o", "", "output PNG (default renders/sync.png)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *width < 300 || *width > 8000 {
		return usageErr("usage: cav sync <video.mp4> [--audio track]")
	}
	video := pos[0]
	fps, err := beats.VideoFPS(video)
	if err != nil {
		return fail(exitError, err.Error(), ffmpegFix())
	}
	src := *audio
	if src == "" {
		src = video
	}
	samples, err := beats.Decode(src)
	if err != nil || len(samples) == 0 {
		msg := "the video has no sound"
		if err != nil {
			msg = err.Error()
		}
		return fail(exitError, msg, "pass the music with --audio track.wav")
	}
	change, err := beats.VisualChange(video)
	if err != nil {
		return fail(exitError, err.Error(), ffmpegFix())
	}
	r := beats.Analyze(samples, *bpm)
	if r.BPM == 0 || len(r.Beats) < 2 {
		return fail(exitError, "no steady beat found in the music", "pass the tempo with --bpm")
	}
	prof := beats.NewProfile(samples)
	st := prof.Describe(r)
	tol := max(1, int(math.Round(0.035*fps)))

	var hits []syncHit
	var marks []beats.Mark
	onGrid := 0
	for _, h := range beats.FindHits(change) {
		t := float64(h.Frame) / fps
		// Measured against the detected beats, so a track that drifts in tempo is judged
		// against its real beats, not a constant-tempo projection.
		beat, nearest := beats.Place(r.Beats, t)
		off := h.Frame - int(math.Round(nearest*fps))
		sh := syncHit{VisualHit: h, Time: math.Round(t*1000) / 1000, Beat: math.Round(beat*100) / 100, Offset: off, OnGrid: abs(off) <= tol}
		if sh.OnGrid {
			onGrid++
		}
		hits = append(hits, sh)
		marks = append(marks, beats.Mark{Time: t, OK: sh.OnGrid})
	}
	// Strong musical moments that no visual hit answers.
	var moments []syncMoment
	for i, s := range st.Sections {
		if i > 0 {
			moments = append(moments, syncMoment{Kind: "section", Time: s.Start, Note: fmt.Sprintf("s%d, bar %d, %s", i+1, s.Bar, s.Level)})
		}
	}
	for _, e := range st.Events {
		switch {
		case e.Kind == "rise":
			moments = append(moments, syncMoment{Kind: e.Kind, Time: e.Start, Note: fmt.Sprintf("%+.1f dB", e.DB)})
		// Silence before the first beat is the lead-in, not a pause in the music.
		case e.Kind == "silence" && e.Start > math.Max(r.Beats[0], 0.25):
			moments = append(moments, syncMoment{Kind: e.Kind, Time: e.Start, Note: fmt.Sprintf("%.2f s", e.End-e.Start)})
		}
	}
	for _, ac := range st.Accents {
		if ac.Band != "high" && ac.Strength >= 0.6 {
			moments = append(moments, syncMoment{Kind: "accent", Time: ac.Time, Note: ac.Band})
		}
	}
	sort.Slice(moments, func(i, j int) bool { return moments[i].Time < moments[j].Time })
	var missed []syncMoment
	window := max(2, int(math.Round(0.07*fps)))
	for _, m := range moments {
		m.Frame = int(math.Round(m.Time * fps))
		if m.Frame >= len(change) {
			continue
		}
		answered := false
		for _, h := range hits {
			if abs(h.Frame-m.Frame) <= window {
				answered = true
				break
			}
		}
		if !answered && (len(missed) == 0 || missed[len(missed)-1].Frame != m.Frame) {
			missed = append(missed, m)
		}
	}

	if *out == "" {
		*out = filepath.Join(outDir(), "sync.png")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	w, h, err := beats.Picture(prof, beats.View{Start: *from, End: *to, Width: *width, FPS: fps, Grid: r, Structure: &st, Visual: change, Hits: marks}, *out)
	if err != nil {
		return err
	}
	img, _ := filepath.Abs(*out)
	a.emit(map[string]any{"image": img, "width": w, "height": h, "fps": fps, "bpm": r.BPM, "frames": len(change), "toleranceFrames": tol,
		"hits": hits, "onGrid": onGrid, "unanswered": missed}, func() {
		fmt.Printf("%s  (%dx%d)\n%.2f BPM, %d frames at %g fps; tolerance ±%d frames\n", img, w, h, r.BPM, len(change), fps, tol)
		fmt.Printf("%d visual hits, %d on the beat grid\n", len(hits), onGrid)
		var off []string
		for _, h := range hits {
			if !h.OnGrid {
				off = append(off, fmt.Sprintf("%s@%d(%+d)", h.Kind, h.Frame, h.Offset))
			}
		}
		if len(off) > 0 {
			fmt.Printf("off the grid (frames from the nearest beat or half beat): %s\n", clip(strings.Join(off, " "), 600))
		}
		if len(missed) > 0 {
			fmt.Println("musical moments with no visual hit within", window, "frames:")
			for _, m := range missed {
				fmt.Printf("  frame %-5d %-8s %s\n", m.Frame, m.Kind, m.Note)
			}
		}
		fmt.Println("open or view the image; cav help sync explains it")
	})
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
