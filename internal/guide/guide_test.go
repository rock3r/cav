package guide

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rock3r/cav/assets"
)

var topics = []string{"workflow", "design", "music", "traps", "native", "production"}

const sample = "# Title\n\nIntro text.\n\n## Keys\n\nSome prose.\n\n" +
	"| Trap | Symptom |\n|---|---|\n" +
	"| Keys on `'rotation'`. | Silently no keys. <!-- aliases: spin, dropped --> |\n" +
	"| Colour keys. | Ignored. |\n\n" +
	"### Sub\n\n```sh\n# not a heading\ncav run x\n```\n\n## Empty\n"

func TestSplitByHeadingAndTableRow(t *testing.T) {
	got := Split("traps", sample)
	type want struct{ path, text string }
	wants := []want{
		{"Title", "Intro text."},
		{"Keys > Keys on 'rotation'", "| Trap | Symptom |\n|---|---|\n| Keys on `'rotation'`. | Silently no keys. |"},
		{"Keys > Colour keys", "| Trap | Symptom |\n|---|---|\n| Colour keys. | Ignored. |"},
		{"Keys", "## Keys\n\nSome prose."},
		{"Keys > Sub", "### Sub\n\n```sh\n# not a heading\ncav run x\n```"},
	}
	if len(got) != len(wants) {
		t.Fatalf("got %d sections, want %d: %+v", len(got), len(wants), got)
	}
	for i, w := range wants {
		if p := strings.Join(got[i].Path, " > "); p != w.path {
			t.Errorf("section %d path = %q, want %q", i, p, w.path)
		}
		if got[i].Text != w.text {
			t.Errorf("section %d text = %q, want %q", i, got[i].Text, w.text)
		}
		if got[i].Topic != "traps" {
			t.Errorf("section %d topic = %q", i, got[i].Topic)
		}
	}
	if a := strings.Join(got[1].Aliases, ","); a != "spin,dropped" {
		t.Errorf("aliases = %q", a)
	}
}

func TestStripAliasesRestoresPrintedText(t *testing.T) {
	in := "a\n<!-- aliases: x -->\n| b | c <!-- aliases: y, z --> |\n"
	if got := StripAliases(in); got != "a\n| b | c |\n" {
		t.Fatalf("StripAliases = %q", got)
	}
	for _, topic := range topics {
		b, err := assets.Guide.ReadFile("guide/" + topic + ".md")
		if err != nil {
			t.Fatal(err)
		}
		if s := StripAliases(string(b)); strings.Contains(s, "aliases:") {
			t.Errorf("%s: an alias comment is left in the printed guide", topic)
		}
	}
}

func TestSplitKeepsEveryGuideLine(t *testing.T) {
	// Every non-blank line of each guide lands in some section. Headings above a table
	// are in the rows' paths instead.
	for _, topic := range topics {
		b, _ := assets.Guide.ReadFile("guide/" + topic + ".md")
		var all strings.Builder
		for _, s := range Split(topic, string(b)) {
			all.WriteString(s.Text + "\n")
		}
		text := all.String()
		for i, l := range strings.Split(StripAliases(string(b)), "\n") {
			if lv, _ := heading(l); lv > 0 || strings.TrimSpace(l) == "" {
				continue
			}
			if !strings.Contains(text, l) {
				t.Errorf("%s line %d is in no section: %q", topic, i+1, l)
			}
		}
	}
}

func TestSearchRanksSymptomsToTheirTrap(t *testing.T) {
	ix, err := Load(assets.Guide, "guide", topics)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"keys dropped":          "traps > Keyframes and easing > Keys on 'rotation' (a 3-channel attribute)",
		"rotation does nothing": "traps > Keyframes and easing > Keys on 'rotation' (a 3-channel attribute)",
		"colour keys ignored":   "traps > Keyframes and easing > Colour keys as hex strings",
		"stroke too thick":      "traps > Look and render > Stroke width scales with the layer's scale",
		"blank render":          "traps > Layers and transforms > api.soloLayers([nested]) + api.renderPNGFrame",
		"beat grid":             "music > 1. Get the grid",
	}
	for q, want := range cases {
		hits := ix.Search(q, 3)
		if len(hits) == 0 {
			t.Errorf("%q: no hits", q)
			continue
		}
		if got := hits[0].Topic + " > " + strings.Join(hits[0].Path, " > "); got != want {
			t.Errorf("%q: first hit %q, want %q", q, got, want)
		}
	}
	if hits := ix.Search("zzzz", 3); len(hits) != 0 {
		t.Errorf("nonsense query matched %d sections", len(hits))
	}
}

func TestCapLimitsEachHitAndTheTotal(t *testing.T) {
	long := strings.Repeat("line of guide text\n", 200) // 3800 bytes
	hits := []Hit{
		{Section: Section{Topic: "a", Text: long}},
		{Section: Section{Topic: "b", Text: "short"}},
		{Section: Section{Topic: "c", Text: long}},
		{Section: Section{Topic: "d", Text: long}},
	}
	got, dropped := Cap(hits, 1500, 3000)
	total := 0
	for _, h := range got {
		total += len(h.Text)
	}
	if total > 3000 {
		t.Fatalf("total %d bytes, cap 3000", total)
	}
	if len(got[0].Text) > 1500 || !got[0].Truncated || !strings.HasSuffix(got[0].Text, "`cav guide a`)") {
		t.Errorf("first hit not cut with a pointer: %d bytes, %q", len(got[0].Text), got[0].Text[len(got[0].Text)-40:])
	}
	if got[1].Truncated || got[1].Text != "short" {
		t.Errorf("short hit changed: %+v", got[1])
	}
	if len(got) != 3 || dropped != 1 {
		t.Errorf("kept %d hits and dropped %d, want 3 and 1", len(got), dropped)
	}
	if !strings.HasPrefix(got[0].Text, "line of guide text\n") || strings.Contains(got[0].Text, "line of gu\n") {
		t.Errorf("hit not cut at a line end")
	}
}

func TestCapCutsAtRuneBoundary(t *testing.T) {
	got, _ := Cap([]Hit{{Section: Section{Topic: "t", Text: strings.Repeat("é", 2000)}}}, 1001, 4000)
	if !utf8.ValidString(got[0].Text) {
		t.Fatal("cut inside a rune")
	}
}
