package board

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// MaxSketchBytes is the largest sketch SVG cav accepts. A sketch is a few simple shapes.
const MaxSketchBytes = 512 << 10

const (
	maxSketchElements = 2000
	maxSketchDepth    = 32
	maxSketchProblems = 10
	svgNS             = "http://www.w3.org/2000/svg"
	xmlURL            = "http://www.w3.org/XML/1998/namespace"
	xlinkURL          = "http://www.w3.org/1999/xlink"
)

// SketchError lists everything wrong with a sketch SVG, so one run shows all of it.
type SketchError struct{ Problems []string }

func (e *SketchError) Error() string { return strings.Join(e.Problems, "; ") }

// sketchElements are the SVG elements a sketch may use: shapes, groups, gradients and text.
var sketchElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true,
	"rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true, "path": true,
	"text": true, "tspan": true, "linearGradient": true, "radialGradient": true, "stop": true,
	"clipPath": true, "use": true,
}

// structuralRootAttrs describe the root element itself. Every other attribute that the check
// accepted on <svg> (fill, color, stroke-dasharray, font and so on) is inherited by the shapes
// inside, so the wrapper group carries it.
var structuralRootAttrs = map[string]bool{
	"xmlns": true, "width": true, "height": true, "viewBox": true, "preserveAspectRatio": true,
	"version": true, "x": true, "y": true, "id": true, "class": true,
}

// AspectLabel names the standard aspect ratio closest to w:h, such as "16:9".
func AspectLabel(w, h int) string {
	r := float64(w) / float64(h)
	best, diff := "16:9", 9.0
	for _, a := range []struct {
		s string
		v float64
	}{{"16:9", 16.0 / 9}, {"9:16", 9.0 / 16}, {"1:1", 1}, {"4:3", 4.0 / 3}, {"3:4", 3.0 / 4}, {"3:2", 1.5}, {"2:3", 2.0 / 3}, {"21:9", 21.0 / 9}, {"4:5", 0.8}, {"5:4", 1.25}} {
		if d := math.Abs(a.v - r); d < diff {
			best, diff = a.s, d
		}
	}
	return best
}

// ReadSketch reads a sketch file. It refuses a symbolic link, anything but a regular file,
// and a file over MaxSketchBytes, before reading it. A missing file returns the os error.
func ReadSketch(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return nil, &SketchError{[]string{"it is a symbolic link: copy the file instead"}}
	case !fi.Mode().IsRegular():
		return nil, &SketchError{[]string{"it is not a regular file"}}
	case fi.Size() > MaxSketchBytes:
		return nil, &SketchError{[]string{fmt.Sprintf("it is %d KiB; a sketch is at most %d KiB: use fewer, simpler shapes", fi.Size()>>10, MaxSketchBytes>>10)}}
	}
	return os.ReadFile(path)
}

type sketchInfo struct {
	vbX, vbY, vbW, vbH float64
	xmlns              []xml.Attr // the root's xmlns:* declarations
	rootPresentation   []xml.Attr // the root's fill, stroke, font and similar attributes
	innerStart         int64
	innerEnd           int64
}

// CheckSketch refuses an SVG that is not a plain drawing of simple shapes for a w x h storyboard.
// It does not clean the file up: the agent that wrote it fixes it. The error is a *SketchError.
func CheckSketch(data []byte, w, h int) error {
	_, err := analyzeSketch(data, w, h)
	return err
}

