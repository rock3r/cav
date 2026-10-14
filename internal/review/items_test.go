package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/board"
)

// writeBoard makes a storyboard of three shots with a frame each, in a new folder.
func writeBoard(t *testing.T) (path string, dir string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "board"), 0o755); err != nil {
		t.Fatal(err)
	}
	sb := &board.Board{BPM: 120, Shots: []board.Shot{
		{ID: "s1", Beats: [2]float64{0, 4}, What: "logo appears", Frame: "board/s1.png"},
		{ID: "s2", Beats: [2]float64{4, 8}, What: "tagline slides in", Camera: "push in", Frame: "board/s2.png"},
		{ID: "s3", Beats: [2]float64{8, 12}, What: "end card"},
	}}
	path = filepath.Join(dir, "storyboard.json")
	if err := sb.Save(path); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s1", "s2"} {
		writeFile(t, filepath.Join(dir, "board", id+".png"), "frame "+id)
	}
	return path, dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func shaOf(items []Item, id string) string {
	for _, it := range items {
		if it.ID == id {
			return it.SHA
		}
	}
	return ""
}

func TestKindOf(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, body string
		want       Kind
		fail       bool
	}{
		{"render.mp4", "", KindVideo, false},
		{"storyboard.json", `{"schema":"cav-board/1"}`, KindStoryboard, false},
		{"anything.json", `{"schema":"cav-board/1"}`, KindStoryboard, false},
		{"moodboard.json", `{"schema":"cav-moodboard/1"}`, KindMoodboard, false},
		{"brief.json", `{"schema":"cav-brief/1"}`, KindBrief, false},
		{"storyboard.json", `{}`, KindStoryboard, false}, // no schema: the file name decides
		{"moodboard.json", `{}`, KindMoodboard, false},
		{"brief.json", `{}`, KindBrief, false},
		{"notes.json", `{}`, "", true},
		{"storyboard.json", `{"schema":"other/1"}`, "", true},
		{"storyboard.json", `not json`, "", true},
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.name)
		if strings.HasSuffix(c.name, ".json") {
			writeFile(t, p, c.body)
		}
		got, err := KindOf(p)
		if (err != nil) != c.fail || got != c.want {
			t.Errorf("KindOf(%s %s) = %q, %v; want %q (fail %v)", c.name, c.body, got, err, c.want, c.fail)
		}
	}
	if _, err := KindOf(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("a missing .json file has no kind")
	}
}

func TestStoryboardItemHashes(t *testing.T) {
	path, dir := writeBoard(t)
	base, err := Items(KindStoryboard, path)
	if err != nil || len(base) != 4 {
		t.Fatalf("items %v, %v", base, err)
	}
	if !base[1].HasImage || base[3].HasImage || base[3].SHA == "" {
		t.Fatalf("images: %+v", base)
	}
	if !strings.Contains(base[2].Text, "Camera: push in") {
		t.Fatalf("text %q", base[1].Text)
	}

	// Writing the same shots with other spacing changes nothing.
	b, _ := os.ReadFile(path)
	var raw map[string]any
	json.Unmarshal(b, &raw)
	spaced, _ := json.MarshalIndent(raw, "\t", "\t\t")
	writeFile(t, path, string(spaced))
	again, _ := Items(KindStoryboard, path)
	for _, it := range base {
		if shaOf(again, it.ID) != it.SHA {
			t.Errorf("%s changed with whitespace", it.ID)
		}
	}

	// New frame bytes change that shot only.
	writeFile(t, filepath.Join(dir, "board", "s2.png"), "a new frame")
	frame, _ := Items(KindStoryboard, path)
	if shaOf(frame, "s2") == shaOf(base, "s2") || shaOf(frame, "s1") != shaOf(base, "s1") || shaOf(frame, "s3") != shaOf(base, "s3") {
		t.Errorf("frame bytes should change s2 only: %+v", frame)
	}

	// New words change that shot only.
	sb, _ := board.Load(path)
	sb.Shots[0].What = "logo spins in"
	sb.Save(path)
	what, _ := Items(KindStoryboard, path)
	if shaOf(what, "s1") == shaOf(frame, "s1") || shaOf(what, "s2") != shaOf(frame, "s2") {
		t.Errorf("what should change s1 only: %+v", what)
	}

	// A frame that is not there yet is fine, and the shot changes when it appears.
	os.Remove(filepath.Join(dir, "board", "s1.png"))
	missing, err := Items(KindStoryboard, path)
	if err != nil || missing[1].HasImage || missing[1].SHA == what[1].SHA {
		t.Errorf("missing frame: %+v, %v", missing[1], err)
	}
	writeFile(t, filepath.Join(dir, "board", "s1.png"), "frame s1")
	back, _ := Items(KindStoryboard, path)
	if back[1].SHA == missing[1].SHA || !back[1].HasImage {
		t.Errorf("a new frame should change the shot: %+v", back[1])
	}
}

