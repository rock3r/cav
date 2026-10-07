package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/beats"
	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/listen"
	"github.com/rock3r/cav/internal/music"
	"github.com/rock3r/cav/internal/services"
)

func init() {
	register(command{
		name:    "listen",
		args:    "<track> [--brief file|text] [--board storyboard.json] [--seconds N] [--fps 60] [--bpm hint] [--ears S | --no-ears] [-o picture.png]",
		summary: "Judge a track without ears: loudness, structure, cuts against the beat, a picture, and an audio model's critique.",
		run:     cmdListen,
	})
	longHelp["listen"] = `
cav listen music/take2.mp3 --brief brief.md --board storyboard.json
reports, for one track:
  - tempo, length (against --seconds or the storyboard), sections and big moments
    (the same analysis as cav beats);
  - loudness: integrated LUFS, true peak and range (EBU R128 through ffmpeg), with advice
    for web delivery (about -14 LUFS, true peak at most -1 dBFS);
  - with --board, where each shot starts against the nearest downbeat, in frames;
  - a picture (the cav spectrogram view, with the shot cuts drawn green on a downbeat and
    red off it). Look at it;
  - a written critique from an audio model (the "ears" job in cav config: Gemini, or
    Qwen3-Omni on a server you set). --brief is what the music should do. --no-ears skips
    it; without a key it is skipped and the rest still runs.
Use it to compare takes from cav music gen and to check a track before cav render --audio.
The numbers and the picture are measurements; the critique is a model's opinion.`
	register(command{
		name:    "music",
		args:    "gen \"<prompt>\" [--seconds 30 | --board storyboard.json] [--takes 2] [--service S] [--model M] [-o music/]",
		summary: "Generate instrumental music with an API (ElevenLabs Music, Stable Audio), planned on the storyboard.",
		run:     cmdMusic,
	})
	longHelp["music"] = `
cav music gen "warm synthwave, confident, builds to a drop" --board storyboard.json --takes 2
    makes takes in music/, recorded in .cav/manifest.json. With --board, each shot
    becomes a section of the same length (ElevenLabs composition plan; Stable Audio gets
    the structure in its prompt), so changes in the music fall on the shot cuts. Without
    --board, --seconds sets the length.
Then compare the takes: cav listen music/<take>.mp3 --board storyboard.json --brief "...".
Needs a key for the "music" job (cav config). Check the service's terms for your use:
ElevenLabs self-serve plans, for example, exclude film, TV and games.`
}

type cutCheck struct {
	Shot   string  `json:"shot"`
	Time   float64 `json:"time"`
	Frame  int     `json:"frame"`
	Offset int     `json:"offsetFrames"` // from the nearest downbeat; minus = early
	OnBeat bool    `json:"onDownbeat"`
}

func readBrief(s string) string {
	if s == "" {
		return ""
	}
	if b, err := os.ReadFile(s); err == nil {
		return strings.TrimSpace(string(b))
	}
	return s
}

