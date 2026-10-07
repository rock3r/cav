package library

import (
	"os"
	"path/filepath"
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
	cc0 := Entry{Path: filepath.Join(root, "sfx/b.wav")}
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
}
