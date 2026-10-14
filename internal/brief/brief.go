// Package brief reads and checks brief.json, the brief the agent agrees with the user
// before any planning starts (stage 1 of cav guide flow).
//
// The file can be wrong in three ways, kept apart on purpose. A problem is a value cav
// cannot use, such as "seconds": "fifteen". A missing field is a gap the agent still has
// to fill. A note is something worth knowing that blocks nothing.
package brief

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Schema is the version of the file this package reads and writes.
const Schema = "cav-brief/1"

// FileName is where the brief lives by default.
const FileName = "brief.json"

// Order is the order of the fields in the file, in `cav brief show` and in review.
var Order = []string{"title", "goal", "audience", "tone", "seconds", "bpm", "format", "deliverables", "mustHave", "avoid", "music", "refs"}

// Required are the fields the plan cannot start without.
var Required = []string{"title", "goal", "audience", "tone", "seconds", "format"}

// List is a list of words. In a file it may be one word ("calm") or a list (["calm", "warm"]).
// It is always written as a list.
type List []string

func (l *List) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*l = List{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*l = many
	return nil
}

func (l List) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(l))
}

// Format is the size and frame rate of the piece.
type Format struct {
	Width  int     `json:"width,omitempty"`
	Height int     `json:"height,omitempty"`
	FPS    float64 `json:"fps,omitempty"`
}

// Brief is brief.json. The fields are in the order of the file.
type Brief struct {
	Schema       string  `json:"schema"`
	Title        string  `json:"title"`
	Goal         string  `json:"goal"`
	Audience     string  `json:"audience"`
	Tone         List    `json:"tone"`
	Seconds      float64 `json:"seconds,omitempty"`
	BPM          float64 `json:"bpm,omitempty"`
	Format       *Format `json:"format,omitempty"`
	Deliverables List    `json:"deliverables"`
	MustHave     List    `json:"mustHave"`
	Avoid        List    `json:"avoid"`
	Music        string  `json:"music"`
	Refs         List    `json:"refs"`

	// Extra holds the fields cav does not know. They are kept and shown in review.
	Extra map[string]json.RawMessage `json:"-"`

	dir      string
	problems []string
	notes    []string
}

// Error is a brief with problems.
type Error struct {
	Path     string
	Problems []string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(filepath.Base(e.Path) + " has problems:")
	for _, p := range e.Problems {
		b.WriteString("\n  - " + p)
	}
	return b.String()
}

// Problems are the values cav cannot use.
func (b *Brief) Problems() []string { return b.problems }

// Notes are things worth knowing that block nothing.
func (b *Brief) Notes() []string { return b.notes }

// Missing lists the required fields that are still empty, in the order of Required.
func (b *Brief) Missing() []string {
	var out []string
	for _, k := range Required {
		var empty bool
		switch k {
		case "title":
			empty = strings.TrimSpace(b.Title) == ""
		case "goal":
			empty = strings.TrimSpace(b.Goal) == ""
		case "audience":
			empty = strings.TrimSpace(b.Audience) == ""
		case "tone":
			empty = !hasWords(b.Tone)
		case "seconds":
			empty = b.Seconds == 0
		case "format":
			empty = b.Format == nil || b.Format.Width == 0 || b.Format.Height == 0
		}
		if empty {
			out = append(out, k)
		}
	}
	return out
}

func hasWords(l List) bool {
	for _, s := range l {
		if strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

var known = func() map[string]bool {
	m := map[string]bool{"schema": true}
	for _, k := range Order {
		m[k] = true
	}
	return m
}()

// Parse reads a brief. dir is the folder of the file, which local refs are relative to. It
// never fails: what is wrong with the file is in Problems.
func Parse(data []byte, dir string) *Brief {
	b := &Brief{dir: dir}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		b.problem("%s", syntaxMessage(data, err))
		return b
	}
	if raw == nil {
		b.problem("a brief must be a JSON object like {\"title\": \"...\"}, not null")
		return b
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if known[k] {
			continue
		}
		if want := lookalike(k); want != "" {
			b.problem("%q is not a field; did you mean %q?", k, want)
			continue
		}
		if b.Extra == nil {
			b.Extra = map[string]json.RawMessage{}
		}
		b.Extra[k] = raw[k]
		b.note("cav does not use %q; it is kept and shown in review", k)
	}
	set := func(key string, dst any, bad string) {
		if v, ok := raw[key]; ok && string(v) != "null" {
			if json.Unmarshal(v, dst) != nil {
				b.problem("%s", bad)
			}
		}
	}
	set("schema", &b.Schema, "schema must be text")
	set("title", &b.Title, "title must be text")
	set("goal", &b.Goal, "goal must be text")
	set("audience", &b.Audience, "audience must be text")
	set("music", &b.Music, "music must be text")
	set("tone", &b.Tone, `tone must be a word or a list of words, like ["calm", "warm"]`)
	set("deliverables", &b.Deliverables, `deliverables must be a list of words, like ["15 s vertical", "16:9 cut"]`)
	set("mustHave", &b.MustHave, `mustHave must be a list of words, like ["the logo in the last shot"]`)
	set("avoid", &b.Avoid, `avoid must be a list of words, like ["stock music"]`)
	set("refs", &b.Refs, `refs must be a list of links or files, like ["https://vimeo.com/123", "moodboard/a.jpg"]`)
	set("seconds", &b.Seconds, "seconds must be a number of seconds, like 15")
	set("bpm", &b.BPM, "bpm must be a number, like 120")
	if v, ok := raw["format"]; ok && string(v) != "null" {
		var f Format
		dec := json.NewDecoder(bytes.NewReader(v))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			b.problem(`format must look like {"width": 1080, "height": 1920, "fps": 60}`)
		} else {
			b.Format = &f
		}
	}
	b.validate()
	return b
}

