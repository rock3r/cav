package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/imagegen"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
)

func init() {
	register(command{
		name:    "board",
		args:    "init | frames | sheet | animatic | mood   (cav help board)",
		summary: "Plan a piece as a storyboard: frames per shot, a board image, an animatic on the music, and mood boards.",
		run:     cmdBoard,
	})
	longHelp["board"] = `
A storyboard is storyboard.json: the piece's tempo and size, a shared style, and shots
timed in beats (like the plan table cav guide asks for).

  cav board init [--bpm 120 --fps 60 --seconds 16 --shots 6] [-o storyboard.json]
        writes a starting storyboard; edit "what" for each shot.
  cav board frames [storyboard.json] [--service S] [--only s1,s3] [--force]
        makes one frame per shot into board/. The free path draws greybox cards; with an
        image key (cav config) it generates frames, sending style.refs with every one so
        the look stays consistent. A shot whose "frame" points at an image you made (for
        example with cav frame) keeps it.
  cav board sheet [storyboard.json] [-o renders/board.png]
        every frame in one labelled picture. Look at it.
  cav board animatic [storyboard.json] [--audio music.wav] [-o renders/animatic.mp4]
        each frame held for its beats, cut on the shot boundaries, with the music. Review
        it with cav review renders/animatic.mp4.
  cav board mood [moodboard/] [-o renders/moodboard.png]
        lays a folder of references out as one picture, each with its source and licence
        from the manifest (cav ref get fills the folder).

With "grid": "build/grid.json" (from cav beats --json) beats follow the real track
instead of a fixed bpm. Generated frames are recorded in .cav/manifest.json with the
provider, model and prompt.`
}

func boardPath(pos []string, def string) string {
	if len(pos) > 0 {
		return pos[0]
	}
	return def
}

func cmdBoard(a *app, args []string) error {
	if len(args) == 0 {
		printHelp("board")
		return nil
	}
	sub, args := args[0], args[1:]
	switch sub {
	case "init":
		return boardInit(a, args)
	case "frames":
		return boardFrames(a, args)
	case "sheet":
		return boardSheet(a, args)
	case "animatic":
		return boardAnimatic(a, args)
	case "mood":
		return boardMood(a, args)
	}
	return usageErr("unknown board command %q (init, frames, sheet, animatic, mood)", sub)
}

func boardInit(a *app, args []string) error {
	fs := flag.NewFlagSet("board init", flag.ContinueOnError)
	bpm := fs.Float64("bpm", 120, "tempo")
	fps := fs.Float64("fps", 60, "frame rate")
	seconds := fs.Float64("seconds", 16, "length")
	shots := fs.Int("shots", 6, "number of shots")
	width := fs.Int("width", 1920, "width")
	height := fs.Int("height", 1080, "height")
	out := fs.String("o", "storyboard.json", "file to write")
	force := fs.Bool("force", false, "overwrite an existing file")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil && !*force {
		return fail(exitError, *out+" already exists", "edit it, or pass --force to start over")
	}
	if *shots < 1 || *bpm <= 0 || *seconds <= 0 {
		return usageErr("--shots, --bpm and --seconds must be positive")
	}
	total := math.Floor(*seconds * *bpm / 60)
	sb := &board.Board{BPM: *bpm, FPS: *fps, Seconds: *seconds, Width: *width, Height: *height,
		Style: board.Style{Prompt: "flat motion-design look, clean shapes, generous empty space", Palette: []string{"#0b0d12", "#f5f7fa", "#ff4d2e"}}}
	// Split the beats into whole bars where possible, so cuts land on downbeats.
	per := total / float64(*shots)
	if per >= 4 {
		per = math.Floor(per/4) * 4
	}
	start := 0.0
	for i := 0; i < *shots; i++ {
		end := start + per
		if i == *shots-1 {
			end = total
		}
		sb.Shots = append(sb.Shots, board.Shot{ID: fmt.Sprintf("s%d", i+1), Beats: [2]float64{start, end}, What: "describe what happens in this shot"})
		start = end
	}
	if err := sb.Save(*out); err != nil {
		return err
	}
	a.emit(map[string]any{"file": *out, "shots": len(sb.Shots), "beats": total}, func() {
		fmt.Printf("wrote %s: %d shots over %g beats (%g s at %g bpm)\nEdit \"what\" for each shot, then: cav board frames\n", *out, len(sb.Shots), total, *seconds, *bpm)
	})
	return nil
}

