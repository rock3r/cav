// Package score writes a REAPER project (.rpp, plain text) for finishing a piece's music by
// ear: the render on a video track, a marker at every shot, and the music takes on their own
// tracks. REAPER renders it from the command line with -renderproject.
//
// The .rpp layout follows REAPER 7 projects; it has not been opened in REAPER by these tests.
package score

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Marker struct {
	Name    string
	Seconds float64
}

type Take struct {
	Path    string
	Seconds float64 // length; 0 means the piece's length
}

type Project struct {
	BPM        float64
	Seconds    float64
	SampleRate int
	Video      string // the render, absolute
	Markers    []Marker
	Takes      []Take // the first is heard; the rest are muted for comparing
	RenderTo   string // the WAV a command-line render writes
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `'`) + `"` }

func sourceKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return "MP3"
	case ".wav":
		return "WAVE"
	case ".ogg", ".opus":
		return "VORBIS"
	case ".flac":
		return "FLAC"
	case ".mp4", ".mov", ".m4v", ".webm":
		return "VIDEO"
	}
	return "WAVE"
}

func item(b *strings.Builder, name, path string, length float64) {
	fmt.Fprintf(b, "    <ITEM\n      POSITION 0\n      LENGTH %.6f\n      NAME %s\n      <SOURCE %s\n        FILE %s\n      >\n    >\n",
		length, quote(name), sourceKind(path), quote(path))
}

// RPP returns the project text.
func (p Project) RPP() string {
	if p.SampleRate == 0 {
		p.SampleRate = 48000
	}
	var b strings.Builder
	b.WriteString("<REAPER_PROJECT 0.1 \"7.0\" 0\n")
	fmt.Fprintf(&b, "  TEMPO %g 4 4\n", p.BPM)
	fmt.Fprintf(&b, "  SAMPLERATE %d 1 0\n", p.SampleRate)
	if p.RenderTo != "" {
		dir, file := filepath.Split(p.RenderTo)
		fmt.Fprintf(&b, "  RENDER_FILE %s\n", quote(strings.TrimSuffix(dir, string(filepath.Separator))))
		fmt.Fprintf(&b, "  RENDER_PATTERN %s\n", quote(strings.TrimSuffix(file, filepath.Ext(file))))
		b.WriteString("  RENDER_SRATE 0\n  RENDER_RANGE 1 0 0 18 1000\n")
	}
	for i, m := range p.Markers {
		fmt.Fprintf(&b, "  MARKER %d %.6f %s 0 0 1\n", i+1, m.Seconds, quote(m.Name))
	}
	if p.Video != "" {
		b.WriteString("  <TRACK\n    NAME \"Video\"\n")
		item(&b, filepath.Base(p.Video), p.Video, p.Seconds)
		b.WriteString("  >\n")
	}
	for i, t := range p.Takes {
		length := t.Seconds
		if length <= 0 {
			length = p.Seconds
		}
		mute := 0
		if i > 0 {
			mute = 1
		}
		fmt.Fprintf(&b, "  <TRACK\n    NAME %s\n    MUTESOLO %d 0 0\n", quote(fmt.Sprintf("Bed %d: %s", i+1, filepath.Base(t.Path))), mute)
		item(&b, filepath.Base(t.Path), t.Path, length)
		b.WriteString("  >\n")
	}
	b.WriteString("  <TRACK\n    NAME \"SFX\"\n  >\n")
	b.WriteString(">\n")
	return b.String()
}

// ReaperArgs is the command line that renders a project and exits.
func ReaperArgs(rpp string) []string {
	return []string{"-nosplash", "-newinst", "-renderproject", rpp}
}
