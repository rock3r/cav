package brief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasProblem(b *Brief, want string) bool {
	for _, p := range b.Problems() {
		if strings.Contains(p, want) {
			return true
		}
	}
	return false
}

func TestToneIsAWordOrAList(t *testing.T) {
	one := Parse([]byte(`{"tone":"calm"}`), ".")
	many := Parse([]byte(`{"tone":["calm","warm"]}`), ".")
	if len(one.Problems()) > 0 || len(one.Tone) != 1 || one.Tone[0] != "calm" {
		t.Fatalf("one word: %+v %v", one.Tone, one.Problems())
	}
	if len(many.Problems()) > 0 || len(many.Tone) != 2 {
		t.Fatalf("two words: %+v %v", many.Tone, many.Problems())
	}
}

func TestTypeErrorsNameTheField(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"seconds":"fifteen"}`, "seconds must be a number of seconds, like 15"},
		{`{"bpm":"fast"}`, "bpm must be a number, like 120"},
		{`{"format":"vertical"}`, `format must look like {"width": 1080, "height": 1920, "fps": 60}`},
		{`{"format":{"width":"big"}}`, "format must look like"},
		{`{"tone":5}`, `tone must be a word or a list of words, like ["calm", "warm"]`},
		{`{"avoid":[1]}`, "avoid must be a list of words"},
		{`{"title":3}`, "title must be text"},
	} {
		if b := Parse([]byte(tc.in), "."); !hasProblem(b, tc.want) {
			t.Errorf("%s: want %q, got %v", tc.in, tc.want, b.Problems())
		}
	}
}

func TestSyntaxErrorGivesLineAndColumn(t *testing.T) {
	b := Parse([]byte("{\n  \"title\": \"x\",\n  \"goal\" \"y\"\n}"), ".")
	if !hasProblem(b, "line 3, column") {
		t.Fatalf("%v", b.Problems())
	}
	if b := Parse(nil, "."); !hasProblem(b, "empty") {
		t.Fatalf("%v", b.Problems())
	}
}

func TestWrongLetterCaseGetsAHint(t *testing.T) {
	b := Parse([]byte(`{"musthave":["logo"],"BPM":120}`), ".")
	if !hasProblem(b, `did you mean "mustHave"?`) || !hasProblem(b, `did you mean "bpm"?`) {
		t.Fatalf("%v", b.Problems())
	}
}

func TestNewerSchemaAsksForAnUpdate(t *testing.T) {
	b := Parse([]byte(`{"schema":"cav-brief/2"}`), ".")
	if !hasProblem(b, "brief.json is cav-brief/2; this cav reads cav-brief/1. Update cav (cav update).") {
		t.Fatalf("%v", b.Problems())
	}
	if b := Parse([]byte(`{"schema":"other/1"}`), "."); !hasProblem(b, `schema must be "cav-brief/1"`) {
		t.Fatalf("%v", b.Problems())
	}
	if b := Parse([]byte(`{"schema":"cav-brief/1"}`), "."); len(b.Problems()) > 0 {
		t.Fatalf("%v", b.Problems())
	}
}

func TestRanges(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"seconds":-1}`, "seconds must be more than 0"},
		{`{"seconds":4000}`, "seconds must be more than 0"},
		{`{"bpm":10}`, "bpm must be between 30 and 300"},
		{`{"bpm":900}`, "bpm must be between 30 and 300"},
		{`{"format":{"width":1080}}`, "format needs both width and height"},
		{`{"format":{"width":4,"height":4}}`, "format width must be between 16 and 16384"},
		{`{"format":{"width":99999,"height":1080}}`, "format width must be between 16 and 16384"},
		{`{"format":{"width":1080,"height":1920,"fps":500}}`, "format fps must be more than 0"},
		{`{"mustHave":["a",""]}`, "mustHave item 2 is empty"},
		{`{"refs":["ftp://x.test/a"]}`, "not a web link"},
	} {
		if b := Parse([]byte(tc.in), "."); !hasProblem(b, tc.want) {
			t.Errorf("%s: want %q, got %v", tc.in, tc.want, b.Problems())
		}
	}
}

func TestMissingIsNotAProblem(t *testing.T) {
	b := Parse([]byte(`{"title":"x","tone":[""],"format":{"fps":60}}`), ".")
	// An empty tone word is a problem; take it out to look at the gaps alone.
	if !hasProblem(b, "tone item 1 is empty") {
		t.Fatalf("%v", b.Problems())
	}
	b = Parse([]byte(`{"title":"x","format":{"fps":60}}`), ".")
	if len(b.Problems()) > 0 {
		t.Fatalf("a gap is not a problem: %v", b.Problems())
	}
	if got := strings.Join(b.Missing(), ","); got != "goal,audience,tone,seconds,format" {
		t.Fatalf("missing %s", got)
	}
	full := Parse([]byte(`{"title":"t","goal":"g","audience":"a","tone":"calm","seconds":15,"format":{"width":1080,"height":1920}}`), ".")
	if len(full.Missing()) != 0 || len(full.Problems()) != 0 {
		t.Fatalf("%v %v", full.Missing(), full.Problems())
	}
}

