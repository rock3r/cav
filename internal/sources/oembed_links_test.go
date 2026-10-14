package sources

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/rock3r/cav/internal/moodboard"
)

func TestSaveThumbNeverWritesThroughLinks(t *testing.T) {
	stubVideoServices(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(png1x1)) })
	ctx := context.Background()
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	unchanged := func(what string) {
		t.Helper()
		if b, _ := os.ReadFile(victim); string(b) != "keep me" {
			t.Errorf("%s: the victim file changed", what)
		}
	}

	// A thumbnail folder that is a link to another folder.
	outside := t.TempDir()
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, moodboard.ThumbDir)); err != nil {
		t.Skip("no symlinks")
	}
	if err := SaveThumb(ctx, "https://i.ytimg.com/a.png", filepath.Join(root, moodboard.ThumbDir, "t.jpg")); err == nil {
		t.Error("a linked thumbnail folder must be refused")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("something was written through the link: %v", entries)
	}

	// A thumbnail file that is a link to the victim.
	root2 := t.TempDir()
	dir := filepath.Join(root2, moodboard.ThumbDir)
	os.MkdirAll(dir, 0o755)
	if err := os.Symlink(victim, filepath.Join(dir, "t.jpg")); err != nil {
		t.Skip("no symlinks")
	}
	if err := SaveThumb(ctx, "https://i.ytimg.com/a.png", filepath.Join(dir, "t.jpg")); err == nil {
		t.Error("a linked thumbnail file must be refused")
	}
	unchanged("linked file")

	// An ordinary file is replaced by rename, and no temporary file stays.
	if err := os.WriteFile(filepath.Join(dir, "plain.jpg"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveThumb(ctx, "https://i.ytimg.com/a.png", filepath.Join(dir, "plain.jpg")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "plain.jpg")); string(b) != png1x1 {
		t.Error("not replaced")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 { // t.jpg (the link) and plain.jpg
		t.Errorf("leftovers: %v", entries)
	}
	// Outside a thumbnail folder there is no write at all.
	if err := SaveThumb(ctx, "https://i.ytimg.com/a.png", filepath.Join(root2, "loose.jpg")); err == nil {
		t.Error("a file outside .thumbs must be refused")
	}
}
