package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodYT = "https://www.youtube.com/watch?v=aqz-KE-bpKQ"

func moodFixture(t *testing.T) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "moodboard.json")
	if err := os.MkdirAll(filepath.Join(dir, "moodboard", ".thumbs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "moodboard", "a.jpg"), "image bytes")
	writeFile(t, filepath.Join(dir, "moodboard", "reel.mp4"), strings.Repeat("v", 5000))
	writeFile(t, filepath.Join(dir, "moodboard", ".thumbs", "reel.png"), "sheet")
	writeFile(t, filepath.Join(dir, "moodboard", ".thumbs", "yt.jpg"), "thumb")
	return dir, path
}

func TestMoodboardEmbedIsRebuiltFromTheParsedID(t *testing.T) {
	_, path := moodFixture(t)
	writeFile(t, path, `{"schema":"cav-moodboard/1","dir":"moodboard","items":[
		{"id":"good","kind":"embed","url":"`+goodYT+`&list=junk","thumb":"moodboard/.thumbs/yt.jpg","licence":"reference-only","addedBy":"brief","take":"the pace"},
		{"id":"evil","kind":"embed","url":"https://evil.test/embed/aqz-KE-bpKQ"},
		{"id":"js","kind":"embed","url":"javascript:alert(1)"},
		{"id":"quote","kind":"embed","url":"https://www.youtube.com/watch?v=\"onload=alert(1)"},
		{"id":"vim","kind":"embed","url":"https://vimeo.com/123456789/abcdef1234"},
		{"id":"kindless","url":"https://youtu.be/aqz-KE-bpKQ"}]}`)
	items, err := Items(KindMoodboard, path)
	if err != nil || len(items) != 6 {
		t.Fatalf("%v %v", items, err)
	}
	by := map[string]Item{}
	for _, it := range items {
		by[it.ID] = it
	}
	if got := by["good"].Embed; got != "https://www.youtube-nocookie.com/embed/aqz-KE-bpKQ?rel=0&playsinline=1" {
		t.Errorf("player %q", got)
	}
	if by["good"].Link != goodYT {
		t.Errorf("the link shown is the canonical one, got %q", by["good"].Link)
	}
	if !by["good"].HasImage || by["good"].Text != "the pace" {
		t.Errorf("thumb and take: %+v", by["good"])
	}
	if tags := strings.Join(by["good"].Tags, ","); tags != "youtube,embed,reference only,from the brief" {
		t.Errorf("tags %s", tags)
	}
	for _, id := range []string{"evil", "js", "quote"} {
		if by[id].Embed != "" {
			t.Errorf("%s got a player: %q", id, by[id].Embed)
		}
	}
	if by["vim"].Embed != "https://player.vimeo.com/video/123456789?dnt=1&h=abcdef1234" {
		t.Errorf("vimeo player %q", by["vim"].Embed)
	}
	if by["kindless"].Kind != "embed" || by["kindless"].Embed == "" {
		t.Errorf("a link without a kind: %+v", by["kindless"])
	}
}

func TestMoodboardSkipsMissingAndDroppedAndKeepsTheHashSteady(t *testing.T) {
	_, path := moodFixture(t)
	body := func(take, extra string) string {
		return `{"schema":"cav-moodboard/1","dir":"moodboard","items":[
		{"id":"a","kind":"image","file":"moodboard/a.jpg","take":"` + take + `","source":"openverse","licence":"CC-BY-4.0","credit":"Photo by X"},
		{"id":"gone","kind":"image","file":"moodboard/gone.jpg","status":"missing"},
		{"id":"nope","kind":"image","file":"moodboard/a.jpg","status":"dropped"}` + extra + `]}`
	}
	writeFile(t, path, body("colour", ""))
	items, err := Items(KindMoodboard, path)
	if err != nil || len(items) != 1 || items[0].ID != "a" {
		t.Fatalf("%+v %v", items, err)
	}
	before := items[0].SHA
	if tags := strings.Join(items[0].Tags, ","); tags != "openverse,CC BY 4.0" || items[0].Credit != "Photo by X" {
		t.Errorf("tags %s credit %q", tags, items[0].Credit)
	}
	// A sync that changes nothing in this item changes nothing in its hash, even when other
	// items come and go.
	writeFile(t, path, body("colour", `,{"id":"new","kind":"image","file":"moodboard/a.jpg"}`))
	items, _ = Items(KindMoodboard, path)
	if len(items) != 2 || items[0].SHA != before {
		t.Fatalf("hash moved: %+v", items)
	}
	// A new take line is a new version of the item, on purpose.
	writeFile(t, path, body("rhythm", ""))
	items, _ = Items(KindMoodboard, path)
	if items[0].SHA == before {
		t.Error("a changed take must change the hash")
	}
}

func TestMoodboardApprovalClearsWhenAnItemIsDropped(t *testing.T) {
	_, path := moodFixture(t)
	writeFile(t, path, `{"schema":"cav-moodboard/1","items":[{"id":"a","kind":"image","file":"moodboard/a.jpg"},{"id":"b","kind":"image","file":"moodboard/a.jpg"}]}`)
	s := &Server{Store: Open(path), Stills: &Stills{Kind: KindMoodboard, Path: path}, Author: "seb", Nonce: testNonce}
	h := s.Handler()
	approveStage(t, h, getState(t, h), 200)
	writeFile(t, path, `{"schema":"cav-moodboard/1","items":[{"id":"a","kind":"image","file":"moodboard/a.jpg"},{"id":"b","kind":"image","file":"moodboard/a.jpg","status":"dropped"}]}`)
	st := getState(t, h)
	if st.Approval.Approved || st.ApprovalCleared == nil || st.ApprovalCleared.Reason != "b removed" {
		t.Fatalf("%+v %+v", st.Approval, st.ApprovalCleared)
	}
}

func TestClipEndpointServesRangesAndRefusesWhatSafeClipRefuses(t *testing.T) {
	dir, path := moodFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	writeFile(t, outside, "secret video")
	if err := os.Symlink(outside, filepath.Join(dir, "moodboard", "link.mp4")); err != nil {
		t.Skip("no symlinks: ", err)
	}
	writeFile(t, filepath.Join(dir, "moodboard", "notes.txt"), "text")
	writeFile(t, path, `{"schema":"cav-moodboard/1","dir":"moodboard","items":[
		{"id":"reel","kind":"clip","file":"moodboard/reel.mp4","thumb":"moodboard/.thumbs/reel.png","seconds":2},
		{"id":"link","kind":"clip","file":"moodboard/link.mp4"},
		{"id":"abs","kind":"clip","file":`+jsonStr(outside)+`},
		{"id":"up","kind":"clip","file":"../outside.mp4"},
		{"id":"txt","kind":"clip","file":"moodboard/notes.txt"},
		{"id":"img","kind":"image","file":"moodboard/a.jpg"}]}`)
	s := &Server{Store: Open(path), Stills: &Stills{Kind: KindMoodboard, Path: path}, Author: "seb", Nonce: testNonce}
	h := s.Handler()

	w := do(t, h, "GET", "/api/item/reel/clip", nil, map[string]string{"Range": "bytes=10-19"})
	if w.Code != 206 || w.Body.Len() != 10 || !strings.HasPrefix(w.Header().Get("Content-Type"), "video/mp4") ||
		w.Header().Get("Content-Range") != "bytes 10-19/5000" {
		t.Fatalf("range: %d %v %d", w.Code, w.Header(), w.Body.Len())
	}
	if w := do(t, h, "GET", "/api/item/reel/clip", nil, nil); w.Code != 200 || w.Body.Len() != 5000 {
		t.Fatalf("whole: %d %d", w.Code, w.Body.Len())
	}
	for _, id := range []string{"link", "abs", "up", "txt", "img", "nothing", "..%2Foutside"} {
		if w := do(t, h, "GET", "/api/item/"+id+"/clip", nil, nil); w.Code != 404 {
			t.Errorf("%s: %d", id, w.Code)
		}
	}
	// The clip's path never appears in what the page is told.
	st := do(t, h, "GET", "/api/state", nil, nil).Body.String()
	if strings.Contains(st, "reel.mp4") || strings.Contains(st, dir) {
		t.Errorf("state leaks a path: %s", st)
	}
	s2 := getState(t, h)
	if it := s2.item("reel"); it == nil || it["hasClip"] != true || it["kind"] != "clip" || it["hasImage"] != true {
		t.Errorf("reel in state: %v", it)
	}
	if it := s2.item("link"); it["hasClip"] == true {
		t.Errorf("a symlinked clip must not play: %v", it)
	}
}

// jsonStr quotes a path for a JSON fixture: a Windows path has backslashes.
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
