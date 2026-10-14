package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rock3r/cav/internal/brief"
	"github.com/rock3r/cav/internal/library"
)

func init() {
	register(command{
		name:    "brief",
		args:    "init [-o brief.json] [--title ...] [--goal ...] [--tone ...]... | show [brief.json]   (cav help brief)",
		summary: "Write and check brief.json, the brief you agree with the user before planning (stage 1 of cav guide flow).",
		run:     cmdBrief,
	})
	longHelp["brief"] = `
The brief is brief.json: what the piece is for, written with the user before any planning.
It is stage 1 of cav guide flow. Fields:

  {
    "schema": "cav-brief/1",
    "title": "Launch day sting",
    "goal": "Announce the launch",
    "audience": "Existing users",
    "tone": ["confident", "playful"],
    "seconds": 15,
    "bpm": 120,
    "format": {"width": 1080, "height": 1920, "fps": 30},
    "deliverables": ["15 s vertical"],
    "mustHave": ["the logo in the last shot"],
    "avoid": ["stock music"],
    "music": "upbeat, no vocals",
    "refs": ["https://vimeo.com/123456789", "moodboard/arena-1234.jpg"]
  }

Required: title, goal, audience, tone, seconds, format. The rest is optional. "tone" may be
one word or a list.

  cav brief init [-o brief.json] [--force]
        [--title T] [--goal G] [--audience A] [--tone WORD]... [--seconds N] [--bpm N]
        [--width W --height H] [--fps N] [--deliverable D]... [--must-have M]...
        [--avoid A]... [--music M] [--ref LINK_OR_FILE]...
        writes the file. Every flag is optional: without flags it writes a template to fill
        by hand. List flags repeat (--tone confident --tone playful); a comma stays inside
        one value. It will not overwrite a file unless you pass --force.
  cav brief show [brief.json]
        checks the file and prints it. A value cav cannot use (seconds: "fifteen") is a
        problem and the command fails. A required field that is still empty is listed under
        "Still to fill" and does not fail. Fields cav does not know are kept, with a note.

cav board init and cav scene new take size, fps and length from brief.json when it
exists; flags win. The brief's refs are the first mood board items: a link stays a link, and
a file in the project should be recorded with cav ref add. Ask the user to review the brief:
cav review brief.json`
}

func cmdBrief(a *app, args []string) error {
	if len(args) == 0 {
		printHelp("brief")
		return nil
	}
	sub, args := args[0], args[1:]
	switch sub {
	case "init":
		return briefInit(a, args)
	case "show":
		return briefShow(a, args)
	}
	return usageErr("unknown brief command %q (init, show)", sub)
}

func briefInit(a *app, args []string) error {
	fs := flag.NewFlagSet("brief init", flag.ContinueOnError)
	out := fs.String("o", brief.FileName, "file to write")
	force := fs.Bool("force", false, "overwrite an existing file")
	title := fs.String("title", "", "name of the piece")
	goal := fs.String("goal", "", "what the piece is for")
	audience := fs.String("audience", "", "who watches it")
	music := fs.String("music", "", "the music in a few words")
	var tone, deliverables, mustHave, avoid, refs stringList
	fs.Var(&tone, "tone", "a word for the tone (repeat for more)")
	fs.Var(&deliverables, "deliverable", "a file to hand over (repeat for more)")
	fs.Var(&mustHave, "must-have", "something it must show (repeat for more)")
	fs.Var(&avoid, "avoid", "something to keep out (repeat for more)")
	fs.Var(&refs, "ref", "a web link or a file (repeat for more)")
	seconds := fs.Float64("seconds", 0, "length in seconds")
	bpm := fs.Float64("bpm", 0, "tempo")
	fps := fs.Float64("fps", 0, "frame rate")
	width := fs.Int("width", 0, "width")
	height := fs.Int("height", 0, "height")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageErr("usage: cav brief init [-o brief.json] [flags]; cav help brief lists them")
	}
	if _, err := os.Stat(*out); err == nil && !*force {
		return fail(exitError, *out+" already exists", "edit it by hand and check it with cav brief show, or pass --force to start over")
	}
	b := &brief.Brief{Title: *title, Goal: *goal, Audience: *audience, Music: *music,
		Tone: brief.List(tone), Deliverables: brief.List(deliverables), MustHave: brief.List(mustHave),
		Avoid: brief.List(avoid), Refs: brief.List(refs), Seconds: *seconds, BPM: *bpm}
	// A format is written only when the flags give one: a default would later look like a decision.
	if *width != 0 || *height != 0 || *fps != 0 {
		b.Format = &brief.Format{Width: *width, Height: *height, FPS: *fps}
	}
	// Check the values the way a later read would, before anything is written.
	checked, err := checkedBrief(b, *out)
	if err != nil {
		return fail(exitError, err.Error(), "fix the flags; cav help brief lists the fields")
	}
	if err := b.Save(*out); err != nil {
		return err
	}
	missing, notes := checked.Missing(), checked.Notes()
	a.emit(map[string]any{"file": *out, "brief": b, "missing": nonNil(missing), "notes": nonNil(notes)}, func() {
		fmt.Printf("wrote %s\n", *out)
		printGaps(missing, notes)
		show, review := "cav brief show", "cav review "+*out
		if *out != brief.FileName {
			show += " " + *out
		}
		fmt.Printf("Check it: %s\nThen ask the user to review it: %s &\n", show, review)
	})
	return nil
}