func analyzeSketch(data []byte, w, h int) (*sketchInfo, error) {
	var problems []string
	add := func(line int, format string, args ...any) {
		if len(problems) == maxSketchProblems {
			problems = append(problems, "and more")
		}
		if len(problems) > maxSketchProblems {
			return
		}
		msg := fmt.Sprintf(format, args...)
		if line > 0 {
			msg = fmt.Sprintf("line %d: %s", line, msg)
		}
		problems = append(problems, msg)
	}
	finish := func(info *sketchInfo) (*sketchInfo, error) {
		if len(problems) > 0 {
			return nil, &SketchError{problems}
		}
		return info, nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		add(0, "the file is empty")
		return finish(nil)
	}
	if len(data) > MaxSketchBytes {
		add(0, "it is %d KiB; a sketch is at most %d KiB: use fewer, simpler shapes", len(data)>>10, MaxSketchBytes>>10)
		return finish(nil)
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	info := &sketchInfo{}
	depth, elements := 0, 0
	rootSeen := false
	xlinkOnRoot := false
	for {
		prev := d.InputOffset()
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		line, _ := d.InputPos()
		if err != nil {
			add(line, "the SVG is not well-formed XML: %v", err)
			break
		}
		switch t := tok.(type) {
		case xml.Directive:
			add(line, "<!%s…> is not allowed in a sketch (no DOCTYPE or ENTITY)", firstWord(string(t)))
		case xml.ProcInst:
			switch {
			case t.Target != "xml":
				add(line, "<?%s?> is not allowed in a sketch", t.Target)
			case rootSeen:
				add(line, "<?xml?> is allowed only before the <svg> element")
			}
		case xml.CharData:
			if info.innerEnd > 0 && len(bytes.TrimSpace(t)) > 0 {
				add(line, "nothing may follow the closing </svg>")
			}
		case xml.StartElement:
			if info.innerEnd > 0 {
				add(line, "nothing may follow the closing </svg>")
				break
			}
			depth++
			elements++
			name := t.Name.Local
			switch {
			case elements == 1:
				rootSeen = true
				if name != "svg" {
					add(line, "the root element must be <svg>, not <%s>", name)
				}
				if t.Name.Space != "" && t.Name.Space != svgNS {
					add(line, "the root element is not in the SVG namespace")
				}
				info.innerStart = d.InputOffset()
				checkCanvas(t, w, h, info, func(f string, a ...any) { add(line, f, a...) })
				for _, a := range t.Attr {
					if a.Name.Space == "xmlns" {
						info.xmlns = append(info.xmlns, a)
						if a.Name.Local == "xlink" && a.Value == xlinkURL {
							xlinkOnRoot = true
						}
					}
					// The attributes were checked above like any other element's.
					if a.Name.Space == "" && !structuralRootAttrs[a.Name.Local] && !hasAttr(info.rootPresentation, a.Name.Local) {
						info.rootPresentation = append(info.rootPresentation, a)
					}
				}
			case name == "svg":
				add(line, "a nested <svg> is not allowed in a sketch")
			case t.Name.Space != "" && t.Name.Space != svgNS:
				add(line, "<%s> is not allowed in a sketch: use simple shapes", name)
			case !sketchElements[name]:
				add(line, "<%s> is not allowed in a sketch: use simple shapes", name)
			}
			if elements == maxSketchElements+1 {
				add(line, "more than %d elements: draw fewer, simpler shapes", maxSketchElements)
			}
			if depth == maxSketchDepth+1 {
				add(line, "groups are nested more than %d deep", maxSketchDepth)
			}
			for _, a := range t.Attr {
				if msg := prefixedAttrProblem(a, elements == 1, xlinkOnRoot); msg != "" {
					add(line, "%s on <%s>", msg, name)
					continue
				}
				checkAttr(a, name, func(f string, args ...any) { add(line, f, args...) })
			}
		case xml.EndElement:
			depth--
			if depth == 0 && info.innerEnd == 0 {
				info.innerEnd = prev
			}
		}
	}
	if !rootSeen && len(problems) == 0 {
		add(0, "the file has no <svg> element")
	}
	return finish(info)
}

func hasAttr(attrs []xml.Attr, name string) bool {
	for _, a := range attrs {
		if a.Name.Local == name {
			return true
		}
	}
	return false
}

func firstWord(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

var forbiddenValue = regexp.MustCompile(`(?i)javascript:|@import|expression\s*\(|data:text/html`)
var urlCall = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*(.?)`)

// prefixedAttrProblem allows only these prefixed attributes: xmlns:xlink on <svg> with the real
// XLink URI, xlink:* when that declaration is on <svg>, and xml:space and xml:lang. Sketches are
// hand-written SVG, so nothing else is needed. The decoder puts the namespace URL (or, when no
// xmlns declares the prefix, the prefix itself) in Name.Space. It returns "" when the attribute is fine.
func prefixedAttrProblem(a xml.Attr, isRoot, xlinkOnRoot bool) string {
	switch a.Name.Space {
	case "":
		return ""
	case "xmlns":
		if isRoot && a.Name.Local == "xlink" && a.Value == xlinkURL {
			return ""
		}
		return "xmlns:" + a.Name.Local + " is not allowed (only xmlns:xlink=\"" + xlinkURL + "\" on <svg>)"
	case xmlURL:
		if a.Name.Local == "space" || a.Name.Local == "lang" {
			return ""
		}
	case xlinkURL:
		if xlinkOnRoot {
			return ""
		}
	}
	return "a prefixed attribute (" + a.Name.Local + ") is not allowed (only xlink:*, with xmlns:xlink declared on <svg>, xml:space and xml:lang)"
}

// checkAttr refuses event handlers, links to other files, scripts and external urls.
func checkAttr(a xml.Attr, element string, bad func(string, ...any)) {
	n := strings.ToLower(a.Name.Local)
	if strings.HasPrefix(n, "on") && a.Name.Space != "xmlns" {
		bad("%s=… on <%s> is not allowed: a sketch has no scripts", a.Name.Local, element)
		return
	}
	if n == "href" && !strings.HasPrefix(strings.TrimSpace(a.Value), "#") {
		bad("href on <%s> may only point at an id in the same file (#name)", element)
		return
	}
	if forbiddenValue.MatchString(a.Value) {
		bad("%s on <%s> holds a script or an import, which is not allowed", a.Name.Local, element)
		return
	}
	for _, m := range urlCall.FindAllStringSubmatch(a.Value, -1) {
		if m[1] != "#" {
			bad("url(…) in %s on <%s> may only point at an id in the same file (url(#name))", a.Name.Local, element)
			return
		}
	}
}

func checkCanvas(root xml.StartElement, w, h int, info *sketchInfo, bad func(string, ...any)) {
	attr := func(name string) (string, bool) {
		for _, a := range root.Attr {
			if a.Name.Space == "" && a.Name.Local == name {
				return a.Value, true
			}
		}
		return "", false
	}
	fix := fmt.Sprintf("use viewBox=\"0 0 %d %d\"", w, h)
	if vb, ok := attr("viewBox"); ok {
		f := strings.FieldsFunc(vb, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r' })
		var v [4]float64
		valid := len(f) == 4
		for i := 0; valid && i < 4; i++ {
			x, err := strconv.ParseFloat(f[i], 64)
			valid = err == nil && !math.IsNaN(x) && !math.IsInf(x, 0)
			v[i] = x
		}
		if !valid || v[2] <= 0 || v[3] <= 0 {
			bad("viewBox needs four numbers with a positive width and height: %s", fix)
			return
		}
		info.vbX, info.vbY, info.vbW, info.vbH = v[0], v[1], v[2], v[3]
	} else {
		ws, wok := attr("width")
		hs, hok := attr("height")
		pw, perr1 := parseSketchLength(ws)
		ph, perr2 := parseSketchLength(hs)
		if !wok || !hok || perr1 != nil || perr2 != nil || pw <= 0 || ph <= 0 {
			bad("the <svg> has no viewBox and no numeric width and height: %s", fix)
			return
		}
		info.vbW, info.vbH = pw, ph
	}
	want, got := float64(w)/float64(h), info.vbW/info.vbH
	if math.Abs(got-want)/want > 0.01 {
		bad("the canvas is %s but the storyboard is %s: %s", AspectLabel(int(math.Round(info.vbW*1000)), int(math.Round(info.vbH*1000))), AspectLabel(w, h), fix)
	}
}

// parseSketchLength reads a number with no unit, or in px.
func parseSketchLength(s string) (float64, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "px")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("not a number")
	}
	return v, nil
}

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// SketchBackground is the colour a sketch is drawn on: the first colour of the style palette,
// or black when the palette is empty or its first colour is not #rrggbb.
func SketchBackground(palette []string) string {
	if len(palette) > 0 && hexColour.MatchString(palette[0]) {
		return palette[0]
	}
	return "#000000"
}

// NormalizeSketch rewrites a checked sketch into the form cav imports into Cavalry: an SVG
// with width and height w x h, viewBox "0 0 w h" and a background rectangle first, so the
// drawing lands 1:1 in a w x h composition. It checks the sketch first. The result is a copy;
// the agent's file is not changed.
func NormalizeSketch(data []byte, w, h int, bg string) ([]byte, error) {
	if !hexColour.MatchString(bg) {
		return nil, fmt.Errorf("background %q is not a #rrggbb colour", bg)
	}
	info, err := analyzeSketch(data, w, h)
	if err != nil {
		return nil, err
	}
	sx, sy := float64(w)/info.vbW, float64(h)/info.vbH
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns=%q`, svgNS)
	for _, a := range info.xmlns {
		var v bytes.Buffer
		_ = xml.EscapeText(&v, []byte(a.Value))
		fmt.Fprintf(&b, ` xmlns:%s="%s"`, a.Name.Local, v.String())
	}
	fmt.Fprintf(&b, ` width="%d" height="%d" viewBox="0 0 %d %d">`, w, h, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`, w, h, bg)
	// The outer group carries only cav's transform. The inner group carries the root's own
	// attributes (a root transform included), so no element has the same attribute twice.
	fmt.Fprintf(&b, `<g transform="translate(%s %s) scale(%s %s)"><g`, num(-info.vbX*sx), num(-info.vbY*sy), num(sx), num(sy))
	for _, a := range info.rootPresentation {
		var v bytes.Buffer
		_ = xml.EscapeText(&v, []byte(a.Value))
		fmt.Fprintf(&b, ` %s="%s"`, a.Name.Local, v.String())
	}
	b.WriteString(">")
	if info.innerEnd > info.innerStart {
		b.Write(data[info.innerStart:info.innerEnd])
	}
	b.WriteString("</g></g></svg>\n")
	return b.Bytes(), nil
}
