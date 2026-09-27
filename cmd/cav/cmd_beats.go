package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/rock3r/cavalry-skill/internal/beats"
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
wrong, run again with --bpm <expected>.`
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
