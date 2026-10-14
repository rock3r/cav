package moodboard

import (
	"path/filepath"
	"testing"

	"github.com/rock3r/cav/internal/library"
)

func TestSyncMirrorsAnEmptiedCredit(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	touch(t, filepath.Join(dir, "a.jpg"), "a")
	m := &library.Manifest{Assets: []library.Entry{{Path: "moodboard/a.jpg", Kind: "image", Source: "openverse", Licence: "CC-BY-4.0",
		Attribution: "Photo by X", AttributionRequired: true}}}
	b, _, err := Sync(js, dir, m, root)
	if err != nil || b.Find("a").Credit != "Photo by X" {
		t.Fatalf("%v %+v", err, b.Items)
	}
	b.Save()
	// The manifest now says CC0: no credit, and the item follows.
	m.Assets[0] = library.Entry{Path: "moodboard/a.jpg", Kind: "image", Source: "openverse", Licence: "CC0-1.0"}
	b, _, err = Sync(js, dir, m, root)
	if err != nil {
		t.Fatal(err)
	}
	if it := b.Find("a"); it.Credit != "" || it.Licence != "CC0-1.0" {
		t.Fatalf("stale words kept: %+v", it)
	}
}
