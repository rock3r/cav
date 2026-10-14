package board

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateRefusesShotIDsThatAreNotSimpleNames(t *testing.T) {
	for _, id := range []string{"../s1", "intro/s1", `a\b`, "s 1", "s1.png", ".", "..", "é", strings.Repeat("a", 65)} {
		sb := &Board{BPM: 120, Shots: []Shot{{ID: id, Beats: [2]float64{0, 4}}}}
		err := sb.Validate()
		if err == nil || !strings.Contains(err.Error(), "may only use letters, digits, - and _") || !strings.Contains(err.Error(), fmt.Sprintf("%q", id)) {
			t.Errorf("id %q: got %v", id, err)
		}
	}
	for _, id := range []string{"s1", "intro_2", "Shot-3", "A", strings.Repeat("a", 64)} {
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
	group := s[strings.Index(s, "<g transform="):strings.Index(s, "<path")]
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