func (b *Brief) problem(format string, a ...any) {
	b.problems = append(b.problems, fmt.Sprintf(format, a...))
}

func (b *Brief) note(format string, a ...any) {
	b.notes = append(b.notes, fmt.Sprintf(format, a...))
}

// lookalike returns the known field that k spells with other letter case ("musthave").
func lookalike(k string) string {
	for _, f := range append([]string{"schema"}, Order...) {
		if strings.EqualFold(k, f) {
			return f
		}
	}
	return ""
}

// syntaxMessage says where a JSON file is broken, as a line and a column.
func syntaxMessage(data []byte, err error) string {
	if len(bytes.TrimSpace(data)) == 0 {
		return "the file is empty"
	}
	var se *json.SyntaxError
	if errors.As(err, &se) {
		off := min(int(se.Offset), len(data))
		line := 1 + bytes.Count(data[:off], []byte("\n"))
		col := off - (bytes.LastIndexByte(data[:off], '\n') + 1)
		return fmt.Sprintf("the file is not valid JSON (line %d, column %d): %v", line, col, err)
	}
	return "the file must be one JSON object with the brief's fields: " + err.Error()
}

func (b *Brief) validate() {
	switch {
	case b.Schema == "" || b.Schema == Schema:
	case strings.HasPrefix(b.Schema, "cav-brief/"):
		b.problem("brief.json is %s; this cav reads %s. Update cav (cav update).", b.Schema, Schema)
	default:
		b.problem("schema must be %q", Schema)
	}
	if b.Seconds < 0 || b.Seconds > 3600 || math.IsNaN(b.Seconds) {
		b.problem("seconds must be more than 0 and at most 3600")
	}
	if b.BPM != 0 && (b.BPM < 30 || b.BPM > 300) {
		b.problem("bpm must be between 30 and 300")
	}
	if f := b.Format; f != nil {
		if (f.Width == 0) != (f.Height == 0) {
			b.problem("format needs both width and height, or neither")
		}
		for _, d := range []struct {
			name string
			v    int
		}{{"width", f.Width}, {"height", f.Height}} {
			if d.v != 0 && (d.v < 16 || d.v > 16384) {
				b.problem("format %s must be between 16 and 16384", d.name)
			}
		}
		if f.FPS < 0 || f.FPS > 240 {
			b.problem("format fps must be more than 0 and at most 240")
		}
		if f.Width%2 != 0 || f.Height%2 != 0 {
			b.note("most video encoders need even sizes; %d x %d has an odd side", f.Width, f.Height)
		}
	}
	for _, l := range []struct {
		name string
		v    List
	}{{"tone", b.Tone}, {"deliverables", b.Deliverables}, {"mustHave", b.MustHave}, {"avoid", b.Avoid}, {"refs", b.Refs}} {
		for i, s := range l.v {
			if strings.TrimSpace(s) == "" {
				b.problem("%s item %d is empty", l.name, i+1)
			}
		}
	}
	for _, r := range b.Refs {
		if strings.TrimSpace(r) == "" {
			continue
		}
		ref := b.ref(r)
		switch {
		case ref.Link && !strings.HasPrefix(strings.ToLower(r), "http://") && !strings.HasPrefix(strings.ToLower(r), "https://"):
			b.problem("refs: %q is not a web link; use http or https, or a file path", r)
		case !ref.Link && !ref.Exists:
			b.note("ref %q was not found next to the brief", r)
		}
	}
}

