package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/pyrun"
)

type renderedMusic struct {
	Out     string   `json:"out"`
	Seconds float64  `json:"seconds"`
	LUFS    float64  `json:"lufs"`
	Peak    float64  `json:"peak"`
	Stems   []string `json:"stems"`
}

// cmdMusicRender renders a written score (JSON) to a mixed WAV locally: synthesised
// instruments and drums, samples, and VST3/AU instruments through pedalboard.
func cmdMusicRender(a *app, args []string) error {
	fs := flag.NewFlagSet("music render", flag.ContinueOnError)
	out := fs.String("o", "", "output WAV (default: music/<score name>.wav)")
	stems := fs.Bool("stems", false, "also write one WAV per track, next to the mix")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("give one score file: cav music render score.json")
	}
	scorePath := pos[0]
	raw, err := os.ReadFile(scorePath)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	var probe struct {
		BPM    float64 `json:"bpm"`
		Tracks []struct {
			Instrument string `json:"instrument"`
		} `json:"tracks"`
		Master struct {
			Reference string `json:"reference"`
		} `json:"master"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return fail(exitError, "the score is not valid JSON: "+err.Error(), "see cav guide production for the format")
	}
	if probe.BPM <= 0 || len(probe.Tracks) == 0 {
		return fail(exitError, "the score needs a bpm and at least one track", "see cav guide production for the format")
	}
	if *out == "" {
		*out = filepath.Join("music", strings.TrimSuffix(filepath.Base(scorePath), filepath.Ext(scorePath))+".wav")
	}
	absOut, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOut), 0o755); err != nil {
		return err
	}
	stemDir := ""
	if *stems {
		stemDir = strings.TrimSuffix(absOut, filepath.Ext(absOut)) + "-stems"
		if err := os.MkdirAll(stemDir, 0o755); err != nil {
			return err
		}
	}
	absScore, err := filepath.Abs(scorePath)
	if err != nil {
		return err
	}
	packages := []string{"numpy", "pedalboard", "pyloudnorm"}
	for _, t := range probe.Tracks {
		if strings.HasPrefix(t.Instrument, "vst3:") || strings.HasPrefix(t.Instrument, "au:") {
			packages = append(packages, "mido")
			break
		}
	}
	if probe.Master.Reference != "" {
		packages = append(packages, "matchering")
	}
	script, err := assets.Python.ReadFile("python/render_music.py")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	fmt.Fprintln(os.Stderr, "rendering the score (the first run fetches the Python packages)…")
	res, err := pyrun.Run(ctx, filepath.Join(config.CacheDir(), "python"), "render_music.py", script, packages,
		[]string{absScore, absOut, stemDir}, nil)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	var r renderedMusic
	if err := json.Unmarshal(res, &r); err != nil {
		return fail(exitError, "unexpected renderer output: "+err.Error(), "")
	}
	r.Out = *out
	root := library.Root()
	m, err := library.Load(root)
	if err != nil {
		return err
	}
	sum, _ := fileSHA(*out)
	m.Put(root, library.Entry{Path: *out, Kind: "audio", Source: "cav music render", Licence: "generated", CommercialOK: true,
		RetrievedAt: time.Now().UTC(), SHA256: sum, Generated: &library.Generated{Provider: "local", Model: "render_music.py", Prompt: scorePath}})
	if err := m.Save(root); err != nil {
		return err
	}
	a.emit(map[string]any{"out": r.Out, "seconds": r.Seconds, "lufs": r.LUFS, "peak": r.Peak, "stems": r.Stems}, func() {
		fmt.Printf("%s  %.2f s, %.1f LUFS, peak %.1f dBFS\n", r.Out, r.Seconds, r.LUFS, r.Peak)
		for _, s := range r.Stems {
			fmt.Println("  stem " + s)
		}
		fmt.Printf("Listen to it: cav listen %s\n", r.Out)
	})
	return nil
}
