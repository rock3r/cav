package board

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const goodSketch = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080">
  <rect x="100" y="100" width="300" height="200" fill="#f5f7fa"/>
  <circle cx="900" cy="500" r="80" fill="#ff4d2e"/>
</svg>`

func wrap(inner string) []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080">` + inner + `</svg>`)
}

func problems(t *testing.T, data []byte) []string {
	t.Helper()
	err := CheckSketch(data, 1920, 1080)
	if err == nil {
		return nil
	}
	var se *SketchError
	if !errors.As(err, &se) {
		t.Fatalf("not a SketchError: %v", err)
	}
	return se.Problems
}

func TestCheckSketchAcceptsSimpleShapes(t *testing.T) {
	if p := problems(t, []byte(goodSketch)); p != nil {
		t.Fatalf("valid sketch refused: %v", p)
	}
	ok := wrap(`<defs><linearGradient id="a"><stop offset="0" stop-color="#000"/></linearGradient></defs>` +
		`<rect id="r" width="10" height="10" fill="url(#a)" style="stroke:url(#a)"/><use href="#r"/>`)
	if p := problems(t, ok); p != nil {
		t.Fatalf("gradients and local references refused: %v", p)
	}
}

func TestCheckSketchRefusals(t *testing.T) {
	big := wrap(strings.Repeat(`<rect width="1" height="1"/>`, 25000))
	deep := wrap(strings.Repeat("<g>", 40) + strings.Repeat("</g>", 40))
	many := wrap(strings.Repeat(`<rect width="1" height="1"/>`, 2100))
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", []byte("  "), "empty"},
		{"too big", big, "at most 512 KiB"},
		{"doctype and entity", []byte(`<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY x "y">]><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"/>`), "DOCTYPE"},
		{"script", wrap(`<script>alert(1)</script>`), "<script> is not allowed"},
		{"foreignObject", wrap(`<foreignObject/>`), "<foreignObject> is not allowed"},
		{"image", wrap(`<image href="http://example.com/a.png"/>`), "<image> is not allowed in a sketch: use simple shapes"},
		{"use of a file", xlinkDoc(`<use xlink:href="file.svg#a"/>`), "href on <use>"},
		{"event handler", wrap(`<rect onload="x()" width="1" height="1"/>`), "onload"},
		{"external url", wrap(`<rect style="fill:url(http://x/a)" width="1" height="1"/>`), "url(#name)"},
		{"css import", wrap(`<rect style="@import 'x'" width="1" height="1"/>`), "script or an import"},
		{"javascript", wrap(`<rect fill="javascript:x" width="1" height="1"/>`), "script or an import"},
		{"stylesheet instruction", []byte(`<?xml-stylesheet href="a.css"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"/>`), "xml-stylesheet"},
		{"nested svg", wrap(`<svg/>`), "nested <svg>"},
		{"wrong root", []byte(`<html/>`), "root element must be <svg>"},
		{"1:1 canvas", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 500 500"/>`), `the canvas is 1:1 but the storyboard is 16:9: use viewBox="0 0 1920 1080"`},
		{"no canvas", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "no viewBox and no numeric width and height"},
		{"units", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10cm" height="5cm"/>`), "no viewBox"},
		{"deep", deep, "nested more than 32"},
		{"many elements", many, "more than 2000 elements"},
		{"malformed", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"><rect></svg>`), "not well-formed"},
		{"unknown entity", wrap(`<title>&nbsp;</title>`), "not well-formed"},
	}
	for _, c := range cases {
		p := problems(t, c.data)
		if len(p) == 0 {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		if !strings.Contains(strings.Join(p, "\n"), c.want) {
			t.Errorf("%s: problems %v lack %q", c.name, p, c.want)
		}
	}
}

func TestCheckSketchAcceptsWidthAndHeight(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="960px" height="540"/>`)
	if p := problems(t, data); p != nil {
		t.Fatalf("width and height refused: %v", p)
	}
}

func TestCheckSketchListsSeveralProblemsWithLines(t *testing.T) {
	data := []byte("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 1920 1080\">\n<script/>\n<image/>\n<rect onclick=\"x\"/>\n</svg>")
	p := problems(t, data)
	if len(p) != 3 {
		t.Fatalf("problems: %v", p)
	}
	for i, line := range []string{"line 2:", "line 3:", "line 4:"} {
		if !strings.HasPrefix(p[i], line) {
			t.Errorf("problem %d = %q, want prefix %q", i, p[i], line)
		}
	}
}

func TestCheckSketchCapsTheProblemList(t *testing.T) {
	p := problems(t, wrap(strings.Repeat(`<image/>`, 30)))
	if len(p) != maxSketchProblems+1 || p[len(p)-1] != "and more" {
		t.Fatalf("got %d problems: %v", len(p), p)
	}
}

func TestNormalizeSketch(t *testing.T) {
	out, err := NormalizeSketch([]byte(goodSketch), 1920, 1080, "#0b0d12")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if err := CheckSketch(out, 1920, 1080); err != nil {
		t.Fatalf("the normalized sketch does not pass the check: %v\n%s", err, s)
	}
	for _, want := range []string{`width="1920" height="1080" viewBox="0 0 1920 1080"`, `translate(-0 -0) scale(1 1)`, `<circle cx="900"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
	if strings.Index(s, `fill="#0b0d12"`) > strings.Index(s, "<rect x=") {
		t.Error("the background is not drawn first")
	}
	if strings.Contains(s, "<?xml") {
		t.Error("the XML declaration was kept")
	}
}

func TestNormalizeSketchScalesAndKeepsNamespaces(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 960 540">` +
		`<rect id="a" width="10" height="10"/><use xlink:href="#a"/></svg>`
	out, err := NormalizeSketch([]byte(in), 1920, 1080, "#000000")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`xmlns:xlink="http://www.w3.org/1999/xlink"`, `scale(2 2)`, `<use xlink:href="#a"/>`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
	if err := CheckSketch(out, 1920, 1080); err != nil {
		t.Fatalf("%v\n%s", err, s)
	}
}

func TestNormalizeSketchOffsetViewBoxAndSelfClosingRoot(t *testing.T) {
	out, err := NormalizeSketch([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="100 50 960 540"/>`), 1920, 1080, "#ffffff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "translate(-200 -100) scale(2 2)") {
		t.Errorf("offset not applied: %s", out)
	}
	if _, err := NormalizeSketch([]byte(goodSketch), 1920, 1080, "red"); err == nil {
		t.Error("a background that is not hex was accepted")
	}
	if _, err := NormalizeSketch(wrap(`<script/>`), 1920, 1080, "#000000"); err == nil {
		t.Error("an invalid sketch was normalized")
	}
}

func TestSketchBackground(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{{nil, "#000000"}, {[]string{"#0b0d12", "#fff"}, "#0b0d12"}, {[]string{"red"}, "#000000"}, {[]string{"#fff"}, "#000000"}} {
		if got := SketchBackground(c.in); got != c.want {
			t.Errorf("%v: got %s, want %s", c.in, got, c.want)
		}
	}
}

func TestReadSketch(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "a.svg")
	if err := os.WriteFile(good, []byte(goodSketch), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSketch(good); err != nil {
		t.Fatal(err)
	}
	huge := filepath.Join(dir, "huge.svg")
	if err := os.WriteFile(huge, make([]byte, MaxSketchBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSketch(huge); err == nil || !strings.Contains(err.Error(), "at most 512 KiB") {
		t.Errorf("big file: %v", err)
	}
	if _, err := ReadSketch(dir); err == nil {
		t.Error("a folder was read")
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.svg")
		if err := os.Symlink(good, link); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadSketch(link); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Errorf("symlink: %v", err)
		}
	}
	if _, err := ReadSketch(filepath.Join(dir, "none.svg")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: %v", err)
	}
}

func TestNormalizeSketchKeepsRootPresentationAttributes(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 540" fill="#ffffff" stroke="#ff0000" stroke-width="4" ` +
		`font-family="Inter" font-size="40" opacity="0.5" style="fill-opacity:0.8" id="x"><path d="M0 0L10 10"/></svg>`
	out, err := NormalizeSketch([]byte(in), 1920, 1080, "#000000")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	i := strings.Index(s, "<g transform=")
	j := strings.Index(s, "<path")
	if i < 0 || j < i {
		t.Fatalf("no wrapper group before the path: %s", s)
	}
	group := s[i:j]
	for _, want := range []string{`fill="#ffffff"`, `stroke="#ff0000"`, `stroke-width="4"`, `font-family="Inter"`, `font-size="40"`, `opacity="0.5"`, `style="fill-opacity:0.8"`} {
		if !strings.Contains(group, want) {
			t.Errorf("the wrapper group lacks %s: %s", want, group)
		}
	}
	if strings.Contains(group, `id="x"`) {
		t.Error("an attribute that is not presentation was copied")
	}
	// The background rectangle is outside the group, so it keeps its own fill.
	if strings.Index(s, `fill="#000000"`) > i {
		t.Error("the background is not drawn before the group")
	}
	if err := CheckSketch(out, 1920, 1080); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeSketchDoesNotCarryUnsafeRootStyle(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 540" style="fill:url(http://x/a)"/>`
	if _, err := NormalizeSketch([]byte(in), 1920, 1080, "#000000"); err == nil {
		t.Fatal("an unsafe root style was accepted")
	}
}

func TestCheckSketchAllowsDashes(t *testing.T) {
	if p := problems(t, wrap(`<line x1="0" y1="0" x2="9" y2="9" stroke="#fff" stroke-dasharray="20 14"/>`)); p != nil {
		t.Fatalf("stroke-dasharray refused: %v", p)
	}
}

func TestCheckSketchRefusesAnythingAfterTheRoot(t *testing.T) {
	root := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"></svg>`
	for _, tail := range []string{"<rect/>", "<svg/>", "text", "<g></g>"} {
		in := []byte(root + "\n" + tail)
		p := problems(t, in)
		if len(p) == 0 || !strings.Contains(strings.Join(p, "\n"), "line 2: nothing may follow the closing </svg>") {
			t.Errorf("tail %q: problems %v", tail, p)
		}
		if out, err := NormalizeSketch(in, 1920, 1080, "#000000"); err == nil {
			t.Errorf("tail %q: NormalizeSketch ran on it: %s", tail, out)
		}
	}
	// Whitespace and comments after the root are fine.
	if p := problems(t, []byte(root+"\n<!-- end -->\n  \n")); p != nil {
		t.Errorf("a comment after the root was refused: %v", p)
	}
}

func xlinkDoc(inner string) []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 1920 1080">` + inner + `</svg>`)
}

func TestCheckSketchAllowsOnlyXlinkAndXMLPrefixedAttributes(t *testing.T) {
	if p := problems(t, xlinkDoc(`<rect id="a" width="1" height="1"/><use xlink:href="#a"/><text xml:space="preserve" xml:lang="en">a</text>`)); p != nil {
		t.Errorf("allowed attributes refused: %v", p)
	}
	for name, in := range map[string][]byte{
		"undeclared prefix":      wrap(`<path foo:bar="x"/>`),
		"declared on a child":    []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"><g xmlns:q="urn:q"><path q:x="1"/></g></svg>`),
		"declared on the root":   []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:q="urn:q" viewBox="0 0 1920 1080"><path q:x="1"/></svg>`),
		"xlink on a child":       wrap(`<use xlink:href="#a" xmlns:xlink="http://www.w3.org/1999/xlink"/>`),
		"xlink with another URI": []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="urn:other" viewBox="0 0 1920 1080"><use xlink:href="#a"/></svg>`),
		"xlink undeclared":       wrap(`<use xlink:href="#a"/>`),
		"other xml attribute":    wrap(`<path xml:base="http://x/"/>`),
	} {
		if p := problems(t, in); len(p) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCheckSketchRefusesProcessingInstructionsAfterTheRootStarts(t *testing.T) {
	ok := `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"/>`
	if p := problems(t, []byte(ok)); p != nil {
		t.Errorf("<?xml?> before the root refused: %v", p)
	}
	for name, in := range map[string]string{
		"inside":   `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"><?xml version="1.0"?></svg>`,
		"after":    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"/><?xml version="1.0"?>`,
		"other PI": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080"><?php x ?></svg>`,
	} {
		if p := problems(t, []byte(in)); len(p) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCheckSketchRefusesPercentagesExceptWhereTheyAreNotLengths(t *testing.T) {
	for _, attr := range []string{`width="50%"`, `x="10%"`, `dx="10%"`, `dy="1%"`, `cx="50%"`, `r="5%"`, `stroke-width="1%"`, `font-size="5%"`, `y2="100%"`,
		`style="stroke-width:5%"`, `style="fill:#fff;font-size: 120%"`, `stroke-dasharray="5% 2"`, `transform="translate(10%)"`} {
		p := problems(t, wrap(`<rect `+attr+` height="1"/>`))
		if len(p) == 0 || !strings.Contains(strings.Join(p, "\n"), "use numbers, not percentages") {
			t.Errorf("%s: problems %v", attr, p)
		}
	}
	// A percentage on the root (other than its own size) is refused too, since the group copies it.
	if p := problems(t, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080" font-size="5%"/>`)); len(p) == 0 {
		t.Error("a root font-size percentage was accepted")
	}
	for _, ok := range []string{
		`<defs><linearGradient id="g"><stop offset="50%" stop-color="#000" stop-opacity="50%"/></linearGradient></defs><rect width="10" height="10" opacity="50%" fill-opacity="20%" stroke-opacity="5%"/>`,
		`<rect width="10" height="10" style="opacity:50%;fill-opacity:20%;fill:#fff"/>`,
	} {
		if p := problems(t, wrap(ok)); p != nil {
			t.Errorf("allowed percentages refused: %v\n%s", p, ok)
		}
	}
}