// aspectOf picks the standard aspect ratio closest to w:h.
func aspectOf(w, h int) string {
	r := float64(w) / float64(h)
	best, diff := "16:9", 9.0
	for _, a := range []struct {
		s string
		v float64
	}{{"16:9", 16.0 / 9}, {"9:16", 9.0 / 16}, {"1:1", 1}, {"4:3", 4.0 / 3}, {"3:4", 3.0 / 4}, {"3:2", 1.5}, {"2:3", 2.0 / 3}, {"21:9", 21.0 / 9}, {"4:5", 0.8}, {"5:4", 1.25}} {
		if d := math.Abs(a.v - r); d < diff {
			best, diff = a.s, d
		}
	}
	return best
}

func framePrompt(sb *board.Board, s board.Shot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Storyboard frame for an animated motion-graphics piece (%s). ", aspectOf(sb.Width, sb.Height))
	if sb.Title != "" {
		fmt.Fprintf(&b, "Piece: %s. ", sb.Title)
	}
	fmt.Fprintf(&b, "This shot: %s. ", s.What)
	if s.Camera != "" {
		fmt.Fprintf(&b, "Camera: %s. ", s.Camera)
	}
	if s.Prompt != "" {
		b.WriteString(s.Prompt + ". ")
	}
	if sb.Style.Prompt != "" {
		fmt.Fprintf(&b, "Look: %s. ", sb.Style.Prompt)
	}
	if len(sb.Style.Palette) > 0 {
		fmt.Fprintf(&b, "Use only this palette: %s. ", strings.Join(sb.Style.Palette, ", "))
	}
	if len(sb.Style.Refs) > 0 {
		b.WriteString("Match the style of the reference images. ")
	}
	b.WriteString("Show one clear key moment. Keep the same style as the other frames of this storyboard.")
	return b.String()
}

