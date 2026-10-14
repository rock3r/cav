package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/brief"
)

func TestApplyBrief(t *testing.T) {
	b := &brief.Brief{Title: "T", Seconds: 15, BPM: 120, Format: &brief.Format{Width: 1080, Height: 1920, FPS: 30}}
	blocked := func(key, flag string) func(string) string {
		return func(k string) string {
			if k == key {
				return flag
			}
			return ""
		}
	}
	for _, tc := range []struct {
		name    string
		start   compParams
		given   func(string) string
		withBPM bool
		want    compParams
		over    string
	}{
		{"takes everything", compParams{}, blocked("", ""), true, compParams{W: 1080, H: 1920, FPS: 30, Secs: 15, BPM: 120}, ""},
		{"a scene has no tempo", compParams{BPM: 7}, blocked("", ""), false, compParams{W: 1080, H: 1920, FPS: 30, Secs: 15, BPM: 7}, ""},
		{"--frames or --range block seconds", compParams{Secs: 4}, blocked("seconds", "--frames"), true,
			compParams{W: 1080, H: 1920, FPS: 30, Secs: 4, BPM: 120}, "--frames"},
		{"--fps-preset blocks fps", compParams{FPS: 24}, blocked("fps", "--fps-preset"), true,
			compParams{W: 1080, H: 1920, FPS: 24, Secs: 15, BPM: 120}, "--fps-preset"},
		{"one size flag blocks both sizes", compParams{W: 800, H: 1080}, blocked("size", "--width"), true,
			compParams{W: 800, H: 1080, FPS: 30, Secs: 15, BPM: 120}, "--width"},
	} {
		p := tc.start
		u := applyBrief(&p, b, "brief.json", tc.given, tc.withBPM)
		if p != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, p, tc.want)
		}
		if got := strings.Join(u.Overridden, ","); got != tc.over {
			t.Errorf("%s: overridden %q, want %q", tc.name, got, tc.over)
		}
	}
}

func TestApplyBriefWithoutABriefChangesNothing(t *testing.T) {
	p := compParams{W: 1, H: 2, FPS: 3, Secs: 4, BPM: 5}
	if u := applyBrief(&p, nil, "brief.json", func(string) string { return "" }, true); u != nil || p != (compParams{W: 1, H: 2, FPS: 3, Secs: 4, BPM: 5}) {
		t.Fatalf("%v %+v", u, p)
	}
	// A brief with only a size gives only a size.
	p = compParams{}
	u := applyBrief(&p, &brief.Brief{Format: &brief.Format{Width: 640, Height: 480}}, "brief.json", func(string) string { return "" }, true)
	if p != (compParams{W: 640, H: 480}) || u.line() != "from brief.json: 640 x 480" {
		t.Fatalf("%+v %s", p, u.line())
	}
}

func TestBriefForFindsOnlyAnExistingFile(t *testing.T) {
	inNewProject(t)
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	path, off := briefFlags(fs)
	if b, err := briefFor(fs, *path, *off); b != nil || err != nil {
		t.Fatalf("no brief.json is fine: %v %v", b, err)
	}
	if err := fs.Parse([]string{"--brief", "nope.json"}); err != nil {
		t.Fatal(err)
	}
	if _, err := briefFor(fs, *path, *off); err == nil {
		t.Fatal("a named brief must exist")
	}
}
