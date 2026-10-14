package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplacedClipThatCannotBeReadLosesItsStaleSheet(t *testing.T) {
	inNewProject(t)
	os.MkdirAll("moodboard/.thumbs", 0o755)
	os.WriteFile("moodboard/reel.mp4", []byte("this is not a video"), 0o644)
	sheet := filepath.Join("moodboard", ".thumbs", "reel.png")
	os.WriteFile(sheet, []byte("old sheet"), 0o644)
	os.WriteFile("moodboard.json", []byte(`{"schema":"cav-moodboard/1","dir":"moodboard","items":[
		{"id":"reel","kind":"clip","file":"moodboard/reel.mp4","thumb":"moodboard/.thumbs/reel.png","seconds":12.4,"take":"kept"}]}`), 0o644)

	b, _, notes, err := moodSync(context.Background(), "moodboard", "moodboard.json")
	if err != nil {
		t.Fatal(err)
	}
	it := b.Find("reel")
	if it.Seconds != 0 || it.Take != "kept" {
		t.Errorf("item %+v", it)
	}
	if _, err := os.Stat(sheet); err == nil {
		t.Error("the stale sheet is still there")
	}
	if len(notes) != 1 || !strings.HasPrefix(notes[0], "reel: no contact sheet") {
		t.Errorf("notes %v", notes)
	}
	if saved := readMood(t, "moodboard.json").Find("reel"); saved.Seconds != 0 {
		t.Errorf("saved %+v", saved)
	}
}
