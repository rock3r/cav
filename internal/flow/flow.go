// Package flow reads the production flow: the list of stages that `cav guide flow` prints.
//
// A flow is a Markdown file. The built-in standard flow is one, and a user can replace it
// with a flow.md in the project or in the cav home folder. A line `## name` starts a stage.
// Inside a stage, the lines `outputs:`, `review:` and `use:` are keys. Everything else is
// text for the agent.
package flow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/review"
)

// File is the name of a flow file.
const File = "flow.md"

// Where a flow comes from.
const (
	SourceProject  = "project"
	SourceUser     = "user"
	SourceStandard = "standard"
)

// Stage is one `## name` section.
type Stage struct {
	Name    string
	Line    int      // line of the `## name` heading
	Outputs []string // paths inside the project; a trailing / is a folder, * is a pattern
	Review  bool     // the user approves the first output in cav review
	Use     string   // free text for the agent
}

// Flow is a parsed flow file.
type Flow struct {
	Source string // project, user or standard
	Path   string // the file, empty for the standard flow
	Text   string // the file as it was read, without a byte order mark

	// Unclosed is the line of a <!-- that is never closed, or 0. The rest of the file
	// after it is ignored.
	Unclosed int
	Stages   []Stage
}

// LineError is a problem on one line of a flow file.
type LineError struct {
	File string
	Line int
	Msg  string
}

func (e *LineError) Error() string {
	return fmt.Sprintf("%s line %d: %s", e.File, e.Line, e.Msg)
}

// Parse reads a flow file. file is only used in messages. It returns every problem it
// finds, joined, and no flow when there is one.
func Parse(file string, src []byte) (*Flow, error) {
	text := strings.TrimPrefix(string(src), "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	p := &parser{file: file}
	p.run(text)
	if len(p.errs) > 0 {
		return nil, errors.Join(p.errs...)
	}
	return &Flow{Text: text, Stages: p.stages, Unclosed: p.unclosed}, nil
}

type parser struct {
	file   string
	stages []Stage
	errs   []error

	// State of the current stage.
	cur         *Stage
	outputsLine int
	reviewLine  int
	useLine     int
	seenStageAt map[string]int
	unclosed    int // line of the last <!-- that opened a comment (valid when still open)
}

func (p *parser) fail(line int, format string, args ...any) {
	p.errs = append(p.errs, &LineError{File: p.file, Line: line, Msg: fmt.Sprintf(format, args...)})
}

func (p *parser) run(text string) {
	p.seenStageAt = map[string]int{}
	fence := ""
	inComment := false
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		t := strings.TrimSpace(line)
		if fence != "" {
			if m := fenceMarker(t); m != "" && !indentedCode(line) && m[0] == fence[0] && len(m) >= len(fence) && strings.Trim(t, m[:1]) == "" {
				fence = ""
			}
			continue
		}
		if !inComment && indentedCode(line) {
			continue
		}
		// What is left of the line outside <!-- --> comments.
		was := inComment
		t = strings.TrimSpace(stripComments(line, &inComment))
		if inComment && !was {
			p.unclosed = n
		}
		if m := fenceMarker(t); m != "" {
			fence = m
			continue
		}
		if name, ok := stageName(t); ok {
			p.startStage(n, name)
			continue
		}
		if key, val, ok := keyLine(t); ok {
			p.key(n, key, val)
		}
	}
	if !inComment {
		p.unclosed = 0
	}
	p.endStage()
	if len(p.stages) == 0 && len(p.errs) == 0 {
		p.fail(1, "flow.md has no stages: start each stage with a line like `## storyboard`")
	}
}

func (p *parser) startStage(line int, name string) {
	p.endStage()
	p.outputsLine, p.reviewLine, p.useLine = 0, 0, 0
	if name == "" {
		p.fail(line, "a stage needs a name: write it after ##, for example `## storyboard`")
		p.cur = &Stage{Line: line}
		return
	}
	lower := strings.ToLower(name)
	if first, dup := p.seenStageAt[lower]; dup {
		p.fail(line, "stage %s is already defined on line %d; give each stage its own name", name, first)
	} else {
		p.seenStageAt[lower] = line
	}
	p.cur = &Stage{Name: name, Line: line}
}

// endStage checks the stage that just ended and stores it.
func (p *parser) endStage() {
	s := p.cur
	if s == nil {
		return
	}
	p.cur = nil
	if s.Review && len(s.Outputs) == 0 && p.outputsLine == 0 {
		p.fail(s.Line, "stage %s (line %d) has review: yes but no outputs:; cav reads the approval from the review file of the first output", s.Name, s.Line)
	}
	p.stages = append(p.stages, *s)
}

