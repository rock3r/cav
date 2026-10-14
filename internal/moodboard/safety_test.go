package moodboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncMatchesFilesWrittenInOtherForms(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	touch(t, filepath.Join(dir, "a.jpg"), "a")
	touch(t, filepath.Join(dir, "b.jpg"), "b")
	touch(t, js, `{"schema":"cav-moodboard/1","dir":"moodboard","items":[
		{"id":"first","file":"./moodboard/a.jpg","take":"kept"},
		{"id":"second","file":`+jsonStr(filepath.Join(dir, "b.jpg"))+`,"take":"also kept"}]}`)
	b, ch, err := Sync(js, dir, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Items) != 2 || len(ch.Added) != 0 || len(ch.Missing) != 0 {
		t.Fatalf("items %+v changes %+v", b.Items, ch)
	}
	if b.Find("first").File != "moodboard/a.jpg" || b.Find("second").File != "moodboard/b.jpg" || b.Find("second").Take != "also kept" {
		t.Fatalf("not canonical: %+v", b.Items)
	}
}

func TestClipThumbIsAlwaysCavsOwnPath(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	victim := filepath.Join(t.TempDir(), "victim")
	touch(t, victim, "keep me")
	touch(t, filepath.Join(dir, "reel.mp4"), "v")
	touch(t, filepath.Join(dir, "x.mp4"), "v")
	touch(t, js, `{"schema":"cav-moodboard/1","dir":"moodboard","items":[
		{"id":"reel","kind":"clip","file":"moodboard/reel.mp4","thumb":`+jsonStr(victim)+`},
		{"id":"../../x","kind":"clip","file":"moodboard/x.mp4","thumb":"../../x.png"}]}`)
	b, _, err := Sync(js, dir, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range b.Items {
		out, err := b.SheetPath(it)
		if err != nil {
			t.Fatalf("%s: %v", it.ID, err)
		}
		if !strings.HasPrefix(out, filepath.Join(dir, ThumbDir)+string(filepath.Separator)) || filepath.Dir(out) != filepath.Join(dir, ThumbDir) {
			t.Errorf("%s: sheet path %s", it.ID, out)
		}
		if strings.Contains(it.Thumb, "..") || filepath.IsAbs(it.Thumb) || it.Thumb != b.ClipThumb(it.ID) {
			t.Errorf("%s: thumb %q", it.ID, it.Thumb)
		}
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep me" {
		t.Error("victim changed")
	}
}

func TestSheetPathRefusesLinks(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	touch(t, filepath.Join(dir, "reel.mp4"), "v")
	b, _, err := Sync(js, dir, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, ThumbDir)); err != nil {
		t.Skip("no symlinks")
	}
	if _, err := b.SheetPath(*b.Find("reel")); err == nil {
		t.Error("a linked thumbnail folder must be refused")
	}
	os.Remove(filepath.Join(dir, ThumbDir))
	os.MkdirAll(filepath.Join(dir, ThumbDir), 0o755)
	target := filepath.Join(outside, "t.png")
	touch(t, target, "keep")
	if err := os.Symlink(target, filepath.Join(dir, ThumbDir, "reel.png")); err != nil {
		t.Skip("no symlinks")
	}
	if _, err := b.SheetPath(*b.Find("reel")); err == nil {
		t.Error("a linked sheet must be refused")
	}
}

func TestEmbedThumbOutsideTheThumbFolderIsDropped(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	touch(t, filepath.Join(dir, "a.jpg"), "a")
	touch(t, js, `{"schema":"cav-moodboard/1","dir":"moodboard","items":[
		{"id":"yt-aqz-KE-bpKQ","kind":"embed","url":"https://youtu.be/aqz-KE-bpKQ","thumb":"/etc/hosts"}]}`)
	b, _, err := Sync(js, dir, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Find("yt-aqz-KE-bpKQ").Thumb; got != "" {
		t.Errorf("thumb %q", got)
	}
}

// jsonStr quotes a path for a JSON fixture: a Windows path has backslashes.
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
