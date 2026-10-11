package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFollowsTheProjectLicence(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"sfx/a.wav", "sfx/b.wav", "img/c.jpg"} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(f)), 0o755)
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}
	m := &Manifest{}
	nc := Entry{Path: filepath.Join(root, "sfx/a.wav"), Title: "Swish", Author: "someone", Source: "freesound"}
	FromCC("by-nc", "4.0").Apply(&nc)
	nc.Attribution = DefaultAttribution(nc)
	cc0 := Entry{Path: filepath.Join(root, "sfx/b.wav"), Attribution: "suggested by the source, not required"}
	FromCC("cc0", "").Apply(&cc0)
	sa := Entry{Path: filepath.Join(root, "img/c.jpg"), Title: "Neon"}
	FromName("CC BY-SA 3.0").Apply(&sa)
	gone := Entry{Path: filepath.Join(root, "sfx/gone.wav")}
	FromCC("cc0", "").Apply(&gone)
	for _, e := range []Entry{nc, cc0, sa, gone} {
		m.Put(root, e)
	}
	if m.Assets[0].Path != "sfx/a.wav" {
		t.Fatalf("paths must be relative to the root: %s", m.Assets[0].Path)
	}
	kinds := func(p *Project) map[string]int {
		k := map[string]int{}
		for _, pr := range Check(root, m, p) {
			k[pr.Kind]++
		}
		return k
	}
	got := kinds(&Project{Licence: "commercial"})
	if got["nc-in-commercial"] != 1 || got["share-alike"] != 1 || got["missing-credit"] != 1 || got["missing-file"] != 1 {
		t.Fatalf("commercial: %v", got)
	}
	got = kinds(&Project{Licence: "nc-ok"})
	if got["nc-in-commercial"] != 0 || got["share-alike"] != 1 {
		t.Fatalf("nc-ok: %v", got)
	}
	lines := Credits(m, false)
	if len(lines) != 2 || lines[0] != "“Swish” by someone (freesound), CC BY-NC 4.0" {
		t.Fatalf("credits: %q", lines)
	}
	if all := Credits(m, true); len(all) != 4 {
		t.Fatalf("--all lists every asset once: %q", all)
	}
}

func TestYuE2TakesFollowTheProjectLicence(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"music/y.wav", "music/e.mp3"} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(f)), 0o755)
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}
	m := &Manifest{}
	yue := Entry{Path: filepath.Join(root, "music/y.wav"), Generated: &Generated{Provider: "yue2", Model: "YuE2-3B"}}
	GeneratedTerms("yue2").Apply(&yue)
	other := Entry{Path: filepath.Join(root, "music/e.mp3"), Generated: &Generated{Provider: "elevenlabs"}}
	GeneratedTerms("elevenlabs").Apply(&other)
	m.Put(root, yue)
	m.Put(root, other)
	if yue.CommercialOK || yue.Licence != "YuE2-model-licence" || yue.AttributionRequired || other.Licence != "generated" || !other.CommercialOK {
		t.Fatalf("terms: %+v / %+v", yue, other)
	}
	for _, c := range []struct {
		licence string
		want    int
	}{{"commercial", 1}, {"", 1}, {"nc-ok", 0}} {
		probs := Check(root, m, &Project{Licence: c.licence})
		if len(probs) != c.want {
			t.Fatalf("%q: %+v", c.licence, probs)
		}
		if c.want == 1 && (probs[0].Path != "music/y.wav" || probs[0].Kind != "nc-in-commercial" || !strings.Contains(probs[0].Detail, "individual creator")) {
			t.Fatalf("%q: %+v", c.licence, probs[0])
		}
	}
}

func TestCoverRecordsTheSongAndIsCredited(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "music"), 0o755)
	os.WriteFile(filepath.Join(root, "music/c.wav"), []byte("x"), 0o644)
	m := &Manifest{}
	e := Entry{Path: filepath.Join(root, "music/c.wav"), Generated: &Generated{Provider: "yue2", Model: "YuE2-3B", CoverOf: "refs/theme.mp3"}}
	CoverTerms("yue2", "refs/theme.mp3").Apply(&e)
	e.Attribution = DefaultAttribution(e)
	m.Put(root, e)
	if e.CommercialOK || !e.AttributionRequired || e.Licence != "YuE2-model-licence" ||
		!strings.Contains(e.LicenceNote, "individual creator") || !strings.Contains(e.LicenceNote, "a cover of refs/theme.mp3") ||
		!strings.Contains(e.LicenceNote, "SheetSage2, whose weights are CC BY-NC 4.0") {
		t.Fatalf("terms: %+v", e)
	}
	want := "Cover of “theme” (melody from refs/theme.mp3). Generated with yue2 (YuE2-3B)"
	if lines := Credits(m, false); len(lines) != 1 || lines[0] != want {
		t.Fatalf("a cover is credited without --all: %q", lines)
	}
	probs := Check(root, m, &Project{Licence: "commercial"})
	if len(probs) != 1 || probs[0].Kind != "nc-in-commercial" || !strings.Contains(probs[0].Detail, "cover licence") {
		t.Fatalf("check: %+v", probs)
	}
	if probs := Check(root, m, &Project{Licence: "nc-ok"}); len(probs) != 0 {
		t.Fatalf("nc-ok, with the credit recorded: %+v", probs)
	}
}
