package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/score"
)

func init() {
	register(command{
		name:    "score",
		args:    "[storyboard.json] [--video renders/final.mp4] [--audio take.mp3]... [-o score/project.rpp] | render <project.rpp>",
		summary: "Write a REAPER project to finish the music by ear: the render, a marker per shot, and the takes.",
		run:     cmdScore,
	})
	longHelp["score"] = `
cav score storyboard.json --video renders/final.mp4 --audio music/take1.mp3 --audio music/take2.mp3
    writes score/project.rpp, a REAPER project (plain text) with the tempo, a marker at
    every shot, the render on a video track, and each take on its own Bed track (the first
    heard, the others muted for comparing), plus an empty SFX track. Open it in REAPER,
    pick a take, trim, fade and fix the sync by ear.
cav score render score/project.rpp
    renders it from the command line (reaper -renderproject) to score/<name>-music.wav,
    then: cav render --audio score/<name>-music.wav. The REAPER window opens while it runs.
    On macOS and Windows, when REAPER has no audio device yet, it first sets REAPER's
    default, so REAPER does not stop to ask.
Needs REAPER (https://www.reaper.fm). The project layout follows REAPER 7.`
}

func cmdScore(a *app, args []string) error {
	if len(args) > 0 && args[0] == "render" {
		return scoreRender(a, args[1:])
	}
	fs := flag.NewFlagSet("score", flag.ContinueOnError)
	video := fs.String("video", "", "the render to put on the video track (default: the newest MP4 in renders/)")
	var takes stringList
	fs.Var(&takes, "audio", "a music take (repeatable)")
	out := fs.String("o", filepath.Join("score", "project.rpp"), "the project file to write")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	sb, err := board.Load(boardPath(pos, "storyboard.json"))
	if err != nil {
		return fail(exitError, err.Error(), "cav score needs a storyboard for the tempo and the shot markers")
	}
	if *video == "" {
		if v, err := reviewVideo(nil); err == nil {
			*video = v
		}
	}
	abs := func(p string) string {
		if p == "" {
			return ""
		}
		a, _ := filepath.Abs(p)
		return a
	}
	p := score.Project{BPM: sb.BPM, Seconds: sb.Seconds, Video: abs(*video)}
	if p.BPM == 0 {
		p.BPM = 120
	}
	for _, s := range sb.Shots {
		p.Markers = append(p.Markers, score.Marker{Name: s.ID + ": " + s.What, Seconds: sb.Time(s.Beats[0])})
	}
	for _, t := range takes {
		if _, err := os.Stat(t); err != nil {
			return fail(exitError, "cannot read "+t, "")
		}
		secs := 0.0
		if d, err := probeDuration(t); err == nil {
			secs = d
		}
		p.Takes = append(p.Takes, score.Take{Path: abs(t), Seconds: secs})
	}
	if p.Seconds == 0 && *video != "" {
		if d, err := probeDuration(*video); err == nil {
			p.Seconds = d
		}
	}
	base := strings.TrimSuffix(filepath.Base(*out), filepath.Ext(*out))
	p.RenderTo = abs(filepath.Join(filepath.Dir(*out), base+"-music.wav"))
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, []byte(p.RPP()), 0o644); err != nil {
		return err
	}
	a.emit(map[string]any{"project": *out, "markers": len(p.Markers), "takes": len(p.Takes), "video": p.Video, "renderTo": p.RenderTo}, func() {
		fmt.Printf("wrote %s: %d markers, %d takes", *out, len(p.Markers), len(p.Takes))
		if p.Video != "" {
			fmt.Printf(", video %s", filepath.Base(p.Video))
		}
		fmt.Printf("\nOpen it in REAPER to finish by ear, then: cav score render %s\n", *out)
	})
	return nil
}

func probeDuration(path string) (float64, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, err
	}
	var d float64
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%g", &d)
	return d, err
}