// Load reads and checks a brief file. It fails with *Error when the file has problems.
// A brief that is only incomplete loads: ask Missing.
func Load(path string) (*Brief, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b := Parse(data, filepath.Dir(path))
	if len(b.problems) > 0 {
		return nil, &Error{Path: path, Problems: b.problems}
	}
	return b, nil
}

// Save writes the brief with the schema set, fields in order and a final newline.
func (b *Brief) Save(path string) error {
	b.Schema = Schema
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Ref is one reference of the brief: a web link, or a file in the project.
type Ref struct {
	Value string
	Link  bool
	// Path is the file, joined to the brief's folder. It is empty for a link.
	Path   string
	Exists bool
}

func (b *Brief) ref(value string) Ref {
	r := Ref{Value: value, Link: strings.Contains(value, "://")}
	if !r.Link {
		r.Path = value
		if !filepath.IsAbs(value) {
			r.Path = filepath.Join(b.dir, value)
		}
		_, err := os.Stat(r.Path)
		r.Exists = err == nil
	}
	return r
}

// RefList lists the refs with what is known about each file.
func (b *Brief) RefList() []Ref {
	var out []Ref
	for _, r := range b.Refs {
		if strings.TrimSpace(r) != "" {
			out = append(out, b.ref(r))
		}
	}
	return out
}

// Field is one field of the file as the review page shows it. Raw is the value as written.
type Field struct {
	Key   string
	Raw   json.RawMessage
	Title string
	Text  string
}

// Fields lists the fields that are in the file: the known ones in Order, then the others
// sorted by name. "schema" is left out. It needs a brief without problems, not a complete one.
func Fields(path string) ([]Field, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b := Parse(data, filepath.Dir(path))
	if len(b.problems) > 0 {
		return nil, &Error{Path: path, Problems: b.problems}
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	delete(raw, "schema")
	var out []Field
	for _, k := range Order {
		if v, ok := raw[k]; ok {
			out = append(out, Field{Key: k, Raw: v, Title: Titles[k], Text: b.text(k, v)})
			delete(raw, k)
		}
	}
	rest := make([]string, 0, len(raw))
	for k := range raw {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		out = append(out, Field{Key: k, Raw: raw[k], Title: k, Text: extraText(raw[k])})
	}
	return out, nil
}

// Titles are the field names as people read them.
var Titles = map[string]string{
	"title": "Title", "goal": "Goal", "audience": "Audience", "tone": "Tone", "seconds": "Length",
	"bpm": "Tempo", "format": "Format", "deliverables": "Deliverables", "mustHave": "Must have",
	"avoid": "Avoid", "music": "Music", "refs": "References",
}

// text is a known field as words.
func (b *Brief) text(key string, raw json.RawMessage) string {
	if string(raw) == "null" {
		return ""
	}
	switch key {
	case "title":
		return b.Title
	case "goal":
		return b.Goal
	case "audience":
		return b.Audience
	case "music":
		return b.Music
	case "tone":
		return strings.Join(b.Tone, "; ")
	case "deliverables":
		return strings.Join(b.Deliverables, "; ")
	case "mustHave":
		return strings.Join(b.MustHave, "; ")
	case "avoid":
		return strings.Join(b.Avoid, "; ")
	case "refs":
		return strings.Join(b.Refs, "; ")
	case "seconds":
		return fmt.Sprintf("%g s", b.Seconds)
	case "bpm":
		return fmt.Sprintf("%g bpm", b.BPM)
	case "format":
		return b.Format.Text()
	}
	return string(raw)
}

// Text says the format in words: "1080 x 1920 at 60 fps".
func (f *Format) Text() string {
	if f == nil {
		return ""
	}
	var parts []string
	if f.Width > 0 && f.Height > 0 {
		parts = append(parts, fmt.Sprintf("%d x %d", f.Width, f.Height))
	}
	if f.FPS > 0 {
		parts = append(parts, fmt.Sprintf("%g fps", f.FPS))
	}
	return strings.Join(parts, " at ")
}

// extraText shows a field cav does not know: a string as it is, a list joined, the rest as JSON.
func extraText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var l []any
	if json.Unmarshal(raw, &l) == nil {
		parts := make([]string, len(l))
		for i, e := range l {
			if s, ok := e.(string); ok {
				parts[i] = s
			} else {
				b, _ := json.Marshal(e)
				parts[i] = string(b)
			}
		}
		return strings.Join(parts, "; ")
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) == nil {
		return buf.String()
	}
	return string(raw)
}