func cmdListen(a *app, args []string) error {
	fs := flag.NewFlagSet("listen", flag.ContinueOnError)
	brief := fs.String("brief", "", "what the music should do: a file or the text itself")
	boardFile := fs.String("board", "", "storyboard.json, to check the shot cuts against the beat")
	seconds := fs.Float64("seconds", 0, "the length the piece needs")
	fps := fs.Float64("fps", 0, "frame rate for frame numbers (default: the storyboard's, or 60)")
	bpm := fs.Float64("bpm", 0, "tempo hint")
	ears := fs.String("ears", "", "audio model to ask (default: the ears order in cav config)")
	noEars := fs.Bool("no-ears", false, "skip the audio model")
	out := fs.String("o", "", "picture (default: renders/listen-<track>.png)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav listen <track> [--brief ...] [--board storyboard.json]")
	}
	track := pos[0]
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var sb *board.Board
	if *boardFile != "" {
		if sb, err = board.Load(*boardFile); err != nil {
			return fail(exitError, err.Error(), "")
		}
		if *seconds == 0 {
			*seconds = sb.Seconds
		}
		if *fps == 0 {
			*fps = sb.FPS
		}
		if *bpm == 0 {
			*bpm = sb.BPM
		}
	}
	if *fps == 0 {
		*fps = 60
	}
	samples, err := beats.Decode(track)
	if err != nil {
		return fail(exitError, err.Error(), ffmpegFix())
	}
	r := beats.Analyze(samples, *bpm)
	prof := beats.NewProfile(samples)
	st := prof.Describe(r)
	loud, lerr := listen.Measure(ctx, track)

	var cuts []cutCheck
	var marks []beats.Mark
	if sb != nil && len(r.Downbeats) > 1 {
		for _, s := range sb.Shots {
			t := sb.Time(s.Beats[0])
			best := r.Downbeats[0]
			for _, d := range r.Downbeats {
				if math.Abs(d-t) < math.Abs(best-t) {
					best = d
				}
			}
			off := int(math.Round((t - best) * *fps))
			ok := math.Abs(t-best) <= 0.035
			cuts = append(cuts, cutCheck{Shot: s.ID, Time: t, Frame: int(math.Round(t * *fps)), Offset: off, OnBeat: ok})
			marks = append(marks, beats.Mark{Time: t, OK: ok})
		}
	}
	if *out == "" {
		*out = filepath.Join(outDir(), "listen-"+strings.TrimSuffix(filepath.Base(track), filepath.Ext(track))+".png")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if _, _, err := beats.Picture(prof, beats.View{Width: 1600, FPS: *fps, Grid: r, Structure: &st, Cuts: marks}, *out); err != nil {
		return err
	}
	var advice []string
	if lerr == nil {
		advice = loud.Advice()
	}
	if *seconds > 0 {
		d := prof.Duration - *seconds
		switch {
		case d < -0.05:
			advice = append(advice, fmt.Sprintf("too short: %.2f s for a %.2f s piece (%.2f s missing)", prof.Duration, *seconds, -d))
		case d > 1:
			advice = append(advice, fmt.Sprintf("%.2f s longer than the piece: trim or fade it at %.2f s, ideally on a downbeat", d, *seconds))
		}
	}
	for _, c := range cuts {
		if !c.OnBeat {
			advice = append(advice, fmt.Sprintf("%s starts %+d frames from the nearest downbeat", c.Shot, c.Offset))
		}
	}

	facts := fmt.Sprintf("length %.2f s; tempo %.1f BPM; %d sections starting at %s", prof.Duration, r.BPM, len(st.Sections), sectionStarts(st))
	if lerr == nil {
		facts += fmt.Sprintf("; loudness %.1f LUFS integrated, true peak %.1f dBFS, range %.1f LU", loud.Integrated, loud.TruePeak, loud.Range)
	}
	if sb != nil {
		var sh []string
		for _, s := range sb.Shots {
			sh = append(sh, fmt.Sprintf("%s at %.2fs: %s", s.ID, sb.Time(s.Beats[0]), s.What))
		}
		facts += "\nThe picture cuts to: " + strings.Join(sh, "; ")
	}
	critique, earsBy, earsNote := "", "", ""
	if !*noEars {
		critique, earsBy, earsNote = askEars(ctx, track, *ears, listen.Prompt(readBrief(*brief), facts))
	}

	abs, _ := filepath.Abs(*out)
	data := map[string]any{"track": track, "duration": prof.Duration, "bpm": r.BPM, "confidence": r.Confidence,
		"sections": st.Sections, "events": st.Events, "picture": abs, "cuts": cuts, "advice": advice}
	if lerr == nil {
		data["loudness"] = loud
	} else {
		data["loudnessError"] = lerr.Error()
	}
	if critique != "" {
		data["critique"] = map[string]string{"by": earsBy, "text": critique}
	} else if earsNote != "" {
		data["earsSkipped"] = earsNote
	}
	a.emit(data, func() {
		fmt.Printf("%s: %.2f s, %.1f BPM (confidence %.2f), %d sections\n", track, prof.Duration, r.BPM, r.Confidence, len(st.Sections))
		if lerr == nil {
			fmt.Printf("loudness: %.1f LUFS integrated, true peak %.1f dBFS, range %.1f LU\n", loud.Integrated, loud.TruePeak, loud.Range)
		} else {
			fmt.Printf("loudness: not measured (%v)\n", lerr)
		}
		for _, c := range cuts {
			mark := "on the downbeat"
			if !c.OnBeat {
				mark = fmt.Sprintf("%+d frames off", c.Offset)
			}
			fmt.Printf("cut %-4s %6.2fs  frame %-5d %s\n", c.Shot, c.Time, c.Frame, mark)
		}
		for _, adv := range advice {
			fmt.Println("note: " + adv)
		}
		fmt.Printf("picture: %s (look at it)\n", abs)
		if critique != "" {
			fmt.Printf("\ncritique from %s (a model's opinion, not a measurement):\n%s\n", earsBy, critique)
		} else if earsNote != "" {
			fmt.Println("critique: skipped (" + earsNote + ")")
		}
	})
	return nil
}

func sectionStarts(st beats.Structure) string {
	var s []string
	for _, sec := range st.Sections {
		s = append(s, fmt.Sprintf("%.1fs (%s)", sec.Start, sec.Level))
	}
	return strings.Join(s, ", ")
}