func TestStoryboardSettingsAreAnItem(t *testing.T) {
	path, _ := writeBoard(t)
	base, _ := Items(KindStoryboard, path)
	if base[0].ID != "board" || !base[0].Meta || base[0].Title != "Storyboard settings" {
		t.Fatalf("first item %+v", base[0])
	}
	// Each setting changes "board" and no shot, so the approval reason reads "board changed".
	for name, edit := range map[string]func(sb *board.Board){
		"bpm":     func(sb *board.Board) { sb.BPM = 90 },
		"fps":     func(sb *board.Board) { sb.FPS = 24 },
		"seconds": func(sb *board.Board) { sb.Seconds = 30 },
		"size":    func(sb *board.Board) { sb.Width, sb.Height = 1080, 1920 },
		"offset":  func(sb *board.Board) { sb.Offset = 0.5 },
		"style":   func(sb *board.Board) { sb.Style.Prompt = "neon" },
		"palette": func(sb *board.Board) { sb.Style.Palette = []string{"#000"} },
	} {
		sb, _ := board.Load(path)
		edit(sb)
		sb.Save(path)
		got, err := Items(KindStoryboard, path)
		if err != nil || shaOf(got, "board") == shaOf(base, "board") {
			t.Errorf("%s: the settings item should change (%v)", name, err)
		}
		for _, id := range []string{"s1", "s2", "s3"} {
			if shaOf(got, id) != shaOf(base, id) {
				t.Errorf("%s: %s should not change", name, id)
			}
		}
		base, _ = Items(KindStoryboard, path) // the next edit starts from this one
	}
}

func TestFrameBytesCountEvenWhenThePageWillNotShowThem(t *testing.T) {
	path, dir := writeBoard(t)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "f.png"), "one")
	if err := os.Symlink(filepath.Join(outside, "f.png"), filepath.Join(dir, "board", "link.png")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	sb, _ := board.Load(path)
	sb.Shots[2].Frame = "board/link.png"
	sb.Save(path)
	a, _ := Items(KindStoryboard, path)
	if a[3].HasImage {
		t.Fatal("a link is not shown")
	}
	writeFile(t, filepath.Join(outside, "f.png"), "two")
	b, _ := Items(KindStoryboard, path)
	if shaOf(a, "s3") == shaOf(b, "s3") {
		t.Error("new bytes in the frame must change the shot, shown or not")
	}
}

func TestMoodboardHashCoversTakeURLAndThumb(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "moodboard.json")
	writeFile(t, filepath.Join(dir, "a.jpg"), "a")
	write := func(extra string) string {
		writeFile(t, path, `{"items":[{"id":"a","file":"a.jpg"`+extra+`}]}`)
		items, err := Items(KindMoodboard, path)
		if err != nil {
			t.Fatal(err)
		}
		return items[0].SHA
	}
	base := write("")
	for _, extra := range []string{`,"take":"the colour"`, `,"url":"https://x.test/a"`, `,"thumb":"t.png"`} {
		if write(extra) == base {
			t.Errorf("%s should change the reference", extra)
		}
	}
}

func TestBriefItemHashes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "brief.json")
	writeFile(t, path, `{"schema":"cav-brief/1","tone":"calm","goal":"sell the app","avoid":["red","stock music"],"zeta":1}`)
	items, err := Items(KindBrief, path)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range items {
		ids = append(ids, it.ID)
		if it.HasImage {
			t.Errorf("%s has an image", it.ID)
		}
	}
	if got := strings.Join(ids, ","); got != "goal,tone,avoid,zeta" {
		t.Fatalf("order %s", got)
	}
	if items[2].Text != "red, stock music" || items[0].Text != "sell the app" {
		t.Fatalf("text %+v", items)
	}

	// Spacing and key order do not matter; one field changing changes that field only.
	writeFile(t, path, "{\n \"zeta\": 1, \"avoid\": [ \"red\", \"stock music\" ],\n \"goal\": \"sell the app\", \"tone\": \"warm\" }")
	changed, _ := Items(KindBrief, path)
	if shaOf(changed, "goal") != shaOf(items, "goal") || shaOf(changed, "avoid") != shaOf(items, "avoid") || shaOf(changed, "tone") == shaOf(items, "tone") {
		t.Fatalf("hashes %+v", changed)
	}
}

