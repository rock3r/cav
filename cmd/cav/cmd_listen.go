package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"sort"

	"errors"
	"flag"
	"fmt"
	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/pyrun"
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
		args:    "<track>... [--brief file|text] [--board storyboard.json] [--score] [--seconds N] [--fps 60] [--bpm hint] [--ears S | --no-ears] [-o picture.png]",
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
--score adds scores from local models, run through uv (no key; the first run downloads
about 2 GB): Meta's Audiobox Aesthetics (production quality, enjoyment, usefulness,
complexity, 0-10) and, with --brief, how well the audio matches it in LAION CLAP's space.
Give several takes (cav listen a.mp3 b.mp3 c.mp3 --score --board storyboard.json) to put
them side by side, ranked by production quality.
Use it to compare takes from cav music gen and to check a track before cav render --audio.
The numbers and the picture are measurements; the critique is a model's opinion.`
	register(command{
		name:    "music",
		args:    "gen \"<prompt>\" [--seconds 30 | --board storyboard.json] [--ref track.mp3]... [--keep 0.4] [--raw-level] [--takes 2] [--service S] [--model M] [-o music/] | render score.json [-o out.wav] [--stems]",
		summary: "Generate instrumental music with an API, or render a written score locally.",
		run:     cmdMusic,
	})
	longHelp["music"] = `
cav music gen "warm synthwave, confident, builds to a drop" --board storyboard.json --takes 2
    makes takes in music/, recorded in .cav/manifest.json. With --board, each shot
    becomes a section of the same length (ElevenLabs composition plan; Lyria gets one
    timed "[m:ss - m:ss]" line per shot; Stable Audio gets the structure in its prompt),
    so changes in the music fall on the shot cuts. Without
    --board, --seconds sets the length.
--ref (repeatable) makes variations of a reference track: --takes takes for each one.
    Stable Audio and ACE-Step get the track itself. Stable Audio keeps its structure
    (--keep, 0-1, how much of it to keep; default 0.4); ACE-Step uses it as a style
    reference. Lyria and ElevenLabs cannot take it, so the audio model (the "ears" job)
    describes its style in words and cav adds that to the prompt: a new track in a similar
    style, not a variation of the track. cav says which way each reference went.
    Stable Audio takes a reference of 6 to 190 s in MP3 or WAV; cav converts other
    formats and sends the first 190 s of a longer track.
A take whose true peak is above -1 dBTP comes down in level (gain only) so it does not
clip; --raw-level keeps what the service sent.
What each service does differently (lengths, plans, moderation, credits): cav guide
production, "Music services".
Then compare the takes: cav listen music/<take>.mp3 --board storyboard.json --brief "...".
Needs a key for the "music" job (cav config). Check the service's terms for your use:
ElevenLabs self-serve plans, for example, exclude film, TV and games.

cav music render score.json [-o music/score.wav] [--stems]
    renders a written score locally, with no key: notes and hits on a beat grid, played
    by built-in synths and drums, your samples, or VST3/AU instruments, then mixed
    (ducking under the kick, reverb, delay, pan) and mastered to a loudness target.
    Needs uv; the first run fetches the Python packages. See cav guide production for
    the score format.`
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
	scoreIt := fs.Bool("score", false, "score the take with local models (Audiobox Aesthetics, CLAP) through uv")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageErr("usage: cav listen <track> [--brief ...] [--board storyboard.json]")
	}
	if len(pos) > 1 {
		return listenCompare(a, pos, readBrief(*brief), *boardFile, *bpm, *scoreIt)
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
	var scores *takeScore
	var scoreNote string
	if *scoreIt {
		if ts, err := scoreTakes(ctx, readBrief(*brief), []string{track}); err != nil {
			scoreNote = err.Error()
		} else if len(ts) == 1 {
			scores = &ts[0]
		}
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
	if scores != nil {
		data["scores"] = scores
	} else if scoreNote != "" {
		data["scoresSkipped"] = scoreNote
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
		if scores != nil {
			fmt.Printf("scores (local models, 0-10): enjoyment %.1f, usefulness %.1f, production quality %.1f, complexity %.1f", scores.Aesthetics["CE"], scores.Aesthetics["CU"], scores.Aesthetics["PQ"], scores.Aesthetics["PC"])
			if scores.Clap != nil {
				fmt.Printf("; match to the brief %.3f", *scores.Clap)
			}
			fmt.Println()
		} else if scoreNote != "" {
			fmt.Println("scores: skipped (" + scoreNote + ")")
		}
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
	if len(args) > 0 && args[0] == "render" {
		return cmdMusicRender(a, args[1:])
	}
	if len(args) == 0 || args[0] != "gen" {
		printHelp("music")
		if len(args) == 0 {
			return nil
		}
		return usageErr("unknown music command %q (gen, render)", args[0])
	}
	fs := flag.NewFlagSet("music gen", flag.ContinueOnError)
	seconds := fs.Float64("seconds", 30, "length when there is no storyboard")
	boardFile := fs.String("board", "", "storyboard.json: one section per shot")
	takes := fs.Int("takes", 1, "how many versions to make")
	service := fs.String("service", "", "music service")
	model := fs.String("model", "", "model")
	outDirFlag := fs.String("o", "music", "folder for the takes")
	var refs stringList
	fs.Var(&refs, "ref", "reference track to make variations of (repeatable)")
	keep := fs.Float64("keep", 0.4, "how much of the reference to keep, 0-1 (Stable Audio)")
	rawLevel := fs.Bool("raw-level", false, "keep the level the service sent, even if it clips")
	pos, err := parseFlags(fs, args[1:])
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageErr("give a prompt: genre, instruments, mood, tempo")
	}
	if *keep < 0 || *keep > 1 {
		return usageErr("--keep is between 0 and 1")
	}
	for _, r := range refs {
		if _, err := os.Stat(r); err != nil {
			return fail(exitError, "reference track: "+err.Error(), "")
		}
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
			req.Sections = append(req.Sections, music.Section{ID: s.ID, Name: name, Seconds: end - start, Styles: styles})
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
		return fail(exitError, err.Error(), "add an ElevenLabs, Stability or Gemini (Lyria) key (cav config); or use cav sfx search to find music")
	}
	// One job per reference track, or one with no reference.
	type refJob struct {
		Path        string `json:"path"`
		Mode        string `json:"mode"` // "sent" (as audio) or "described" (in words)
		Note        string `json:"note,omitempty"`
		Description string `json:"description,omitempty"`
		name        string // distinct among the references, for the take's file name
		req         music.Request
	}
	tmp, err := os.MkdirTemp("", "cav-music")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	jobs := []refJob{{req: req}}
	if len(refs) > 0 {
		jobs = nil
		names := refNames(refs)
		for i, r := range refs {
			j := refJob{Path: r, name: names[i], req: req}
			if music.SendsAudio(ch.Service) {
				send, note, err := prepareRef(ctx, ch.Service, r, tmp)
				if err != nil {
					return fail(exitError, err.Error(), "")
				}
				j.Mode, j.Note = "sent", note
				j.req.Ref, j.req.Keep = send, *keep
			} else {
				fmt.Fprintf(os.Stderr, "%s cannot take a reference track as audio; describing %s with the audio model…\n", ch.Service, r)
				desc, _, note := askEars(ctx, r, "", listen.StylePrompt)
				if desc == "" {
					return fail(exitError, "cannot describe "+r+": "+note,
						"use a service that takes audio (--service stability or acestep), or describe the reference in the prompt yourself")
				}
				j.Mode, j.Description = "described", desc
				j.req.Prompt = strings.TrimRight(req.Prompt, ". ") + ". In the style of this reference: " + desc
			}
			jobs = append(jobs, j)
		}
	}
	root := library.Root()
	var made []string
	var errs []error
	levels := map[string]string{} // take -> what cav did to its level
	for _, j := range jobs {
		for i := 1; i <= *takes; i++ {
			what := fmt.Sprintf("take %d of %d", i, *takes)
			if j.Path != "" {
				what += " from " + j.Path
			}
			fmt.Fprintf(os.Stderr, "%s with %s…\n", what, ch.Service)
			tr, err := music.Generate(ctx, c, ch, j.req)
			if err != nil {
				errs = append(errs, err)
				break
			}
			name := safeSlug(prompt)
			if len(jobs) > 1 {
				name += "-" + j.name
			}
			if *takes > 1 {
				name += fmt.Sprintf("-take%d", i)
			}
			path, note, err := saveTake(ctx, root, *outDirFlag, name, tr, j.req.Prompt, j.Path, !*rawLevel)
			if err != nil {
				return err
			}
			if note != "" {
				levels[path] = note
			}
			made = append(made, path)
		}
	}
	if len(made) == 0 {
		return fail(exitError, errors.Join(errs...).Error(), "")
	}
	out := map[string]any{"takes": made, "service": ch.Service, "sections": len(req.Sections), "levels": levels}
	if len(refs) > 0 {
		out["refs"] = jobs
	}
	a.emit(out, func() {
		for _, j := range jobs {
			switch j.Mode {
			case "sent":
				fmt.Fprintf(os.Stderr, "note: sent %s to %s as audio\n", j.Path, ch.Service)
				if j.Note != "" {
					fmt.Fprintf(os.Stderr, "note: %s: %s\n", j.Path, j.Note)
				}
			case "described":
				fmt.Fprintf(os.Stderr, "note: %s cannot take audio, so %s went in as words: %s\n", ch.Service, j.Path, j.Description)
			}
		}
		for _, p := range made {
			if n := levels[p]; n != "" {
				fmt.Fprintf(os.Stderr, "note: %s: %s\n", p, n)
			}
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

// saveTake writes one take into dir, lowers its level when it would clip (level), and
// records it in the manifest. The note says what cav did to the level.
func saveTake(ctx context.Context, root, dir, name string, tr *music.Track, prompt, ref string, level bool) (string, string, error) {
	path := filepath.Join(dir, name+tr.Ext)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, tr.Data, 0o644); err != nil {
		return "", "", err
	}
	note := ""
	if level {
		gain, err := addHeadroom(ctx, path)
		switch {
		case err != nil:
			note = "level not checked (" + err.Error() + ")"
		case gain != 0:
			note = fmt.Sprintf("the true peak was above %g dBTP, so cav lowered the level by %.1f dB (--raw-level keeps it)", headroomPeak, -gain)
		}
	}
	m, err := library.Load(root)
	if err != nil {
		return "", "", err
	}
	sum, _ := fileSHA(path)
	var refs []string
	if ref != "" {
		refs = []string{ref}
	}
	m.Put(root, library.Entry{Path: path, Kind: "audio", Source: tr.Provider, Licence: "generated", CommercialOK: true,
		RetrievedAt: time.Now().UTC(), SHA256: sum, Generated: &library.Generated{Provider: tr.Provider, Model: tr.Model, Prompt: prompt, Refs: refs}})
	return path, note, m.Save(root)
}

type takeScore struct {
	Path       string             `json:"path"`
	Aesthetics map[string]float64 `json:"aesthetics"`
	Clap       *float64           `json:"clap,omitempty"`
}

// scoreTakes runs Audiobox Aesthetics (and CLAP against the prompt, when there is one) on
// the takes, locally through uv. The first run downloads the models (about 2 GB).
func scoreTakes(ctx context.Context, prompt string, tracks []string) ([]takeScore, error) {
	script, err := assets.Python.ReadFile("python/score_takes.py")
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "cav-score")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	var wavs []string
	for i, t := range tracks {
		w := filepath.Join(tmp, fmt.Sprintf("take%d.wav", i))
		if b, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-i", t, "-vn", "-ac", "1", "-ar", "48000", "-c:a", "pcm_s16le", w).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg %s: %v: %s", t, err, strings.TrimSpace(string(b)))
		}
		wavs = append(wavs, w)
	}
	fmt.Fprintln(os.Stderr, "scoring with local models (the first run downloads them)…")
	out, err := pyrun.Run(ctx, filepath.Join(config.CacheDir(), "python"), "score_takes.py", script,
		// audiobox_aesthetics imports requests without declaring it.
		[]string{"audiobox_aesthetics", "requests", "transformers", "torch", "numpy"}, append([]string{prompt}, wavs...), nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Takes []takeScore `json:"takes"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("unexpected scorer output: %w", err)
	}
	for i := range r.Takes {
		r.Takes[i].Path = tracks[i]
	}
	return r.Takes, nil
}

// listenCompare puts several takes side by side: length, tempo, loudness, cuts off the
// downbeat, and the local scores, ranked by production quality.
func listenCompare(a *app, tracks []string, brief, boardFile string, bpm float64, withScores bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var sb *board.Board
	if boardFile != "" {
		var err error
		if sb, err = board.Load(boardFile); err != nil {
			return fail(exitError, err.Error(), "")
		}
		if bpm == 0 {
			bpm = sb.BPM
		}
	}
	type row struct {
		Track    string           `json:"track"`
		Seconds  float64          `json:"seconds"`
		BPM      float64          `json:"bpm"`
		Loudness *listen.Loudness `json:"loudness,omitempty"`
		OffBeat  int              `json:"cutsOffDownbeat"`
		Score    *takeScore       `json:"scores,omitempty"`
	}
	var rows []row
	for _, t := range tracks {
		samples, err := beats.Decode(t)
		if err != nil {
			return fail(exitError, t+": "+err.Error(), ffmpegFix())
		}
		r := beats.Analyze(samples, bpm)
		rw := row{Track: t, Seconds: float64(len(samples)) / beats.SampleRate, BPM: r.BPM}
		if l, err := listen.Measure(ctx, t); err == nil {
			rw.Loudness = &l
		}
		if sb != nil && len(r.Downbeats) > 0 {
			for _, s := range sb.Shots {
				ts := sb.Time(s.Beats[0])
				best := r.Downbeats[0]
				for _, d := range r.Downbeats {
					if math.Abs(d-ts) < math.Abs(best-ts) {
						best = d
					}
				}
				if math.Abs(best-ts) > 0.035 {
					rw.OffBeat++
				}
			}
		}
		rows = append(rows, rw)
	}
	note := ""
	if withScores {
		if ts, err := scoreTakes(ctx, brief, tracks); err != nil {
			note = err.Error()
		} else {
			for i := range rows {
				rows[i].Score = &ts[i]
			}
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].Score.Aesthetics["PQ"] > rows[j].Score.Aesthetics["PQ"] })
		}
	}
	a.emit(map[string]any{"takes": rows, "scoresSkipped": note}, func() {
		for _, r := range rows {
			fmt.Printf("%s\n  %.2f s, %.1f BPM", r.Track, r.Seconds, r.BPM)
			if r.Loudness != nil {
				fmt.Printf(", %.1f LUFS, peak %.1f dBTP", r.Loudness.Integrated, r.Loudness.TruePeak)
			}
			if sb != nil {
				fmt.Printf(", %d of %d cuts off the downbeat", r.OffBeat, len(sb.Shots))
			}
			fmt.Println()
			if r.Score != nil {
				fmt.Printf("  quality %.1f, enjoyment %.1f", r.Score.Aesthetics["PQ"], r.Score.Aesthetics["CE"])
				if r.Score.Clap != nil {
					fmt.Printf(", match to the brief %.3f", *r.Score.Clap)
				}
				fmt.Println()
			}
		}
		if note != "" {
			fmt.Println("scores: skipped (" + note + ")")
		} else if withScores {
			fmt.Println("Ranked by production quality (local model). Listen to the top two before choosing.")
		}
	})
	return nil
}