// checkedBrief runs the file checks on a brief that is not on disk yet.
func checkedBrief(b *brief.Brief, path string) (*brief.Brief, error) {
	tmp := *b
	tmp.Schema = brief.Schema
	data, err := json.Marshal(&tmp)
	if err != nil {
		return nil, err
	}
	c := brief.Parse(data, filepath.Dir(path))
	if p := c.Problems(); len(p) > 0 {
		return nil, &brief.Error{Path: path, Problems: p}
	}
	return c, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// printGaps says what the brief still lacks, and what is worth knowing about it.
func printGaps(missing, notes []string) {
	if len(missing) > 0 {
		fmt.Println("Still to fill: " + strings.Join(missing, ", "))
	}
	for _, n := range notes {
		fmt.Println("note: " + n)
	}
}

func briefShow(a *app, args []string) error {
	fs := flag.NewFlagSet("brief show", flag.ContinueOnError)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 1 {
		return usageErr("usage: cav brief show [brief.json]")
	}
	path := boardPath(pos, brief.FileName)
	b, err := brief.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return fail(exitError, path+" not found", "cav brief init writes one")
	}
	if err != nil {
		return fail(exitError, err.Error(), "fix the file by hand, then run cav brief show again")
	}
	refs := briefRefStates(b)
	missing, notes := b.Missing(), b.Notes()
	data := map[string]any{"file": path, "brief": b, "missing": nonNil(missing), "notes": nonNil(notes), "refs": refs}
	if len(b.Extra) > 0 {
		data["extra"] = b.Extra
	}
	a.emit(data, func() {
		printBrief(path, b, refs)
		printGaps(missing, notes)
		for _, r := range refs {
			if r.Kind == "file" && r.Exists && !r.Recorded {
				fmt.Printf("record it: cav ref add %s --licence LICENCE_ID\n", shellArg(r.Path))
			}
		}
	})
	return nil
}

// briefRef is one ref of the brief and what the project knows about it.
type briefRef struct {
	Ref      string `json:"ref"`
	Kind     string `json:"kind"` // link or file
	Exists   bool   `json:"exists"`
	Recorded bool   `json:"recorded"`
	Licence  string `json:"licence,omitempty"`
	Path     string `json:"path,omitempty"` // a file ref as seen from the current folder
}

// briefRefStates matches the brief's file refs against the manifest. It only reads: a
// manifest it cannot read leaves every file unrecorded.
func briefRefStates(b *brief.Brief) []briefRef {
	root := library.Root()
	m, _ := library.Load(root)
	refs := []briefRef{}
	for _, r := range b.RefList() {
		s := briefRef{Ref: r.Value, Kind: "file", Exists: r.Exists}
		if !r.Link {
			s.Path = r.Path
			if wd, err := os.Getwd(); err == nil {
				if rel, err := filepath.Rel(wd, r.Path); err == nil {
					s.Path = rel
				}
			}
		}
		if r.Link {
			s.Kind, s.Exists = "link", true
		} else if m != nil {
			rel := library.Rel(root, r.Path)
			for _, e := range m.Assets {
				if e.Path == rel {
					s.Recorded, s.Licence = true, e.Licence
				}
			}
		}
		refs = append(refs, s)
	}
	return refs
}

func printBrief(path string, b *brief.Brief, refs []briefRef) {
	row := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			value = "-"
		}
		fmt.Printf("%-13s %s\n", label, value)
	}
	list := func(l brief.List) string { return strings.Join(l, "; ") }
	title := b.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Printf("%s (%s)\n", title, path)
	row("Goal", b.Goal)
	row("Audience", b.Audience)
	row("Tone", list(b.Tone))
	length := ""
	if b.Seconds > 0 {
		length = fmt.Sprintf("%g s", b.Seconds)
		if b.BPM > 0 {
			length += fmt.Sprintf(" at %g bpm (%g beats)", b.BPM, b.Seconds*b.BPM/60)
		}
	} else if b.BPM > 0 {
		length = fmt.Sprintf("%g bpm", b.BPM)
	}
	row("Length", length)
	format := ""
	if f := b.Format; f != nil {
		format = f.Text()
		if f.Width > 0 && f.Height > 0 {
			format = fmt.Sprintf("%d x %d (%s)", f.Width, f.Height, aspectOf(f.Width, f.Height))
			if f.FPS > 0 {
				format += fmt.Sprintf(" at %g fps", f.FPS)
			}
		}
	}
	row("Format", format)
	row("Deliverables", list(b.Deliverables))
	row("Must have", list(b.MustHave))
	row("Avoid", list(b.Avoid))
	row("Music", b.Music)
	if len(refs) == 0 {
		row("References", "")
	}
	for i, r := range refs {
		label := ""
		if i == 0 {
			label = "References"
		}
		row(label, r.Ref+" ("+refKind(r)+")")
	}
	extraKeys := make([]string, 0, len(b.Extra))
	for k := range b.Extra {
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		row(k, string(b.Extra[k]))
	}
}

// refKind says in a few words what a ref is and what the project knows about it.
func refKind(r briefRef) string {
	switch {
	case r.Kind == "link":
		return "link; stays an embed, never downloaded"
	case !r.Exists:
		return "file; not found"
	case r.Recorded:
		return "file; recorded, licence " + r.Licence
	}
	return "file; not recorded yet"
}
