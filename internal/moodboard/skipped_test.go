package moodboard

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncSkipsImagesThePageCannotShow(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	for _, n := range []string{"a.png", "b.JPG", "c.webp", "d.gif", "e.jpeg", "f.tiff", "g.bmp", "h.tif"} {
		touch(t, filepath.Join(dir, n), "x")
	}
	b, ch, err := Sync(js, dir, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(b); got != "a,b,c,d,e" {
		t.Errorf("items %s", got)
	}
	if got := strings.Join(ch.Skipped, ","); got != "moodboard/f.tiff,moodboard/g.bmp,moodboard/h.tif" {
		t.Errorf("skipped %s", got)
	}
}