func TestMoodboardItemsMinimal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "moodboard.json")
	writeFile(t, filepath.Join(dir, "arena-1234.jpg"), "jpeg bytes")
	writeFile(t, filepath.Join(dir, "thumb.png"), "png bytes")
	writeFile(t, filepath.Join(dir, "clip.mp4"), "video bytes")
	writeFile(t, path, `{"schema":"cav-moodboard/1","items":[
		{"file":"arena-1234.jpg","take":"the colour"},
		{"id":"clip","file":"clip.mp4","thumb":"thumb.png"},
		{"id":"site","url":"https://example.com/a","take":"the layout"}]}`)
	items, err := Items(KindMoodboard, path)
	if err != nil || len(items) != 3 {
		t.Fatalf("%v %v", items, err)
	}
	if items[0].ID != "arena-1234.jpg" || !items[0].HasImage || items[0].Text != "the colour" {
		t.Errorf("file item %+v", items[0])
	}
	if !items[1].HasImage || !strings.HasSuffix(items[1].Image, "thumb.png") {
		t.Errorf("a clip shows its thumb: %+v", items[1])
	}
	if items[2].HasImage || items[2].Link != "https://example.com/a" || items[2].SHA == "" {
		t.Errorf("url item %+v", items[2])
	}
	before := items[0].SHA
	writeFile(t, filepath.Join(dir, "arena-1234.jpg"), "other jpeg bytes")
	items, _ = Items(KindMoodboard, path)
	if items[0].SHA == before {
		t.Error("new file bytes should change the item")
	}
	writeFile(t, path, `{"items":[{"id":"a","url":"x"},{"id":"a","url":"y"}]}`)
	if _, err := Items(KindMoodboard, path); err == nil {
		t.Error("two items with one id should fail")
	}
}

func TestVideoReviewFileRoundTripsWithoutNewKeys(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "v.mp4")
	st := Open(video)
	old := `{
  "schema": "cav-review/1",
  "source": {"render": "v.mp4", "sha256": "abc", "fps": 30, "frames": 10, "width": 4, "height": 4},
  "comments": [{"id": "c_01", "version": "abc", "frame": 3, "timecode": "00:00:00:03", "fragment": "#t=0.1,0.13", "author": "seb",
    "createdAt": "2026-01-01T00:00:00Z", "status": "open", "text": "too slow"}]
}`
	writeFile(t, st.DocPath, old)
	if _, err := st.Update(func(d *Doc) error { d.Comments[0].Text = "much too slow"; return nil }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(st.DocPath)
	for _, key := range []string{`"kind"`, `"approved"`, `"approvalCleared"`, `"item"`, `"itemSha"`, `"verdict"`} {
		if strings.Contains(string(b), key) {
			t.Errorf("a video review file gained %s:\n%s", key, b)
		}
	}

	writeFile(t, st.DocPath, `{"schema":"cav-review/2","comments":[]}`)
	if _, err := st.Load(); err == nil || !strings.Contains(err.Error(), "newer cav") {
		t.Errorf("cav-review/2 should be refused: %v", err)
	}
	writeFile(t, st.DocPath, `{"comments":[]}`)
	if _, err := st.Load(); err != nil {
		t.Errorf("a file without a schema loads: %v", err)
	}
}

func TestCheckOwnerRefusesAReviewFileOfAnotherKind(t *testing.T) {
	dir := t.TempDir()
	sb := filepath.Join(dir, "storyboard.json")
	video := filepath.Join(dir, "storyboard.mp4")
	if err := CheckOwner(sb, KindStoryboard); err != nil {
		t.Fatalf("no review file yet: %v", err)
	}
	doc, _ := Paths(video)
	if err := os.WriteFile(doc, []byte(`{"schema":"cav-review/1","source":{"render":"`+video+`"},"comments":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckOwner(video, KindVideo); err != nil {
		t.Fatalf("the video's own review file: %v", err)
	}
	err := CheckOwner(sb, KindStoryboard)
	if err == nil || !strings.Contains(err.Error(), "storyboard.mp4") {
		t.Fatalf("storyboard.json must not take over the video's review file, got %v", err)
	}
	if err := os.WriteFile(doc, []byte(`{"schema":"cav-review/1","kind":"storyboard","source":{"render":"`+sb+`"},"comments":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckOwner(video, KindVideo); err == nil {
		t.Fatal("the video must not take over the storyboard's review file")
	}
}