func TestUnknownFieldIsKeptWithANote(t *testing.T) {
	b := Parse([]byte(`{"title":"t","zeta":{"a":1}}`), ".")
	if len(b.Problems()) > 0 || string(b.Extra["zeta"]) != `{"a":1}` {
		t.Fatalf("%v %v", b.Problems(), b.Extra)
	}
	if len(b.Notes()) != 1 || !strings.Contains(b.Notes()[0], `cav does not use "zeta"; it is kept and shown in review`) {
		t.Fatalf("%v", b.Notes())
	}
}

func TestOddSizeIsOnlyANote(t *testing.T) {
	b := Parse([]byte(`{"format":{"width":1081,"height":1920}}`), ".")
	if len(b.Problems()) > 0 || len(b.Notes()) != 1 || !strings.Contains(b.Notes()[0], "even sizes") {
		t.Fatalf("%v %v", b.Problems(), b.Notes())
	}
}

func TestRefsAreLinksFilesOrMissing(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.jpg"), "x")
	b := Parse([]byte(`{"refs":["https://vimeo.com/123","a.jpg","gone.png"]}`), dir)
	if len(b.Problems()) > 0 {
		t.Fatalf("%v", b.Problems())
	}
	refs := b.RefList()
	if len(refs) != 3 || !refs[0].Link || refs[0].Path != "" {
		t.Fatalf("link: %+v", refs)
	}
	if refs[1].Link || !refs[1].Exists || refs[1].Path != filepath.Join(dir, "a.jpg") {
		t.Fatalf("file: %+v", refs[1])
	}
	if refs[2].Exists {
		t.Fatalf("missing: %+v", refs[2])
	}
	if len(b.Notes()) != 1 || !strings.Contains(b.Notes()[0], `"gone.png" was not found`) {
		t.Fatalf("%v", b.Notes())
	}
}

func TestSaveLoadRoundTripKeepsTheOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brief.json")
	in := &Brief{Title: "Launch", Goal: "g", Audience: "a", Tone: List{"confident", "playful"}, Seconds: 15, BPM: 120,
		Format: &Format{Width: 1080, Height: 1920, FPS: 30}, Refs: List{"https://vimeo.com/123456789"}}
	if err := in.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	last := -1
	for _, k := range append([]string{"schema"}, Order...) {
		i := strings.Index(string(data), `"`+k+`"`)
		if i < 0 || i < last {
			t.Fatalf("%s is missing or out of order:\n%s", k, data)
		}
		last = i
	}
	if strings.Contains(string(data), "null") || !strings.HasSuffix(string(data), "}\n") {
		t.Fatalf("lists are never null, and the file ends with a newline:\n%s", data)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Launch" || out.Seconds != 15 || out.BPM != 120 || out.Format.FPS != 30 || len(out.Tone) != 2 || out.Schema != Schema {
		t.Fatalf("%+v", out)
	}
	// An empty brief writes the template: strings empty, lists empty, no numbers.
	empty := filepath.Join(t.TempDir(), "brief.json")
	if err := (&Brief{}).Save(empty); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(empty)
	for _, want := range []string{`"tone": []`, `"title": ""`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("template lacks %s:\n%s", want, data)
		}
	}
	for _, not := range []string{`"seconds"`, `"bpm"`, `"format"`} {
		if strings.Contains(string(data), not) {
			t.Errorf("template has %s:\n%s", not, data)
		}
	}
}

func TestLoadReturnsAnErrorWithProblems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brief.json")
	write(t, path, `{"seconds":"fifteen","bpm":1}`)
	_, err := Load(path)
	if err == nil || !strings.HasPrefix(err.Error(), "brief.json has problems:\n  - seconds must be") {
		t.Fatalf("%v", err)
	}
	if e, ok := err.(*Error); !ok || len(e.Problems) != 2 {
		t.Fatalf("%v", err)
	}
}

func TestFieldsOrderAndText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brief.json")
	write(t, path, `{"schema":"cav-brief/1","zeta":[1,"b"],"avoid":["red","stock music"],"format":{"width":1080,"height":1920,"fps":60},
		"seconds":15,"bpm":120,"tone":"calm","mustHave":["a logo"],"alpha":"x"}`)
	fields, err := Fields(path)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	text := map[string]string{}
	for _, f := range fields {
		keys = append(keys, f.Key)
		text[f.Key] = f.Text
	}
	if got := strings.Join(keys, ","); got != "tone,seconds,bpm,format,mustHave,avoid,alpha,zeta" {
		t.Fatalf("order %s", got)
	}
	for k, want := range map[string]string{"tone": "calm", "seconds": "15 s", "bpm": "120 bpm", "format": "1080 x 1920 at 60 fps",
		"avoid": "red; stock music", "zeta": `1; b`} {
		if text[k] != want {
			t.Errorf("%s text %q, want %q", k, text[k], want)
		}
	}
	if fields[0].Title != "Tone" || fields[4].Title != "Must have" || fields[6].Title != "alpha" {
		t.Errorf("titles %+v", fields)
	}
	var raw string
	if json.Unmarshal(fields[0].Raw, &raw) != nil || raw != "calm" {
		t.Errorf("Raw is the value as written: %s", fields[0].Raw)
	}
	write(t, path, `{"seconds":"x"}`)
	if _, err := Fields(path); err == nil {
		t.Error("a problem should fail")
	}
}
