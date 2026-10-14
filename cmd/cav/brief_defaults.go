package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rock3r/cav/internal/brief"
)

// briefFlags adds --brief and --no-brief to a command that takes its defaults from the brief.
func briefFlags(fs *flag.FlagSet) (path *string, off *bool) {
	path = fs.String("brief", brief.FileName, "take defaults from this brief")
	off = fs.Bool("no-brief", false, "ignore brief.json")
	return path, off
}

// briefFor loads the brief a command may take defaults from. It returns nil when there is
// none to use: --no-brief was given, or no --brief was named and ./brief.json does not
// exist. A brief that was named but is missing, or that has problems, is an error.
func briefFor(fs *flag.FlagSet, path string, off bool) (*brief.Brief, error) {
	explicit := flagSet(fs, "brief")
	if off && explicit {
		return nil, usageErr("--brief and --no-brief cannot go together")
	}
	if off {
		return nil, nil
	}
	if _, err := os.Stat(path); err != nil {
		if explicit || !os.IsNotExist(err) {
			return nil, fail(exitError, path+" not found", "check the path, or pass --no-brief")
		}
		return nil, nil
	}
	b, err := brief.Load(path)
	if err != nil {
		return nil, fail(exitError, err.Error(), "fix "+path+" (cav brief show), or pass --no-brief")
	}
	return b, nil
}

// briefUse records what a command took from the brief, and which values a flag replaced.
type briefUse struct {
	File       string         `json:"file"`
	Took       map[string]any `json:"took"`
	Overridden []string       `json:"overridden"`
	Notes      []string       `json:"notes,omitempty"`
}

// line says it in one sentence: "from brief.json: 1080 x 1920, 60 fps, 15 s (--fps given)".
func (u *briefUse) line() string {
	var parts []string
	if w, ok := u.Took["width"]; ok {
		parts = append(parts, fmt.Sprintf("%v x %v", w, u.Took["height"]))
	}
	for _, k := range []struct{ key, unit string }{{"fps", " fps"}, {"seconds", " s"}, {"bpm", " bpm"}} {
		if v, ok := u.Took[k.key]; ok {
			parts = append(parts, fmt.Sprintf("%v%s", v, k.unit))
		}
	}
	if v, ok := u.Took["title"]; ok {
		parts = append(parts, fmt.Sprintf("title %q", v))
	}
	s := "from " + u.File + ": "
	if len(parts) == 0 {
		s += "nothing to take"
	} else {
		s += strings.Join(parts, ", ")
	}
	if len(u.Overridden) > 0 {
		s += " (" + strings.Join(u.Overridden, ", ") + " given)"
	}
	return s
}

// compParams are the values a brief can supply to a command.
type compParams struct {
	W, H           int
	FPS, Secs, BPM float64
}

// applyBrief fills the values the user did not give from the brief, which was read from
// file. given says which flag the user gave for a value, and returns "" when none: "size"
// stands for width and height, which come from the brief together or not at all. withBPM is
// false for a command that has no tempo, such as cav scene new.
func applyBrief(p *compParams, b *brief.Brief, file string, given func(key string) string, withBPM bool) *briefUse {
	if b == nil {
		return nil
	}
	u := &briefUse{File: file, Took: map[string]any{}, Overridden: []string{}}
	take := func(key string, has bool, set func()) {
		if !has {
			return
		}
		if flag := given(key); flag != "" {
			u.Overridden = append(u.Overridden, flag)
			return
		}
		set()
	}
	f := b.Format
	take("size", f != nil && f.Width > 0 && f.Height > 0, func() {
		p.W, p.H = f.Width, f.Height
		u.Took["width"], u.Took["height"] = f.Width, f.Height
	})
	take("fps", f != nil && f.FPS > 0, func() { p.FPS = f.FPS; u.Took["fps"] = f.FPS })
	take("seconds", b.Seconds > 0, func() { p.Secs = b.Seconds; u.Took["seconds"] = b.Seconds })
	if withBPM {
		take("bpm", b.BPM > 0, func() { p.BPM = b.BPM; u.Took["bpm"] = b.BPM })
	}
	return u
}

// flagsGiven names the flags among names that the user passed, as "--width, --height".
func flagsGiven(fs *flag.FlagSet, names []string) string {
	var given []string
	for _, n := range names {
		if flagSet(fs, n) {
			given = append(given, "--"+n)
		}
	}
	return strings.Join(given, ", ")
}

// say prints what a command took from the brief, and the notes that go with it.
func (u *briefUse) say(w io.Writer) {
	if u == nil {
		return
	}
	fmt.Fprintln(w, u.line())
	for _, n := range u.Notes {
		fmt.Fprintln(w, "note: "+n)
	}
}