func (p *parser) key(line int, key, val string) {
	if p.cur == nil {
		p.fail(line, "%s: comes before the first stage; put it under a line like `## storyboard`", key)
		return
	}
	s := p.cur
	switch key {
	case "outputs":
		if p.outputsLine != 0 {
			p.fail(line, "stage %s already has outputs: (line %d); put all paths on one line", s.Name, p.outputsLine)
			return
		}
		p.outputsLine = line
		s.Outputs = p.outputs(line, val)
	case "review":
		if p.reviewLine != 0 {
			p.fail(line, "stage %s already has review: (line %d)", s.Name, p.reviewLine)
			return
		}
		p.reviewLine = line
		switch strings.ToLower(val) {
		case "yes":
			s.Review = true
		case "no":
		default:
			p.fail(line, "review: must be yes or no, not %q", val)
		}
	case "use":
		if p.useLine != 0 {
			p.fail(line, "stage %s already has use: (line %d)", s.Name, p.useLine)
			return
		}
		p.useLine = line
		s.Use = val
	}
}

func (p *parser) outputs(line int, val string) []string {
	var out []string
	rejected := 0
	for _, item := range strings.Split(val, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "`")
		item = strings.TrimSpace(item)
		item = strings.ReplaceAll(item, `\`, "/")
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "/") || filepath.IsAbs(item) || (len(item) > 1 && item[1] == ':') {
			p.fail(line, "outputs are paths inside the project; %s is not", item)
			rejected++
			continue
		}
		bad := false
		for _, seg := range strings.Split(item, "/") {
			if seg == ".." {
				p.fail(line, "outputs are paths inside the project; %s is not", item)
				bad = true
				rejected++
				break
			}
		}
		if bad {
			continue
		}
		if strings.Contains(item, "**") {
			p.fail(line, "%s: ** is not supported; * matches inside one folder", item)
			rejected++
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 && rejected == 0 {
		p.fail(line, "outputs: is empty")
	}
	return out
}

// stripComments removes HTML comments from a line. *inComment says whether the line starts
// inside a comment that an earlier line opened, and is updated for the next line. Text
// after a closing --> on the same line is kept.
func stripComments(line string, inComment *bool) string {
	var out strings.Builder
	for {
		if *inComment {
			end := strings.Index(line, "-->")
			if end < 0 {
				return out.String()
			}
			line = line[end+len("-->"):]
			*inComment = false
			continue
		}
		start := commentStart(line)
		if start < 0 {
			out.WriteString(line)
			return out.String()
		}
		out.WriteString(line[:start])
		line = line[start+len("<!--"):]
		*inComment = true
	}
}

// commentStart is the index of the first <!-- that is not inside an inline code span, or -1.
// A code span starts with a run of backticks and ends at the next run of the same length.
// A run with no closer is plain text.
func commentStart(line string) int {
	for i := 0; i < len(line); {
		switch {
		case line[i] == '`':
			n := 1
			for i+n < len(line) && line[i+n] == '`' {
				n++
			}
			if end := codeSpanEnd(line, i+n, n); end >= 0 {
				i = end
			} else {
				i += n
			}
		case strings.HasPrefix(line[i:], "<!--"):
			return i
		default:
			i++
		}
	}
	return -1
}

// indentedCode reports whether a line is indented by four or more spaces, or a tab. As in
// CommonMark, such a line is code: it is neither a heading nor a key.
func indentedCode(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " \t") || strings.HasPrefix(line, "  \t") || strings.HasPrefix(line, "   \t")
}

// stageName reports whether line is a `## name` heading.
func stageName(line string) (string, bool) {
	if !strings.HasPrefix(line, "## ") && strings.TrimRight(line, " \t") != "##" {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "##")), true
}

// keyLine splits `outputs: a, b` into its lower-case key and trimmed value.
func keyLine(t string) (key, val string, ok bool) {
	for _, k := range []string{"outputs", "review", "use"} {
		if len(t) > len(k) && strings.EqualFold(t[:len(k)], k) && t[len(k)] == ':' {
			return k, strings.TrimSpace(t[len(k)+1:]), true
		}
	}
	return "", "", false
}

func fenceMarker(t string) string {
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(t, m) {
			n := len(t) - len(strings.TrimLeft(t, m[:1]))
			return strings.Repeat(m[:1], n)
		}
	}
	return ""
}

