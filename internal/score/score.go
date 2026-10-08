// Package score writes a REAPER project (.rpp, plain text) for finishing a piece's music by
// ear: the render on a video track, a marker at every shot, and the music takes on their own
// tracks. REAPER renders it from the command line with -renderproject.
//
// The .rpp layout follows REAPER 7 projects. It was checked by hand in REAPER 7.82 (see issue #12).
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
		// The whole project with no tail: REAPER's default adds a 1 s tail, so the WAV
		// came out a second longer than the piece. The render sample rate follows the
		// project; REAPER 7 has no RENDER_SRATE token and warns about it on load.
		b.WriteString("  RENDER_RANGE 1 0 0 0 1000\n")
	}
	for i, m := range p.Markers {
		fmt.Fprintf(&b, "  MARKER %d %.6f %s 0 0 1\n", i+1, m.Seconds, quote(m.Name))
	}
	if p.Video != "" {
		// Volume at zero: the render carries its own soundtrack, which would otherwise
		// play under the takes and end up in the music WAV. Muting the track would also
		// hide the video.
		b.WriteString("  <TRACK\n    NAME \"Video\"\n    VOLPAN 0 0 -1 -1 1\n")
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

// ReaperArgs is the command line that renders a project and exits. -ignoreerrors keeps a
// load error (a missing file, an unknown token) from waiting on a dialog.
func ReaperArgs(rpp string) []string {
	return []string{"-nosplash", "-newinst", "-ignoreerrors", "-renderproject", rpp}
}

// audioDevice is what REAPER 7 writes to reaper.ini when its default audio device is
// accepted at the first-run question "You have not yet selected an audio device". Until
// the marker key is present, REAPER asks that question at startup, and -renderproject
// waits on it. Both were found by testing REAPER 7.82 (issue #12):
//   - macOS: the default Core Audio device; all four keys are needed.
//   - Windows: mode=2 in [audioconfig] is REAPER's own default audio system; it alone
//     is enough. REAPER fills in the other [audioconfig] keys with defaults.
var audioDevice = map[string]struct {
	section, marker string
	keys            []string
}{
	"darwin": {"reaper", "coreaudiooutdevnew", []string{
		"coreaudioindevnew=<default>",
		"coreaudiooutdevnew=<default>",
		"coreaudiobs=512",
		"coreaudiosrate=48000",
	}},
	"windows": {"audioconfig", "mode", []string{"mode=2"}},
}

// WithAudioDevice returns REAPER's reaper.ini text with REAPER's default audio device
// selected for goos, and whether it changed anything. It leaves a device the user picked
// alone, and changes nothing on other systems.
func WithAudioDevice(ini, goos string) (string, bool) {
	dev, ok := audioDevice[goos]
	if !ok {
		return ini, false
	}
	eol := "\n"
	if strings.Contains(ini, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(ini, "\r\n", "\n"), "\n")
	section, start, end := "", -1, len(lines)
	have := map[string]bool{}
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if section == dev.section {
				end = i
			}
			section = strings.ToLower(strings.Trim(t, "[]"))
			if section == dev.section {
				start = i
			}
			continue
		}
		if section == dev.section {
			if k, _, ok := strings.Cut(t, "="); ok {
				have[k] = true
			}
		}
	}
	if have[dev.marker] {
		return ini, false
	}
	var add []string
	for _, kv := range dev.keys {
		if k, _, _ := strings.Cut(kv, "="); !have[k] {
			add = append(add, kv)
		}
	}
	if start < 0 {
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
		lines = append(lines, "["+dev.section+"]")
		lines = append(lines, add...)
		return strings.Join(lines, eol) + eol, true
	}
	// Insert after the section's last non-blank line.
	at := end
	for at > start+1 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	out := append(append(append([]string{}, lines[:at]...), add...), lines[at:]...)
	return strings.Join(out, eol), true
}