// askEars returns the critique and who wrote it, or a note saying why there is none.
func askEars(ctx context.Context, track, only, prompt string) (string, string, string) {
	c, err := services.Load()
	if err != nil {
		return "", "", err.Error()
	}
	ch, err := services.Pick(ctx, c, "ears", only)
	if err != nil {
		return "", "", "no audio model: add a Gemini key or a qwen-omni server (cav config)"
	}
	audio, err := listen.Compact(ctx, track)
	if err != nil {
		return "", "", err.Error()
	}
	text, err := listen.Critique(ctx, c, ch, audio, prompt)
	if err != nil {
		return "", "", err.Error()
	}
	return text, ch.Service, ""
}

func cmdMusic(a *app, args []string) error {
	if len(args) == 0 || args[0] != "gen" {
		printHelp("music")
		if len(args) == 0 {
			return nil
		}
		return usageErr("unknown music command %q (gen)", args[0])
	}
	fs := flag.NewFlagSet("music gen", flag.ContinueOnError)
	seconds := fs.Float64("seconds", 30, "length when there is no storyboard")
	boardFile := fs.String("board", "", "storyboard.json: one section per shot")
	takes := fs.Int("takes", 1, "how many versions to make")
	service := fs.String("service", "", "music service")
	model := fs.String("model", "", "model")
	outDirFlag := fs.String("o", "music", "folder for the takes")
	pos, err := parseFlags(fs, args[1:])
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageErr("give a prompt: genre, instruments, mood, tempo")
	}
	prompt := strings.Join(pos, " ")
	req := music.Request{Prompt: prompt, Seconds: *seconds, Model: *model}
	if *boardFile != "" {
		sb, err := board.Load(*boardFile)
		if err != nil {
			return fail(exitError, err.Error(), "")
		}
		req.Seconds = sb.Seconds
		if sb.BPM > 0 {
			req.Prompt += fmt.Sprintf(", %g BPM", sb.BPM)
		}
		// One section per shot. The first also covers any lead-in before it, the last any
		// tail after it, so the sections add up to the piece's length.
		for i, s := range sb.Shots {
			start, end := sb.Time(s.Beats[0]), sb.Time(s.Beats[1])
			if i == 0 {
				start = 0
			}
			if i == len(sb.Shots)-1 && sb.Seconds > end {
				end = sb.Seconds
			}
			if i+1 < len(sb.Shots) {
				end = sb.Time(sb.Shots[i+1].Beats[0])
			}
			name := s.ID + ": " + s.What
			if len(name) > 100 {
				name = name[:100]
			}
			var styles []string
			if s.Prompt != "" {
				styles = append(styles, s.Prompt)
			}
			req.Sections = append(req.Sections, music.Section{Name: name, Seconds: end - start, Styles: styles})
		}
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	ch, err := services.Pick(ctx, c, "music", *service)
	if err != nil {
		return fail(exitError, err.Error(), "add an ElevenLabs or Stability key (cav config); or use cav sfx search to find music")
	}
	root := library.Root()
	var made []string
	var errs []error
	for i := 1; i <= *takes; i++ {
		fmt.Fprintf(os.Stderr, "take %d of %d with %s…\n", i, *takes, ch.Service)
		tr, err := music.Generate(ctx, c, ch, req)
		if err != nil {
			errs = append(errs, err)
			break
		}
		name := safeSlug(prompt)
		if *takes > 1 {
			name += fmt.Sprintf("-take%d", i)
		}
		path := filepath.Join(*outDirFlag, name+tr.Ext)
		if err := os.MkdirAll(*outDirFlag, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, tr.Data, 0o644); err != nil {
			return err
		}
		m, err := library.Load(root)
		if err != nil {
			return err
		}
		sum, _ := fileSHA(path)
		m.Put(root, library.Entry{Path: path, Kind: "audio", Source: tr.Provider, Licence: "generated", CommercialOK: true,
			RetrievedAt: time.Now().UTC(), SHA256: sum, Generated: &library.Generated{Provider: tr.Provider, Model: tr.Model, Prompt: req.Prompt}})
		if err := m.Save(root); err != nil {
			return err
		}
		made = append(made, path)
	}
	if len(made) == 0 {
		return fail(exitError, errors.Join(errs...).Error(), "")
	}
	a.emit(map[string]any{"takes": made, "service": ch.Service, "sections": len(req.Sections)}, func() {
		for _, p := range made {
			fmt.Println(p)
		}
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "error: "+e.Error())
		}
		board := ""
		if *boardFile != "" {
			board = " --board " + *boardFile
		}
		fmt.Printf("Compare them: cav listen <take>%s --brief \"...\"\n", board)
	})
	return nil
}
