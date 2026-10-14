package board

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestValidateRefusesShotIDsThatCannotBeFileNames(t *testing.T) {
	for _, id := range []string{"../x", "a/b", `a\b`, "..", ".", ".hidden", "a\x00b", "a\nb", strings.Repeat("a", 65)} {
		sb := &Board{BPM: 120, Shots: []Shot{{ID: id, Beats: [2]float64{0, 4}}}}
		err := sb.Validate()
		if err == nil || !strings.Contains(err.Error(), "cannot be a file name") || !strings.Contains(err.Error(), fmt.Sprintf("%q", id)) {
			t.Errorf("id %q: got %v", id, err)
		}
	}
	// Older boards use dots, spaces and accents in ids.
	for _, id := range []string{"s1", "shot.1", "plan 2", "intro_2", "Shot-3", "é", "a.b.c", strings.Repeat("a", 64)} {
		sb := &Board{BPM: 120, Shots: []Shot{{ID: id, Beats: [2]float64{0, 4}}}}
		if err := sb.Validate(); err != nil {
			t.Errorf("id %q refused: %v", id, err)
		}
	}
}

func TestNormalizeSketchCopiesEveryInheritableRootAttribute(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 540" version="1.1" id="x" class="c" x="0" y="0" ` +
		`preserveAspectRatio="none" color="#ff0000" fill="currentColor" stroke-dasharray="20 14" stroke="#fff"><path d="M0 0L10 10"/></svg>`
	out, err := NormalizeSketch([]byte(in), 1920, 1080, "#000000")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	group := s[strings.LastIndex(s[:strings.Index(s, "<path")], "<g"):strings.Index(s, "<path")]
	for _, want := range []string{`color="#ff0000"`, `fill="currentColor"`, `stroke-dasharray="20 14"`, `stroke="#fff"`} {
		if !strings.Contains(group, want) {
			t.Errorf("the wrapper group lacks %s: %s", want, group)
		}
	}
	for _, not := range []string{` version=`, ` id=`, ` class=`, ` x=`, ` y=`, `preserveAspectRatio`, `viewBox="0 0 960`} {
		if strings.Contains(group, not) {
			t.Errorf("a structural attribute %s was copied: %s", not, group)
		}
	}
}

// uniqueAttributes fails the test when the SVG is not well-formed or an element repeats an attribute.
func uniqueAttributes(t *testing.T, svg string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(svg))
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("not well-formed: %v\n%s", err, svg)
		}
		if se, ok := tok.(xml.StartElement); ok {
			seen := map[xml.Name]bool{}
			for _, a := range se.Attr {
				if seen[a.Name] {
					t.Errorf("<%s> has %s twice:\n%s", se.Name.Local, a.Name.Local, svg)
				}
				seen[a.Name] = true
			}
		}
	}
}

func TestNormalizeSketchRootTransformGoesOnTheInnerGroup(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 540" transform="translate(10 10)" fill="#fff"><path d="M0 0L10 10"/></svg>`
	out, err := NormalizeSketch([]byte(in), 1920, 1080, "#000000")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	uniqueAttributes(t, s)
	outer := strings.Index(s, `<g transform="translate(-0 -0) scale(2 2)">`)
	inner := strings.Index(s, `<g transform="translate(10 10)" fill="#fff">`)
	if outer < 0 || inner < outer || inner > strings.Index(s, "<path") {
		t.Fatalf("want cav's group around a group with the root's transform and fill: %s", s)
	}
}

func TestNormalizeSketchNeverRepeatsAnAttribute(t *testing.T) {
	for _, in := range []string{
		goodSketch,
		`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 1920 1080" fill="red" fill="blue" transform="rotate(5)" style="opacity:.5"><g transform="scale(2)"><rect width="1" height="1" fill="#000"/></g></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540" stroke="#fff" stroke-dasharray="4 2"/>`,
	} {
		out, err := NormalizeSketch([]byte(in), 1920, 1080, "#101010")
		if err != nil {
			t.Fatal(err)
		}
		uniqueAttributes(t, string(out))
	}
}
