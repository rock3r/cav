package moodboard

import (
	"path/filepath"
	"testing"

	"github.com/rock3r/cav/internal/library"
)

func TestEmbedDoesNotTakeOverAFileWithTheSameID(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	touch(t, filepath.Join(dir, "yt-abcdefghijk.jpg"), "image")
	touch(t, filepath.Join(dir, ThumbDir, "yt-abcdefghijk.jpg"), "thumb")
	m := &library.Manifest{Assets: []library.Entry{
		{Path: "moodboard/yt-abcdefghijk.jpg", Kind: "image", Source: "openverse", Licence: "CC0-1.0", Title: "My photo"},
		{Path: "moodboard/.thumbs/yt-abcdefghijk.jpg", Kind: "embed", Source: "youtube", Licence: "reference-only", Title: "Reel", URL: "https://youtu.be/abcdefghijk"},
	}}
	for round := 0; round < 2; round++ { // the second sync finds the same two items again
		b, _, err := Sync(js, dir, m, root)
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Items) != 2 {
			t.Fatalf("round %d: items %+v", round, b.Items)
		}
		img := b.Find("yt-abcdefghijk")
		emb := b.Find("yt-abcdefghijk-2")
		if img == nil || img.Kind != KindImage || img.Source != "openverse" || img.Title != "My photo" || img.URL != "" || img.Thumb != "" {
			t.Fatalf("image item %+v", img)
		}
		if emb == nil || emb.Kind != KindEmbed || emb.Licence != "reference-only" || emb.URL != "https://www.youtube.com/watch?v=abcdefghijk" {
			t.Fatalf("embed item %+v", emb)
		}
		if err := b.Save(); err != nil {
			t.Fatal(err)
		}
	}
}
