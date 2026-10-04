// Package search is a small BM25 ranker for the offline API reference and docs index.
package search

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Doc is one searchable unit. Fields are weighted: a hit in Name counts most.
type Doc struct {
	Name string // e.g. function name or page title
	// Title optionally prefers a complete page title at the start of the query.
	Title string
	Head  string // e.g. signature or section heading
	Body  string
	// Boost multiplies the score (1 when zero).
	Boost float64
}

type Hit struct {
	Index int
	Score float64
}

type Index struct {
	docs  []Doc
	tf    []map[string]float64
	lens  []float64
	avg   float64
	df    map[string]int
	names []map[string]bool
}

const (
	wName = 4.0
	wHead = 2.0
	wBody = 1.0
	k1    = 1.2
	b     = 0.75
)

func New(docs []Doc) *Index {
	ix := &Index{docs: docs, df: map[string]int{}}
	total := 0.0
	for _, d := range docs {
		tf := map[string]float64{}
		names := map[string]bool{}
		n := 0.0
		add := func(text string, w float64, isName bool) {
			for _, t := range Tokens(text) {
				tf[t] += w
				n += w
				if isName {
					names[t] = true
				}
			}
		}
		add(d.Name, wName, true)
		add(d.Head, wHead, false)
		add(d.Body, wBody, false)
		for t := range tf {
			ix.df[t]++
		}
		ix.tf = append(ix.tf, tf)
		ix.names = append(ix.names, names)
		ix.lens = append(ix.lens, n)
		total += n
	}
	if len(docs) > 0 {
		ix.avg = total / float64(len(docs))
	}
	return ix
}

// Search returns the best hits for the query, best first.
func (ix *Index) Search(query string, limit int) []Hit {
	q := Tokens(query)
	if len(q) == 0 {
		return nil
	}
	lowerQ := strings.ToLower(strings.TrimSpace(query))
	N := float64(len(ix.docs))
	hits := make([]Hit, 0, len(ix.docs))
	var maxScore float64
	for i := range ix.docs {
		s := 0.0
		matched := 0
		for _, t := range q {
			f := ix.tf[i][t]
			if f == 0 {
				continue
			}
			matched++
			idf := math.Log(1 + (N-float64(ix.df[t])+0.5)/(float64(ix.df[t])+0.5))
			s += idf * f * (k1 + 1) / (f + k1*(1-b+b*ix.lens[i]/ix.avg))
		}
		if s == 0 {
			continue
		}
		// Prefer documents that match every query word, and exact name matches.
		s *= 0.5 + 0.5*float64(matched)/float64(len(q))
		name := strings.ToLower(ix.docs[i].Name)
		if name == lowerQ || strings.ReplaceAll(name, " ", "") == strings.ReplaceAll(lowerQ, " ", "") {
			s *= 3
		}
		if ix.docs[i].Boost > 0 {
			s *= ix.docs[i].Boost
		}
		hits = append(hits, Hit{Index: i, Score: s})
		maxScore = math.Max(maxScore, s)
	}
	// Page-title intent takes precedence over contextual words in the body. Keep
	// longer complete titles ahead of shorter prefixes, then use BM25 within a
	// title. API entries omit Title and retain their existing ranking.
	queryTitle := normalizeTitle(query)
	for i := range hits {
		title := normalizeTitle(ix.docs[hits[i].Index].Title)
		if title != "" && (queryTitle == title || strings.HasPrefix(queryTitle, title+" ")) {
			hits[i].Score += float64(len(strings.Fields(title))) * (maxScore + 1)
		}
	}

	sort.SliceStable(hits, func(a, b int) bool { return hits[a].Score > hits[b].Score })
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func normalizeTitle(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// Tokens splits text into lower-case words, also splitting camelCase and dotted names,
// and keeps the joined form: "getBoundingBox" gives get, bounding, box, getboundingbox.
func Tokens(s string) []string {
	var out []string
	var word []rune
	flush := func() {
		if len(word) == 0 {
			return
		}
		parts := splitCamel(word)
		joined := strings.ToLower(string(word))
		for _, p := range parts {
			p = strings.ToLower(p)
			if len(p) > 1 && !stop[p] {
				out = append(out, stem(p))
			}
		}
		if len(parts) > 1 {
			out = append(out, joined)
		}
		word = word[:0]
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word = append(word, r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

func splitCamel(w []rune) []string {
	var parts []string
	start := 0
	for i := 1; i < len(w); i++ {
		if unicode.IsUpper(w[i]) && (unicode.IsLower(w[i-1]) || (i+1 < len(w) && unicode.IsLower(w[i+1]) && unicode.IsUpper(w[i-1]))) {
			parts = append(parts, string(w[start:i]))
			start = i
		}
	}
	return append(parts, string(w[start:]))
}

// stem is a tiny suffix stripper so "keyframes" finds "keyframe".
func stem(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 4 && strings.HasSuffix(w, "ing"):
		return w[:len(w)-3]
	case len(w) > 3 && strings.HasSuffix(w, "es") && strings.HasSuffix(w[:len(w)-2], "sh"):
		return w[:len(w)-2]
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}

var stop = map[string]bool{
	"the": true, "a": true, "an": true, "to": true, "of": true, "and": true, "or": true, "in": true,
	"on": true, "for": true, "is": true, "it": true, "this": true, "that": true, "with": true, "how": true,
	"do": true, "can": true, "be": true, "you": true, "your": true, "as": true, "by": true, "at": true,
	"from": true, "are": true, "will": true, "use": true, "what": true, "which": true, "if": true, "into": true,
}