func boardFrames(a *app, args []string) error {
	fs := flag.NewFlagSet("board frames", flag.ContinueOnError)
	service := fs.String("service", "", "image service (default: the image order in cav config)")
	only := fs.String("only", "", "comma-separated shot ids")
	force := fs.Bool("force", false, "remake frames that exist")
	dir := fs.String("dir", "board", "folder for the frames, next to the storyboard")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	path := boardPath(pos, "storyboard.json")
	sb, err := board.Load(path)
	if err != nil {
		return fail(exitError, err.Error(), "cav board init writes a starting storyboard")
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	ch, err := services.Pick(ctx, c, "image", *service)
	if err != nil {
		return fail(exitError, err.Error(), "see cav config")
	}
	var refs []string
	for _, r := range sb.Style.Refs {
		refs = append(refs, sb.Path(r))
	}
	want := map[string]bool{}
	for _, id := range splitList(*only) {
		want[id] = true
	}
	root := library.Root()
	m, err := library.Load(root)
	if err != nil {
		return err
	}
	type made struct {
		Shot   string `json:"shot"`
		Frame  string `json:"frame"`
		Source string `json:"source"`
		Kept   bool   `json:"kept,omitempty"`
	}
	var done []made
	for i := range sb.Shots {
		s := &sb.Shots[i]
		if len(want) > 0 && !want[s.ID] {
			continue
		}
		if s.Frame != "" && !*force {
			if _, err := os.Stat(sb.Path(s.Frame)); err == nil {
				done = append(done, made{s.ID, s.Frame, s.Source, true})
				continue
			}
		}
		var im *imagegen.Image
		prompt := framePrompt(sb, *s)
		if ch.Service == "greybox" {
			lines := []string{s.What}
			if s.Camera != "" {
				lines = append(lines, "camera: "+s.Camera)
			}
			data, err := imagegen.Greybox(imagegen.Card{Width: 1280, Height: 1280 * sb.Height / sb.Width, Title: s.ID,
				Lines: lines, Footer: sb.Label(*s), Palette: sb.Style.Palette})
			if err != nil {
				return err
			}
			im = &imagegen.Image{Data: data, MIME: "image/png", Provider: "greybox"}
		} else {
			fmt.Fprintf(os.Stderr, "%s: generating with %s…\n", s.ID, ch.Service)
			im, err = imagegen.Generate(ctx, c, ch, imagegen.Request{Prompt: prompt, Refs: refs, Aspect: aspectOf(sb.Width, sb.Height)})
			if err != nil {
				return fail(exitError, s.ID+": "+err.Error(), "the frames made so far are saved; run again to continue")
			}
		}
		rel := filepath.ToSlash(filepath.Join(*dir, s.ID+im.Ext()))
		out := sb.Path(rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, im.Data, 0o644); err != nil {
			return err
		}
		s.Frame, s.Source = rel, im.Provider
		if im.Provider != "greybox" {
			sum, _ := fileSHA(out)
			m.Put(root, library.Entry{Path: out, Kind: "image", Title: "storyboard " + s.ID, Source: im.Provider,
				Licence: "generated", CommercialOK: true, RetrievedAt: time.Now().UTC(), SHA256: sum,
				Generated: &library.Generated{Provider: im.Provider, Model: im.Model, Prompt: prompt, Refs: sb.Style.Refs}})
		}
		// Save after every frame, so an interrupted run keeps what it made.
		if err := sb.Save(path); err != nil {
			return err
		}
		if err := m.Save(root); err != nil {
			return err
		}
		done = append(done, made{s.ID, rel, im.Provider, false})
	}
	a.emit(map[string]any{"storyboard": path, "service": ch.Service, "skipped": ch.Skipped, "frames": done}, func() {
		for _, s := range ch.Skipped {
			fmt.Fprintln(os.Stderr, "note: skipped "+s)
		}
		for _, d := range done {
			state := "made with " + d.Source
			if d.Kept {
				state = "kept"
			}
			fmt.Printf("%-4s %s (%s)\n", d.Shot, d.Frame, state)
		}
		fmt.Println("Look at them together: cav board sheet")
	})
	return nil
}

func boardSheet(a *app, args []string) error {
	fs := flag.NewFlagSet("board sheet", flag.ContinueOnError)
	out := fs.String("o", filepath.Join(outDir(), "board.png"), "output PNG")
	cols := fs.Int("cols", 3, "columns")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	sb, err := board.Load(boardPath(pos, "storyboard.json"))
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := sb.Sheet(context.Background(), *out, *cols); err != nil {
		return fail(exitError, err.Error(), "")
	}
	a.emit(map[string]any{"file": *out, "shots": len(sb.Shots)}, func() { fmt.Printf("%s (%d shots)\n", *out, len(sb.Shots)) })
	return nil
}

func boardAnimatic(a *app, args []string) error {
	fs := flag.NewFlagSet("board animatic", flag.ContinueOnError)
	out := fs.String("o", filepath.Join(outDir(), "animatic.mp4"), "output MP4")
	audio := fs.String("audio", "", "music to play under it")
	noCap := fs.Bool("no-captions", false, "leave out the shot captions")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	sb, err := board.Load(boardPath(pos, "storyboard.json"))
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	secs, err := sb.Animatic(ctx, *audio, *out, !*noCap)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	a.emit(map[string]any{"file": *out, "seconds": secs, "shots": len(sb.Shots), "audio": *audio}, func() {
		fmt.Printf("%s: %.2f s, %d shots\nReview it: cav review %s\n", *out, secs, len(sb.Shots), *out)
	})
	return nil
}

func boardMood(a *app, args []string) error {
	fs := flag.NewFlagSet("board mood", flag.ContinueOnError)
	out := fs.String("o", filepath.Join(outDir(), "moodboard.png"), "output PNG")
	cols := fs.Int("cols", 0, "columns (default: 3 or 4)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	dir := boardPath(pos, "moodboard")
	imgs, err := board.MoodImages(dir)
	if err != nil {
		return fail(exitError, err.Error(), "put reference images in "+dir+", for example with cav ref get")
	}
	if len(imgs) == 0 {
		return fail(exitError, "no images in "+dir, "add some with cav ref search / cav ref get")
	}
	root := library.Root()
	m, _ := library.Load(root)
	byPath := map[string]library.Entry{}
	if m != nil {
		for _, e := range m.Assets {
			byPath[e.Path] = e
		}
	}
	var tiles []board.MoodTile
	for _, p := range imgs {
		cap := filepath.Base(p)
		if e, ok := byPath[library.Rel(root, p)]; ok {
			cap = e.Source + "  " + library.LicenceName(e.Licence)
			if e.Author != "" {
				cap += "  " + e.Author
			}
		}
		tiles = append(tiles, board.MoodTile{Path: p, Caption: cap})
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	w, h, err := board.Mood(context.Background(), tiles, *out, *cols)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	a.emit(map[string]any{"file": *out, "images": len(tiles), "width": w, "height": h}, func() {
		fmt.Printf("%s (%d images, %dx%d)\n", *out, len(tiles), w, h)
	})
	return nil
}
