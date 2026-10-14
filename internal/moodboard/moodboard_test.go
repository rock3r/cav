package moodboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/library"
)

func touch(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ids(b *Board) string { return strings.Join(b.IDs(), ",") }

func TestSyncKeepsIDsAndTakesAndTracksFiles(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	touch(t, filepath.Join(dir, "Neon City.jpg"), "a")
	touch(t, filepath.Join(dir, "neon-city.png"), "b")
	touch(t, filepath.Join(dir, "reel.mp4"), "c")
	touch(t, filepath.Join(dir, "notes.txt"), "d")
	touch(t, filepath.Join(dir, ".hidden.png"), "e")

	b, ch, err := Sync(js, dir, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(b); got != "neon-city,neon-city-2,reel" || len(ch.Added) != 3 {
		t.Fatalf("ids %s, changes %+v", got, ch)
	}
	if b.Items[2].Kind != KindClip || b.Items[2].Thumb != "moodboard/.thumbs/reel.png" || b.Items[0].AddedBy != ByAgent {
		t.Fatalf("clip item %+v", b.Items[2])
	}
	b.Find("neon-city").Take = "the glow"
	b.Find("neon-city-2").Status = StatusDropped
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	// A file goes, another is added: the take and the ids stay, the gone file is marked.
	os.Remove(filepath.Join(dir, "Neon City.jpg"))
	os.Remove(filepath.Join(dir, "neon-city.png")) // the dropped one: its file leaving changes nothing
	touch(t, filepath.Join(dir, "fresh.png"), "f")
	b, ch, err = Sync(js, dir, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	if b.Find("neon-city").Status != StatusMissing || b.Find("neon-city").Take != "the glow" {
		t.Fatalf("a gone file is marked and keeps its take: %+v", b.Find("neon-city"))
	}
	if b.Find("neon-city-2").Status != StatusDropped {
		t.Fatalf("dropped stays dropped: %+v", b.Find("neon-city-2"))
	}
	if len(ch.Missing) != 1 || ch.Missing[0] != "neon-city" || len(ch.Added) != 1 || ch.Added[0] != "fresh" {
		t.Fatalf("changes %+v", ch)
	}
	b.Save()

	// The file comes back: the item is active again with its take. A dropped file that
	// returns stays dropped.
	touch(t, filepath.Join(dir, "Neon City.jpg"), "a")
	touch(t, filepath.Join(dir, "neon-city.png"), "b")
	b, ch, _ = Sync(js, dir, nil, root)
	if it := b.Find("neon-city"); it.Status != "" || it.Take != "the glow" || len(ch.Revived) != 1 {
		t.Fatalf("revived %+v %+v", it, ch)
	}
	if b.Find("neon-city-2").Status != StatusDropped {
		t.Fatal("a dropped item must never come back")
	}
	if len(b.Active()) != 3 {
		t.Fatalf("active %d", len(b.Active()))
	}
}

func TestSyncCopiesLicenceFromTheManifestAndAddsEmbeds(t *testing.T) {
	root := t.TempDir()
	js := filepath.Join(root, "moodboard.json")
	dir := filepath.Join(root, "moodboard")
	touch(t, filepath.Join(dir, "a.jpg"), "a")
	touch(t, filepath.Join(dir, ThumbDir, "yt-aqz-KE-bpKQ.jpg"), "thumb")
	m := &library.Manifest{Assets: []library.Entry{
		{Path: "moodboard/a.jpg", Kind: "image", Source: "openverse", Licence: "CC-BY-4.0", Attribution: "Photo by X", Title: "A", URL: "https://example.test/a"},
		{Path: "moodboard/.thumbs/yt-aqz-KE-bpKQ.jpg", Kind: "embed", Source: "youtube", Licence: "reference-only", Title: "Reel", Attribution: "by Studio",
			URL: "https://youtu.be/aqz-KE-bpKQ"},
		{Path: "moodboard/.thumbs/evil.jpg", Kind: "embed", URL: "https://evil.test/watch?v=aqz-KE-bpKQ"},
	}}
	b, _, err := Sync(js, dir, m, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Items) != 2 {
		t.Fatalf("items %+v", b.Items)
	}
	a := b.Find("a")
	if a.Source != "openverse" || a.Credit != "Photo by X" || a.URL != "https://example.test/a" || a.Licence != "CC-BY-4.0" {
		t.Fatalf("manifest data %+v", a)
	}
	e := b.Find("yt-aqz-KE-bpKQ")
	if e == nil || e.Kind != KindEmbed || e.URL != "https://www.youtube.com/watch?v=aqz-KE-bpKQ" || e.Thumb != "moodboard/.thumbs/yt-aqz-KE-bpKQ.jpg" || e.Licence != "reference-only" {
		t.Fatalf("embed %+v", e)
	}
	// A second sync neither duplicates the embed nor marks it missing.
	b.Save()
	b, ch, _ := Sync(js, dir, m, root)
	if len(b.Items) != 2 || len(ch.Added) != 0 || len(ch.Missing) != 0 {
		t.Fatalf("second sync %+v %+v", b.Items, ch)
	}
}

func TestSyncRefusesAFolderOutsideTheFile(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Sync(filepath.Join(root, "a", "moodboard.json"), filepath.Join(root, "b"), nil, root); err == nil {
		t.Fatal("a folder outside the file's folder cannot be shown by the review page")
	}
}

func TestParseVideoURL(t *testing.T) {
	good := []struct{ in, provider, id, hash string }{
		{"https://www.youtube.com/watch?v=aqz-KE-bpKQ", YouTube, "aqz-KE-bpKQ", ""},
		{"https://youtube.com/watch?v=aqz-KE-bpKQ&t=3", YouTube, "aqz-KE-bpKQ", ""},
		{"https://m.youtube.com/watch?v=aqz-KE-bpKQ", YouTube, "aqz-KE-bpKQ", ""},
		{"https://youtu.be/aqz-KE-bpKQ?si=x", YouTube, "aqz-KE-bpKQ", ""},
		{"https://www.youtube.com/shorts/aqz-KE-bpKQ", YouTube, "aqz-KE-bpKQ", ""},
		{"https://www.youtube.com/embed/aqz-KE-bpKQ", YouTube, "aqz-KE-bpKQ", ""},
		{"https://vimeo.com/123456789", Vimeo, "123456789", ""},
		{"https://vimeo.com/123456789/abcdef1234", Vimeo, "123456789", "abcdef1234"},
		{"https://player.vimeo.com/video/123456789?h=abcdef1234", Vimeo, "123456789", "abcdef1234"},
		{"http://www.vimeo.com/42", Vimeo, "42", ""},
	}
	for _, c := range good {
		p, id, h, ok := ParseVideoURL(c.in)
		if !ok || p != c.provider || id != c.id || h != c.hash {
			t.Errorf("%s -> %s %s %s %v", c.in, p, id, h, ok)
		}
	}
	bad := []string{
		"", "javascript:alert(1)", "data:text/html,x", "ftp://youtube.com/watch?v=aqz-KE-bpKQ",
		"https://evil.test/watch?v=aqz-KE-bpKQ",
		"https://youtube.com.evil.test/watch?v=aqz-KE-bpKQ",
		"https://evil.test/https://www.youtube.com/watch?v=aqz-KE-bpKQ",
		"https://user@www.youtube.com/watch?v=aqz-KE-bpKQ",
		"https://www.youtube.com:8443/watch?v=aqz-KE-bpKQ",
		"https://www.youtube.com/watch?v=short",
		"https://www.youtube.com/watch?v=aqz-KE-bpK%22",
		"https://www.youtube.com/watch?v=\"><script>x",
		"https://www.youtube.com/watch",
		"https://www.youtube.com/results?v=aqz-KE-bpKQ",
		"https://youtu.be/aqz-KE-bpKQ/extra",
		"https://vimeo.com/abc", "https://vimeo.com/123456789/not a hash", "https://vimeo.com/123/a/b",
		"https://vimeo.com/channels/staffpicks/123",
	}
	for _, in := range bad {
		if p, id, _, ok := ParseVideoURL(in); ok {
			t.Errorf("%q was accepted: %s %s", in, p, id)
		}
	}
}

func TestPlayerURLUsesOnlyCheckedParts(t *testing.T) {
	if got := PlayerURL(YouTube, "aqz-KE-bpKQ", ""); got != "https://www.youtube-nocookie.com/embed/aqz-KE-bpKQ?rel=0&playsinline=1&autoplay=1" {
		t.Error(got)
	}
	if got := PlayerURL(Vimeo, "123", "abcdef12"); got != "https://player.vimeo.com/video/123?dnt=1&autoplay=1&h=abcdef12" {
		t.Error(got)
	}
	for _, c := range [][3]string{
		{YouTube, "x\"onload=1", ""}, {Vimeo, "12/../3", ""}, {Vimeo, "123", "a&b=c"}, {"other", "123", ""}, {YouTube, "aqz-KE-bpKQ", "hash"},
	} {
		if got := PlayerURL(c[0], c[1], c[2]); got != "" {
			t.Errorf("%v gave %s", c, got)
		}
	}
	if PlayerFor("https://evil.test/embed/aqz-KE-bpKQ") != "" {
		t.Error("another host got a player")
	}
}

func TestContactSheet(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "c.mp4")
	if b, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=10", "-t", "2", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot make a test clip: %v %s", err, b)
	}
	out := filepath.Join(dir, ThumbDir, "c.png")
	secs, err := ContactSheet(context.Background(), clip, out, 6)
	if err != nil || secs < 1.9 || secs > 2.1 {
		t.Fatalf("%v %v", secs, err)
	}
	st1, err := os.Stat(out)
	if err != nil || st1.Size() == 0 {
		t.Fatal("no sheet")
	}
}
