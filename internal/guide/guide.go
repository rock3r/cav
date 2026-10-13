// Package guide splits the Markdown guides that `cav guide` prints into sections and
// searches them, so an agent can look up a symptom without knowing the topic name.
package guide

import (
	"io/fs"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rock3r/cav/internal/search"
)

// Section is one searchable part of a guide: the text under a heading, or one row of a
// table (with the table's header, so the row keeps its column names).
type Section struct {
	Topic string `json:"topic"`
	// Path is the heading path inside the topic, for example ["Keyframes and easing"]. A
	// table row adds its first cell as the last element.
	Path    []string `json:"path"`
	Text    string   `json:"text"`
	Aliases []string `json:"-"`
	heading string
	label   string
}

// aliasRe matches the alias comments in the guides: <!-- aliases: spin, dropped keys -->.
// They add search words that the text does not use, and `cav guide` does not print them.
var aliasRe = regexp.MustCompile(`[ \t]*<!--\s*aliases:\s*(.*?)\s*-->`)

// StripAliases removes the alias comments, and the lines that only hold one.
func StripAliases(md string) string {
	if !strings.Contains(md, "<!--") {
		return md
	}
	lines := strings.SplitAfter(md, "\n")
	var b strings.Builder
	for _, l := range lines {
		if !aliasRe.MatchString(l) {
			b.WriteString(l)
			continue
		}
		s := aliasRe.ReplaceAllString(l, "")
		if strings.TrimSpace(s) == "" {
			continue
		}
		b.WriteString(s)
	}
	return b.String()
}

func aliases(s string) []string {
	var out []string
	for _, m := range aliasRe.FindAllStringSubmatch(s, -1) {
		for _, a := range strings.Split(m[1], ",") {
			if a = strings.TrimSpace(a); a != "" {
				out = append(out, a)
			}
		}
	}
	return out
}

// Split cuts a guide into sections at its headings. Lines in fenced code blocks are never
// headings. The text before the first second-level heading is a section under the title.
// A table becomes one section per row; the other lines under the heading stay together.
func Split(topic, md string) []Section {
	var out []Section
	var path []string // path[0] is the level-1 title, if any
	var levels []int
	var body []string
	headingLine := ""

	flush := func() {
		p := sectionPath(path)
		heading := ""
		if len(p) > 0 {
			heading = p[len(p)-1]
		}
		var prose []string
		if headingLine != "" {
			prose = append(prose, headingLine)
		}
		for i := 0; i < len(body); {
			if !isTableLine(body[i]) {
				prose = append(prose, body[i])
				i++
				continue
			}
			j := i
			for j < len(body) && isTableLine(body[j]) {
				j++
			}
			table := body[i:j]
			if len(table) >= 3 && isSeparator(table[1]) {
				head := table[0] + "\n" + table[1]
				for _, row := range table[2:] {
					label := rowLabel(row)
					s := Section{
						Topic:   topic,
						Path:    append(append([]string{}, p...), label),
						Text:    aliasRe.ReplaceAllString(head+"\n"+row, ""),
						Aliases: aliases(row),
						heading: heading,
						label:   label,
					}
					out = append(out, s)
				}
			} else {
				prose = append(prose, table...)
			}
			i = j
		}
		text := strings.Trim(strings.Join(prose, "\n"), "\n")
		if strings.TrimSpace(strings.TrimPrefix(text, headingLine)) != "" {
			out = append(out, Section{
				Topic:   topic,
				Path:    p,
				Text:    strings.Trim(StripAliases(text), "\n"),
				Aliases: aliases(text),
				heading: heading,
			})
		}
		body = body[:0]
	}

	fence := ""
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
			body = append(body, line)
			continue
		}
		if f := fenceMarker(t); f != "" {
			fence = f
			body = append(body, line)
			continue
		}
		level, title := heading(line)
		if level == 0 {
			body = append(body, line)
			continue
		}
		flush()
		for len(levels) > 0 && levels[len(levels)-1] >= level {
			levels = levels[:len(levels)-1]
			path = path[:len(path)-1]
		}
		levels = append(levels, level)
		path = append(path, title)
		headingLine = line
		if level == 1 {
			// The title is not repeated in the intro section's text.
			headingLine = ""
		}
	}
	flush()
	return out
}

