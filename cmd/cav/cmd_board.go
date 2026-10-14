package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/imagegen"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
	"github.com/rock3r/cav/internal/videogen"
)

func init() {
	register(command{
		name:    "board",
		args:    "init | frames | motion | sheet | animatic | place | mood   (cav help board)",
		summary: "Plan a piece as a storyboard: frames per shot, a board image, an animatic on the music, and mood boards.",
		run:     cmdBoard,
	})
	longHelp["board"] = `
A storyboard is storyboard.json: the piece's tempo and size, a shared style, and shots
timed in beats (like the plan table cav guide asks for).

  cav board init [--bpm 120 --fps 60 --seconds 16 --width 1920 --height 1080 --shots 6]
        [--brief brief.json | --no-brief] [-o storyboard.json]
        writes a starting storyboard; edit "what" and "why" for each shot. Values you do not give come
        from brief.json when it exists (size, fps, length, tempo and title); flags win. It
        prints what it took from the brief.
  cav board frames [storyboard.json] [--service S] [--only s1,s3] [--force]
        makes one frame per shot into board/. With an image key (cav config) it generates
        frames, sending style.refs with every one so the look stays consistent. With no
        image service, a shot with board/<id>.svg (simple shapes, written by you) is
        rendered through Cavalry as a sketch, and the others get greybox cards.
        --service sketch renders only sketches. A sketch is drawn again when its SVG is
        newer than its PNG. Needs Cavalry open; the open scene must be saved or empty, and
        is reopened afterwards. A shot whose "frame" points at an image you made (for
        example with cav frame) keeps it.
  cav board motion [storyboard.json] --only s2,s4 | --all [--service veo] [--force]
        turns a shot's frame into a moving clip (Veo 3.1 Lite by default, with the Gemini
        key) into board/<id>.mp4 and sets the shot's "clip". Veo makes 4, 6 or 8 seconds:
        the clip covers the shot, and a longer shot holds the clip's last frame. Each clip
        costs money, so name the shots or pass --all.
  cav board sheet [storyboard.json] [-o renders/board.png]
        every frame in one labelled picture. Look at it.
  cav board animatic [storyboard.json] [--audio music.wav] [-o renders/animatic.mp4]
        each frame held for its beats (or its clip played), cut on the shot boundaries,
        with the music. Review it with cav review renders/animatic.mp4.
  cav board place [storyboard.json] [--only s1,s3] [--comp NAME]
        adds each shot's frame to the open Cavalry composition as an image layer named
        by its shot id, visible from its start beat to its end beat and scaled to fit.
        Time 0 of the music lands on the composition's start frame, so a storyboard
        "offset" (or a beat grid) moves beat 0 later, as it does in the music.
        Run it again to update the placeholders: it points image layers with those names at
        the current frames and timing, keeping their parent and transforms (delete one to
        have it fitted again), and leaves alone a shot whose name a built layer (a group, a
        shape) already uses.
  cav board mood [moodboard/] [-o renders/moodboard.png] [--file moodboard.json]
        syncs moodboard.json (beside the folder) with the images and clips in it and the
        videos added with cav ref add <youtube or vimeo link>, makes a contact sheet of
        each clip, and lays everything out as one picture, each with its source and
        licence from the manifest (cav ref get fills the folder). Items keep their id and
        take line when files come and go. It lists the items that have no take line yet.
        The page the person reviews is cav review moodboard.json.
  cav board mood take <id> "what to take from it" [--dir moodboard] [--file moodboard.json]
        writes the item's "take from it" line: the one thing to take from this reference
        (a colour, a rhythm, a layout), in plain words. Editing the file by hand works too.
  cav board mood drop <id>
        takes an item off the board. The file stays, and cav never brings the item back.
        A renamed file counts as one gone and one new item.

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
	case "motion":
		return boardMotion(a, args)
	case "sheet":
		return boardSheet(a, args)
	case "animatic":
		return boardAnimatic(a, args)
	case "place":
		return boardPlace(a, args)
	case "mood":
		return boardMood(a, args)
	}
	return usageErr("unknown board command %q (init, frames, motion, sheet, animatic, place, mood)", sub)
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
	briefPath, noBrief := briefFlags(fs)
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil && !*force {
		return fail(exitError, *out+" already exists", "edit it, or pass --force to start over")
	}
	b, err := briefFor(fs, *briefPath, *noBrief)
	if err != nil {
		return err
	}
	p := compParams{W: *width, H: *height, FPS: *fps, Secs: *seconds, BPM: *bpm}
	use := applyBrief(&p, b, *briefPath, func(key string) string {
		return flagsGiven(fs, map[string][]string{"size": {"width", "height"}, "fps": {"fps"}, "seconds": {"seconds"}, "bpm": {"bpm"}}[key])
	}, true)
	title := ""
	if b != nil {
		title = b.Title
		if title != "" {
			use.Took["title"] = title
		}
		if b.BPM == 0 && !flagSet(fs, "bpm") {
			use.Notes = append(use.Notes, fmt.Sprintf("%s has no bpm; using %g. Set --bpm or the brief's bpm when there is music", *briefPath, *bpm))
		}
	}
	*width, *height, *fps, *seconds, *bpm = p.W, p.H, p.FPS, p.Secs, p.BPM
	if *shots < 1 || *bpm <= 0 || *seconds <= 0 {
		return usageErr("--shots, --bpm and --seconds must be positive")
	}
	total := math.Floor(*seconds * *bpm / 60)
	if total < 1 {
		return fail(exitError, fmt.Sprintf("%g s at %g bpm is less than one beat", *seconds, *bpm), "make the piece longer, or raise --bpm")
	}
	sb := &board.Board{Title: title, BPM: *bpm, FPS: *fps, Seconds: *seconds, Width: *width, Height: *height,
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
		sb.Shots = append(sb.Shots, board.Shot{ID: fmt.Sprintf("s%d", i+1), Beats: [2]float64{start, end}, What: shotPlaceholder, Why: whyPlaceholder})
		start = end
	}
	if err := sb.Save(*out); err != nil {
		return err
	}
	data := map[string]any{"file": *out, "shots": len(sb.Shots), "beats": total}
	if use != nil {
		data["fromBrief"] = use
	}
	a.emit(data, func() {
		use.say(os.Stdout)
		fmt.Printf("wrote %s: %d shots over %g beats (%g s at %g bpm)\nEdit \"what\" and \"why\" for each shot, then: cav board frames\n", *out, len(sb.Shots), total, *seconds, *bpm)
	})
	return nil
}

// aspectOf picks the standard aspect ratio closest to w:h.
func aspectOf(w, h int) string { return board.AspectLabel(w, h) }

// shotPlaceholder is the "what" that cav board init writes. A shot that still has it is not described yet.
const shotPlaceholder = "describe what happens in this shot"

// whyPlaceholder is the "why" that cav board init writes. cav check warns about a shot that still has it.
// Unlike shotPlaceholder, it never stops cav board frames or motion: the image service does not use it.
const whyPlaceholder = "why the viewer needs this shot"

// missingWhy says whether a shot has no real reason yet: empty, or still the placeholder.
func missingWhy(s board.Shot) bool {
	w := strings.TrimSpace(s.Why)
	return w == "" || w == whyPlaceholder
}

// noTextRule keeps the image service from drawing words the shot did not ask for.
const noTextRule = "Do not draw any text, labels or captions unless the shot description asks for them."

// placeholderShots names the shots in the set that still have the placeholder text.
// pick says whether a shot is going to be sent to a paid service.
func placeholderShots(sb *board.Board, pick func(board.Shot) bool) error {
	var ids []string
	for _, s := range sb.Shots {
		if pick(s) && strings.TrimSpace(s.What) == shotPlaceholder {
			ids = append(ids, s.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return fail(exitError, strings.Join(ids, ", ")+": write what happens in the shot first (storyboard.json \"what\")",
		"edit \"what\" for these shots; --force does not skip this check")
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
	b.WriteString("Show one clear key moment. Keep the same style as the other frames of this storyboard. " + noTextRule)
	return b.String()
}

// frameCurrent says whether the shot's frame stands as it is. A frame that is missing is not
// current. When sketches are being drawn and the shot has an SVG, a greybox card is not
// current (the sketch replaces it), and nor is a sketch frame older than its SVG. A frame
// from any other source stands: --force remakes it.
func frameCurrent(sb *board.Board, s board.Shot, svgAbs string, sketching bool) bool {
	if s.Frame == "" {
		return false
	}
	png, err := os.Stat(sb.Path(s.Frame))
	if err != nil {
		return false
	}
	if !sketching {
		return true
	}
	svg, err := os.Stat(svgAbs)
	if err != nil {
		return true
	}
	switch s.Source {
	case "greybox":
		return false
	case "sketch":
		return !svg.ModTime().After(png.ModTime())
	}
	return true
}

// sketchFiles gives the SVG of a shot: its path inside the board folder, and absolute.
func sketchFiles(sb *board.Board, dir, id string) (rel, abs string) {
	rel = filepath.ToSlash(filepath.Join(dir, id+".svg"))
	return rel, sb.Path(rel)
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// withSketch adds sketch to an image order just before greybox, or at the end when the order
// has no greybox. Every other service keeps its place, so a free choice stays ahead of a paid one.
func withSketch(order []string) []string {
	out := make([]string, 0, len(order)+1)
	added := false
	for _, n := range order {
		if n == "sketch" {
			return append([]string(nil), order...)
		}
		if n == "greybox" && !added {
			out = append(out, "sketch")
			added = true
		}
		out = append(out, n)
	}
	if !added {
		out = append(out, "sketch")
	}
	return out
}

func boardFrames(a *app, args []string) (err error) {
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
	want := map[string]bool{}
	for _, id := range splitList(*only) {
		want[id] = true
	}
	hasSVG := func(s board.Shot) bool {
		_, abs := sketchFiles(sb, *dir, s.ID)
		return fileExists(abs)
	}
	// needsFrame says whether the run makes a frame for the shot.
	needsFrame := func(s board.Shot, sketching bool) bool {
		if len(want) > 0 && !want[s.ID] {
			return false
		}
		_, abs := sketchFiles(sb, *dir, s.ID)
		return *force || !frameCurrent(sb, s, abs, sketching)
	}
	// Refuse before any service call: a placeholder "what" makes the image service invent content.
	// A shot with an SVG may be drawn as a sketch, which does not use "what".
	maybeSketch := *service == "" || *service == "sketch"
	if err := placeholderShots(sb, func(s board.Shot) bool {
		return needsFrame(s, maybeSketch) && !(maybeSketch && hasSVG(s))
	}); err != nil {
		return err
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour) // a whole board; local models also download on their first run
	defer cancel()
	ch, err := services.Pick(ctx, c, "image", *service)
	if err != nil {
		return fail(exitError, err.Error(), "see cav config")
	}
	explicitSketch := *service == "sketch"
	sketching := ch.Service == "sketch"
	// The sketches to draw, each checked now: one bad SVG stops the run before any frame is made.
	sketchIDs := map[string]bool{}
	var needSketch []string
	if sketching {
		var missing, problems []string
		for _, s := range sb.Shots {
			if !needsFrame(s, true) {
				continue
			}
			rel, abs := sketchFiles(sb, *dir, s.ID)
			if !fileExists(abs) {
				missing = append(missing, s.ID)
				continue
			}
			sketchIDs[s.ID] = true
			raw, err := board.ReadSketch(abs)
			if err == nil {
				err = board.CheckSketch(raw, sb.Width, sb.Height)
			}
			var se *board.SketchError
			switch {
			case errors.As(err, &se):
				problems = append(problems, rel+": "+strings.Join(se.Problems, "; "))
			case err != nil:
				problems = append(problems, rel+": "+err.Error())
			}
		}
		if len(problems) > 0 {
			return fail(exitError, strings.Join(problems, "\n"), "a sketch uses simple shapes only: cav guide production (Sketch frames)")
		}
		if explicitSketch && len(missing) > 0 {
			files := make([]string, len(missing))
			for i, id := range missing {
				files[i], _ = sketchFiles(sb, *dir, id)
			}
			return fail(exitError, strings.Join(missing, ", ")+": write "+strings.Join(files, ", ")+" first",
				"a sketch is an SVG of simple shapes drawn by you: cav guide production (Sketch frames)")
		}
		needSketch = missing
	}
	if err := placeholderShots(sb, func(s board.Shot) bool { return needsFrame(s, sketching) && !sketchIDs[s.ID] }); err != nil {
		return err
	}
	var refs []string
	for _, r := range sb.Style.Refs {
		refs = append(refs, sb.Path(r))
	}
	root := library.Root()
	// A broken manifest stops the run before the first paid call.
	if _, err := library.Load(root); err != nil {
		return err
	}
	var notes []string
	// A free run that picked greybox does not know about the SVGs unless sketch is in the order.
	if ch.Service == "greybox" && *service == "" {
		var withSVG []string
		for _, s := range sb.Shots {
			if (len(want) == 0 || want[s.ID]) && hasSVG(s) {
				withSVG = append(withSVG, s.ID+".svg")
			}
		}
		if len(withSVG) > 0 {
			notes = append(notes, fmt.Sprintf("%s in %s exist but the image order has no sketch: cav config order image %s",
				strings.Join(withSVG, ", "), *dir, strings.Join(withSketch(c.OrderFor("image")), ",")))
		}
	}
	var sess *sketchSession
	closeSketch := func() error { return sess.close() }
	defer func() {
		if cerr := closeSketch(); cerr != nil {
			if err == nil {
				err = cerr
			} else {
				fmt.Fprintln(os.Stderr, "error: "+cerr.Error())
			}
		}
	}()
	if len(sketchIDs) > 0 {
		var serr error
		sess, serr = startSketch(a, sb, sb.Path(*dir))
		var un *sketchUnavailable
		switch {
		case errors.As(serr, &un) && !explicitSketch:
			// Cavalry is not open: the shots with an SVG get cards, like the others.
			notes = append(notes, "sketch: Cavalry is not answering; open Cavalry with the cav bridge, then run cav board frames again")
			for id := range sketchIDs {
				needSketch = append(needSketch, id)
			}
			sketchIDs, sketching = map[string]bool{}, false
			if err := placeholderShots(sb, func(s board.Shot) bool { return needsFrame(s, false) }); err != nil {
				return err
			}
		case serr != nil:
			return serr
		}
	}
	// The service that draws the cards: with no sketch to draw, a sketch run is a greybox run.
	svc := ch.Service
	if svc == "sketch" && len(sketchIDs) == 0 {
		svc = "greybox"
	}
	type made struct {
		Shot   string `json:"shot"`
		Frame  string `json:"frame"`
		Source string `json:"source"`
		Kept   bool   `json:"kept,omitempty"`
		// DroppedClip is the clip made from the old frame, no longer used by the animatic.
		DroppedClip string `json:"droppedClip,omitempty"`
	}
	var done []made
	for i := range sb.Shots {
		s := &sb.Shots[i]
		if len(want) > 0 && !want[s.ID] {
			continue
		}
		svgRel, svgAbs := sketchFiles(sb, *dir, s.ID)
		if !*force && frameCurrent(sb, *s, svgAbs, sketching) {
			done = append(done, made{Shot: s.ID, Frame: s.Frame, Source: s.Source, Kept: true})
			continue
		}
		var im *imagegen.Image
		prompt := framePrompt(sb, *s)
		switch {
		case sketchIDs[s.ID]:
			fmt.Fprintf(os.Stderr, "%s: drawing %s through Cavalry…\n", s.ID, svgRel)
			im, err = sess.render(s.ID, svgAbs)
			if err != nil {
				return fail(exitError, s.ID+": "+err.Error(), "the frames made so far are saved; fix the problem and run again to continue")
			}
		case svc == "greybox" || svc == "sketch":
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
		default:
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
		// A clip starts from the frame it was made from: a new frame makes it stale.
		dropped := s.Clip
		s.Clip = ""
		// Save after every frame, so an interrupted run keeps what it made.
		if err := sb.Save(path); err != nil {
			return err
		}
		if im.Provider != "greybox" {
			sum, _ := fileSHA(out)
			e := library.Entry{Path: out, Kind: "image", Title: "storyboard " + s.ID, Source: im.Provider,
				Licence: "generated", CommercialOK: true, RetrievedAt: time.Now().UTC(), SHA256: sum,
				Generated: &library.Generated{Provider: im.Provider, Model: im.Model, Prompt: prompt, Refs: sb.Style.Refs}}
			if im.Provider == "sketch" {
				// The picture comes from the SVG, not from a prompt.
				e.Title = "storyboard " + s.ID + " (sketch)"
				e.Generated = &library.Generated{Provider: im.Provider, Model: im.Model, Refs: []string{svgRel}}
			}
			if err := library.Update(root, func(m *library.Manifest) error {
				m.Put(root, e)
				return nil
			}); err != nil {
				return err
			}
		}
		done = append(done, made{Shot: s.ID, Frame: rel, Source: im.Provider, DroppedClip: dropped})
	}
	// Give Cavalry back before reporting.
	if err := closeSketch(); err != nil {
		return err
	}
	data := map[string]any{"storyboard": path, "service": svc, "skipped": ch.Skipped, "frames": done}
	if len(needSketch) > 0 && !explicitSketch {
		sort.Strings(needSketch)
		files := make([]string, len(needSketch))
		for i, id := range needSketch {
			files[i], _ = sketchFiles(sb, *dir, id)
		}
		notes = append(notes, "write "+strings.Join(files, ", ")+" for sketch frames instead of text cards (cav guide production, Sketch frames), then run cav board frames again")
		data["needSketch"] = needSketch
	}
	if len(notes) > 0 {
		data["sketchNote"] = strings.Join(notes, "; ")
	}
	a.emit(data, func() {
		for _, s := range ch.Skipped {
			fmt.Fprintln(os.Stderr, "note: skipped "+s)
		}
		for _, n := range notes {
			fmt.Fprintln(os.Stderr, "note: "+n)
		}
		for _, d := range done {
			state := "made with " + d.Source
			if d.DroppedClip != "" {
				state += "; its old clip " + d.DroppedClip + " is no longer used: cav board motion --only " + d.Shot
			}
			if d.Kept {
				state = "kept"
			}
			fmt.Printf("%-4s %s (%s)\n", d.Shot, d.Frame, state)
		}
		fmt.Println("Look at them together: cav board sheet")
	})
	return nil
}

// motionPrompt asks for movement that starts from the shot's frame.
func motionPrompt(sb *board.Board, s board.Shot) string {
	var b strings.Builder
	b.WriteString("Animate this storyboard frame of a motion-graphics piece. Start exactly on the frame and keep its look, layout and colours. ")
	fmt.Fprintf(&b, "This shot: %s. ", s.What)
	if s.Camera != "" {
		fmt.Fprintf(&b, "Camera: %s. ", s.Camera)
	} else {
		b.WriteString("Camera: locked off unless the shot says otherwise. ")
	}
	if s.Prompt != "" {
		b.WriteString(s.Prompt + ". ")
	}
	if sb.Style.Prompt != "" {
		fmt.Fprintf(&b, "Look: %s. ", sb.Style.Prompt)
	}
	b.WriteString("Smooth, deliberate motion. No new text, no captions, no logos.")
	return b.String()
}

func boardMotion(a *app, args []string) error {
	fs := flag.NewFlagSet("board motion", flag.ContinueOnError)
	service := fs.String("service", "", "video service (default: the video order in cav config)")
	only := fs.String("only", "", "comma-separated shot ids")
	all := fs.Bool("all", false, "every shot")
	force := fs.Bool("force", false, "remake clips that exist")
	model := fs.String("model", "", "model (default: cav config model veo, or Veo 3.1 Lite)")
	res := fs.String("resolution", "720p", "720p, or 1080p (8-second clips only)")
	dir := fs.String("dir", "board", "folder for the clips, next to the storyboard")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if *only == "" && !*all {
		return usageErr("name the shots to animate with --only s2,s4, or pass --all (each clip costs money)")
	}
	path := boardPath(pos, "storyboard.json")
	sb, err := board.Load(path)
	if err != nil {
		return fail(exitError, err.Error(), "cav board init writes a starting storyboard")
	}
	want := map[string]bool{}
	for _, id := range splitList(*only) {
		want[id] = true
	}
	for id := range want {
		found := false
		for _, s := range sb.Shots {
			found = found || s.ID == id
		}
		if !found {
			return usageErr("no shot is called %s", id)
		}
	}
	if err := placeholderShots(sb, func(s board.Shot) bool {
		if !*all && !want[s.ID] {
			return false
		}
		if s.Clip != "" && !*force {
			if _, err := os.Stat(sb.Path(s.Clip)); err == nil {
				return false
			}
		}
		return true
	}); err != nil {
		return err
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	ch, err := services.Pick(ctx, c, "video", *service)
	if err != nil {
		return fail(exitError, err.Error(), "Veo uses the Gemini key: cav config set-key gemini <source>")
	}
	aspect := "16:9"
	if sb.Height > sb.Width {
		aspect = "9:16"
	}
	root := library.Root()
	// A broken manifest stops the run before the first paid call.
	if _, err := library.Load(root); err != nil {
		return err
	}
	type made struct {
		Shot    string `json:"shot"`
		Clip    string `json:"clip"`
		Seconds int    `json:"seconds,omitempty"`
		Kept    bool   `json:"kept,omitempty"`
	}
	var done []made
	for i := range sb.Shots {
		s := &sb.Shots[i]
		if !*all && !want[s.ID] {
			continue
		}
		if s.Clip != "" && !*force {
			if _, err := os.Stat(sb.Path(s.Clip)); err == nil {
				done = append(done, made{Shot: s.ID, Clip: s.Clip, Kept: true})
				continue
			}
		}
		if s.Frame == "" {
			return fail(exitError, s.ID+" has no frame yet", "cav board frames makes the frame the clip starts from")
		}
		length := sb.Time(s.Beats[1]) - sb.Time(s.Beats[0])
		prompt := motionPrompt(sb, *s)
		fmt.Fprintf(os.Stderr, "%s: animating with %s (%d s clip for a %.1f s shot)…\n", s.ID, ch.Service, videogen.Duration(length), board.RoundSeconds(length, 1))
		cl, err := videogen.Generate(ctx, c, ch, videogen.Request{Prompt: prompt, Image: sb.Path(s.Frame), Aspect: aspect,
			Seconds: length, Resolution: *res, Model: *model})
		if err != nil {
			return fail(exitError, s.ID+": "+err.Error(), "the clips made so far are saved; run again to continue")
		}
		rel := filepath.ToSlash(filepath.Join(*dir, s.ID+".mp4"))
		out := sb.Path(rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, cl.Data, 0o644); err != nil {
			return err
		}
		s.Clip = rel
		sum, _ := fileSHA(out)
		e := library.Entry{Path: out, Kind: "video", Title: "storyboard " + s.ID + " (moving)", Source: cl.Provider,
			Licence: "generated", CommercialOK: true, RetrievedAt: time.Now().UTC(), SHA256: sum,
			Generated: &library.Generated{Provider: cl.Provider, Model: cl.Model, Prompt: prompt, Refs: []string{s.Frame}}}
		// Save after every clip, so an interrupted run keeps what it made.
		if err := sb.Save(path); err != nil {
			return err
		}
		if err := library.Update(root, func(m *library.Manifest) error {
			m.Put(root, e)
			return nil
		}); err != nil {
			return err
		}
		done = append(done, made{Shot: s.ID, Clip: rel, Seconds: cl.Seconds})
	}
	a.emit(map[string]any{"storyboard": path, "service": ch.Service, "clips": done}, func() {
		for _, d := range done {
			state := fmt.Sprintf("%d s, made with %s", d.Seconds, ch.Service)
			if d.Kept {
				state = "kept"
			}
			fmt.Printf("%-4s %s (%s)\n", d.Shot, d.Clip, state)
		}
		fmt.Println("See them in place: cav board animatic")
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

func boardPlace(a *app, args []string) error {
	fs := flag.NewFlagSet("board place", flag.ContinueOnError)
	only := fs.String("only", "", "comma-separated shot ids")
	comp := fs.String("comp", "", "composition ID or unique name (default: the active one)")
	timeout := fs.Duration("timeout", 2*time.Minute, "maximum wait for Cavalry")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	path := boardPath(pos, "storyboard.json")
	sb, err := board.Load(path)
	if err != nil {
		return fail(exitError, err.Error(), "cav board init writes a starting storyboard")
	}
	want := map[string]bool{}
	for _, id := range splitList(*only) {
		want[id] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	ps, err := sb.Placements(ctx, want)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	shots, err := json.Marshal(ps)
	if err != nil {
		return err
	}
	// One job reads the composition and places the frames, so nothing can change in between.
	a.compSelector = *comp
	o, err := a.execJS("var shots = "+string(shots)+";\n"+assets.BoardPlace, execOpts{helpers: true, timeout: *timeout, source: "cav board place"})
	if err != nil {
		return err
	}
	if !o.result.OK {
		return scriptErr(o)
	}
	type row struct {
		Shot    string  `json:"shot"`
		Layer   string  `json:"layer"`
		In      int     `json:"in"`
		Out     int     `json:"out"`
		Scale   float64 `json:"scale,omitempty"` // left out when an existing layer keeps its scale
		Updated bool    `json:"updated,omitempty"`
	}
	var r struct {
		Comp struct {
			FPS   float64 `json:"fps"`
			Start int     `json:"start"`
			End   int     `json:"end"`
		} `json:"comp"`
		Placed []struct {
			ID string `json:"id"`
			row
		} `json:"placed"`
		Skipped []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"` // "built", "before" or "short"
			Layer  string `json:"layer,omitempty"`
			Type   string `json:"type,omitempty"`
		} `json:"skipped"`
		Last int `json:"last"` // the last frame any shot needs, placed or skipped
	}
	if err := json.Unmarshal(o.result.Value, &r); err != nil {
		return err
	}
	notes := []string{}
	for _, p := range ps {
		if p.Width == 0 {
			notes = append(notes, p.ID+": unknown image size, placed at its native size")
		}
	}
	rows := []row{}
	for _, p := range r.Placed {
		x := p.row
		x.Shot, x.Scale = p.ID, math.Round(x.Scale*1000)/1000
		rows = append(rows, x)
	}
	if last := r.Last; last > r.Comp.End {
		// --range keeps the start frame; --seconds would move it back to 0 under the placed layers.
		fix := fmt.Sprintf("extend it with cav scene comp --range %d-%d", r.Comp.Start, last)
		if *comp != "" {
			// cav scene comp changes only the active composition.
			fix = fmt.Sprintf("make %s the active composition in Cavalry, then %s", *comp, fix)
		}
		notes = append(notes, fmt.Sprintf("the composition ends at frame %d but the last shot runs to frame %d: %s", r.Comp.End, last, fix))
	}
	a.emit(map[string]any{"storyboard": path, "fps": r.Comp.FPS, "placed": rows, "skipped": r.Skipped, "notes": notes}, func() {
		for _, x := range rows {
			state, scale := "added", fmt.Sprintf("scale %g", x.Scale)
			if x.Updated {
				state, scale = "updated", "scale kept"
			} else if x.Scale == 0 {
				scale = "native size"
			}
			fmt.Printf("%-4s %-16s frames %d-%d  %s  (%s)\n", x.Shot, x.Layer, x.In, x.Out, scale, state)
		}
		for _, s := range r.Skipped {
			why := map[string]string{"before": "it ends before the composition starts", "short": "it is shorter than one frame"}[s.Reason]
			switch {
			case why == "":
				fmt.Printf("%-4s kept %s (a %s already uses the name)\n", s.ID, s.Layer, s.Type)
			case s.Layer != "":
				fmt.Printf("%-4s skipped: %s (hid %s)\n", s.ID, why, s.Layer)
			default:
				fmt.Printf("%-4s skipped: %s\n", s.ID, why)
			}
		}
		for _, n := range notes {
			fmt.Fprintln(os.Stderr, "note: "+n)
		}
	})
	return nil
}