func reaperBinary() string {
	if p, err := exec.LookPath("reaper"); err == nil {
		return p
	}
	switch runtime.GOOS {
	case "darwin":
		for _, p := range []string{"/Applications/REAPER.app/Contents/MacOS/REAPER", "/Applications/REAPER64.app/Contents/MacOS/REAPER"} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	case "windows":
		for _, p := range []string{`C:\Program Files\REAPER (x64)\reaper.exe`, `C:\Program Files\REAPER\reaper.exe`} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// pickReaperAudioDevice selects REAPER's default audio device in its settings on macOS
// and Windows when none is set yet, as answering REAPER's first-run question would. Without it,
// -renderproject stops on that question and waits for a click. It returns the file it
// changed, or "" when it changed nothing.
func pickReaperAudioDevice(bin string) (string, error) {
	ini, err := reaperINI(bin, runtime.GOOS)
	if err != nil || ini == "" {
		return "", err
	}
	b, err := os.ReadFile(ini)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	text, changed := score.WithAudioDevice(string(b), runtime.GOOS)
	if !changed {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(ini), 0o755); err != nil {
		return "", err
	}
	return ini, os.WriteFile(ini, []byte(text), 0o644)
}

// reaperINI is the reaper.ini that the REAPER binary bin reads. A portable install keeps
// its reaper.ini beside the program: next to reaper.exe on Windows, next to REAPER.app on
// macOS. Otherwise REAPER uses the per-user file. It returns "" on other systems, and an
// error when the per-user folder cannot be found.
func reaperINI(bin, goos string) (string, error) {
	if p, err := filepath.EvalSymlinks(bin); err == nil {
		bin = p
	}
	dir := filepath.Dir(bin)
	if goos == "darwin" {
		// .../REAPER.app/Contents/MacOS/REAPER: the folder that holds REAPER.app.
		dir = filepath.Dir(filepath.Dir(filepath.Dir(dir)))
	}
	portable := filepath.Join(dir, "reaper.ini")
	if _, err := os.Stat(portable); err == nil && (goos == "darwin" || goos == "windows") {
		return portable, nil
	}
	switch goos {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "REAPER", "reaper.ini"), nil
	case "windows":
		dir, err := os.UserConfigDir() // %AppData%
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "REAPER", "reaper.ini"), nil
	}
	return "", nil
}

func scoreRender(a *app, args []string) error {
	pos, err := parseFlags(flag.NewFlagSet("score render", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav score render <project.rpp>")
	}
	bin := reaperBinary()
	if bin == "" {
		return fail(exitError, "REAPER is not installed", "install it from https://www.reaper.fm, or render from REAPER's File > Render")
	}
	rpp, _ := filepath.Abs(pos[0])
	picked, err := pickReaperAudioDevice(bin)
	if err != nil {
		return fail(exitError, "cannot set REAPER's audio device: "+err.Error(), "open REAPER once and pick a device in Preferences > Audio > Device")
	}
	if picked != "" && !a.json {
		fmt.Fprintf(os.Stderr, "REAPER had no audio device yet; set REAPER's default in %s so the render does not stop to ask\n", picked)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, bin, score.ReaperArgs(rpp)...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return fail(exitError, "REAPER render failed: "+err.Error()+": "+strings.TrimSpace(string(b)), "open the project in REAPER and render from File > Render to see the problem")
	}
	base := strings.TrimSuffix(filepath.Base(rpp), filepath.Ext(rpp))
	wav := filepath.Join(filepath.Dir(rpp), base+"-music.wav")
	info, err := os.Stat(wav)
	if err != nil || info.ModTime().Before(start) {
		return fail(exitError, "REAPER finished but did not write "+wav, "check the project's render settings in REAPER (File > Render)")
	}
	a.emit(map[string]any{"wav": wav}, func() {
		fmt.Printf("%s\nAdd it to the piece: cav render --audio %s\n", wav, wav)
	})
	return nil
}