// sectionPath drops the level-1 title, except for the intro, where it is all there is.
func sectionPath(path []string) []string {
	if len(path) > 1 {
		return append([]string{}, path[1:]...)
	}
	return append([]string{}, path...)
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

func heading(line string) (int, string) {
	n := len(line) - len(strings.TrimLeft(line, "#"))
	if n == 0 || n > 6 || len(line) == n || line[n] != ' ' {
		return 0, ""
	}
	return n, strings.TrimSpace(line[n:])
}

func isTableLine(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), "|") }

func isSeparator(l string) bool {
	return strings.Trim(strings.TrimSpace(l), "|-: ") == ""
}

// rowLabel is a row's first cell without Markdown, shortened to a readable length.
func rowLabel(row string) string {
	cells := strings.Split(strings.Trim(strings.TrimSpace(row), "|"), "|")
	s := strings.TrimSpace(aliasRe.ReplaceAllString(cells[0], ""))
	s = strings.NewReplacer("`", "", "**", "").Replace(s)
	s = strings.TrimSuffix(s, ".")
	if utf8.RuneCountInString(s) > 70 {
		r := []rune(s)
		s = strings.TrimSpace(string(r[:69])) + "…"
	}
	return s
}

// Index ranks guide sections with the same BM25 ranker as `cav api` and `cav docs`.
type Index struct {
	Sections []Section
	ix       *search.Index
}

// Load reads <dir>/<topic>.md for each topic from fsys and indexes the sections.
func Load(fsys fs.FS, dir string, topics []string) (*Index, error) {
	var all []Section
	for _, t := range topics {
		b, err := fs.ReadFile(fsys, dir+"/"+t+".md")
		if err != nil {
			return nil, err
		}
		all = append(all, Split(t, string(b))...)
	}
	return New(all), nil
}

// New indexes sections. The heading counts most, then the topic, the row's first cell
// and the aliases, then the text. A query that starts with a topic name prefers that topic.
func New(sections []Section) *Index {
	docs := make([]search.Doc, len(sections))
	for i, s := range sections {
		docs[i] = search.Doc{
			Name:  s.heading,
			Title: s.Topic,
			Head:  s.Topic + " " + s.label + " " + strings.Join(s.Aliases, " "),
			Body:  s.Text,
		}
	}
	return &Index{Sections: sections, ix: search.New(docs)}
}

// Hit is a ranked section.
type Hit struct {
	Section
	Score     float64 `json:"score"`
	Truncated bool    `json:"truncated,omitempty"`
}

// Search returns the best n sections for the query, best first.
func (ix *Index) Search(query string, n int) []Hit {
	var out []Hit
	for _, h := range ix.ix.Search(query, n) {
		out = append(out, Hit{Section: ix.Sections[h.Index], Score: float64(int(h.Score*100)) / 100})
	}
	return out
}

// Cap limits each hit's text to perHit bytes and all texts together to total bytes, so a
// search can never print as much as the guides it replaces. Text is cut at a line end
// where possible, and a cut hit says where the rest is. Hits that no longer fit are
// dropped; the second result counts them.
func Cap(hits []Hit, perHit, total int) ([]Hit, int) {
	var out []Hit
	left := total
	for i, h := range hits {
		limit := min(perHit, left)
		if len(out) > 0 && limit < 200 {
			return out, len(hits) - i
		}
		if len(h.Text) > limit {
			note := "\n… (the rest is in `cav guide " + h.Topic + "`)"
			h.Text = cut(h.Text, max(limit-len(note), 0)) + note
			h.Truncated = true
		}
		left -= len(h.Text)
		out = append(out, h)
	}
	return out, 0
}

// cut shortens s to at most n bytes, at the last line end if there is one in the second
// half, otherwise at a rune boundary.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	if i := strings.LastIndexByte(s, '\n'); i > n/2 {
		return s[:i]
	}
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