// Standard returns the built-in flow.
func Standard() *Flow {
	f, err := Parse("standard flow", assets.StandardFlow)
	if err != nil {
		panic("the built-in flow does not parse: " + err.Error())
	}
	f.Source = SourceStandard
	return f
}

// Find returns the flow file to use and where it comes from: flow.md in projectDir, then
// flow.md in the cav home folder. It returns "", "standard" when neither exists.
func Find(projectDir string) (path, source string) {
	if p := filepath.Join(projectDir, File); isFile(p) {
		return p, SourceProject
	}
	if p := filepath.Join(config.Home(), File); isFile(p) {
		return p, SourceUser
	}
	return "", SourceStandard
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// Load returns the flow in effect for the project. A file that does not parse is an
// error; Load never falls back to the next file.
func Load(projectDir string) (*Flow, error) {
	path, source := Find(projectDir)
	if path == "" {
		return Standard(), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(path, b)
	if err != nil {
		return nil, err
	}
	f.Source, f.Path = source, path
	return f, nil
}

// Label says in words where the flow comes from.
func (f *Flow) Label() string {
	switch f.Source {
	case SourceProject:
		return "flow.md in this project"
	case SourceUser:
		if os.Getenv("CAV_HOME") != "" {
			return f.Path
		}
		return "~/.cav/flow.md"
	}
	return "standard (built in)"
}

// ReviewTarget is the file the user reviews for this stage: its first output, when that is
// one plain .json file. Otherwise it returns a problem to report, not an error.
func (s Stage) ReviewTarget() (target, problem string) { return s.ReviewTargetIn("") }

// ReviewTargetIn is ReviewTarget for a project folder. A .json file with another name than
// brief.json, moodboard.json or storyboard.json is a target only when it exists in root and
// cav review can open it (it holds the schema of one of them). With root "" it is a problem.
func (s Stage) ReviewTargetIn(root string) (target, problem string) {
	if len(s.Outputs) == 0 {
		return "", "the stage has no outputs:"
	}
	first := s.Outputs[0]
	switch ext := strings.ToLower(filepath.Ext(first)); {
	case strings.HasSuffix(first, "/"):
		return "", fmt.Sprintf("%s is a folder; list the .json file to review first, or set review: no", first)
	case strings.Contains(first, "*"):
		return "", fmt.Sprintf("%s is a pattern; list the .json file to review first, or set review: no", first)
	case ext == ".json":
		switch filepath.Base(first) {
		case "brief.json", "moodboard.json", "storyboard.json":
			return first, ""
		}
		// Another name works when the file exists and holds one of their schemas.
		if root != "" {
			if _, err := review.KindOf(filepath.Join(root, filepath.FromSlash(first))); err == nil {
				return first, ""
			}
		}
		return "", fmt.Sprintf("%s is not a file cav review can approve (brief.json, moodboard.json or storyboard.json, or a file with their schema); set review: no or list one of those first", first)
	case ext == ".mp4" || ext == ".mov" || ext == ".webm" || ext == ".mkv" || ext == ".gif":
		return "", fmt.Sprintf("%s is a video; a video review holds notes but no approval: set review: no, or list a .json file first", first)
	}
	return "", fmt.Sprintf("%s is not a .json file; list the .json file to review first, or set review: no", first)
}

// Warnings lists the stages that ask for a review that cannot hold an approval: a
// review: yes stage whose first output is not one .json file. They are not parse errors.
// Each text names the file, the line and the stage. root is the project folder, used to
// accept a .json file with another name that holds the schema of a reviewable file; "" skips
// that check.
func (f *Flow) Warnings(root string) []string {
	file := File
	if f.Path != "" {
		file = f.Path
	}
	out := []string{}
	if f.Unclosed > 0 {
		out = append(out, fmt.Sprintf("%s line %d: <!-- is never closed; the rest of the file is ignored", file, f.Unclosed))
	}
	for _, s := range f.Stages {
		if !s.Review || len(s.Outputs) == 0 {
			continue
		}
		if _, problem := s.ReviewTargetIn(root); problem != "" {
			out = append(out, fmt.Sprintf("%s line %d: stage %s: %s", file, s.Line, s.Name, problem))
		}
	}
	return out
}

// codeSpanEnd returns the index just after the first run of exactly n backticks at or after
// from, or -1.
func codeSpanEnd(line string, from, n int) int {
	for j := from; j < len(line); {
		if line[j] != '`' {
			j++
			continue
		}
		k := j
		for k < len(line) && line[k] == '`' {
			k++
		}
		if k-j == n {
			return k
		}
		j = k
	}
	return -1
}
