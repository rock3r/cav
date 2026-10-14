package review

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// approveStoryboard writes the approval of every item, as the page does.
func approveStoryboard(t *testing.T, path string) {
	t.Helper()
	items, err := Items(KindStoryboard, path)
	if err != nil {
		t.Fatal(err)
	}
	shas := map[string]string{}
	for _, it := range items {
		shas[it.ID] = it.SHA
	}
	_, err = Open(path).Update(func(d *Doc) error {
		d.Kind = "storyboard"
		d.Source.Render = path
		d.Approved = &Approval{By: "seb", At: time.Now().UTC(), Items: shas}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func fileState(t *testing.T, p string) (string, time.Time) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), st.ModTime()
}

func TestReadStatusApproved(t *testing.T) {
	path, _ := writeBoard(t)
	approveStoryboard(t, path)
	st := ReadStatus(path)
	if !st.Approved || st.ApprovalUnknown || !st.HasReview || st.Kind != KindStoryboard {
		t.Fatalf("status = %+v", st)
	}
	if st.Items != 3 {
		t.Errorf("items = %d, want the 3 shots without the settings", st.Items)
	}
	if st.ReviewFile != Open(path).DocPath {
		t.Errorf("review file = %s", st.ReviewFile)
	}
}

func TestReadStatusNoReviewFile(t *testing.T) {
	path, _ := writeBoard(t)
	st := ReadStatus(path)
	if st.Approved || st.HasReview || st.ApprovalUnknown {
		t.Fatalf("status = %+v", st)
	}
	if _, err := os.Stat(Open(path).DocPath); err == nil {
		t.Error("ReadStatus wrote a review file")
	}
}

func TestReadStatusChangedItemDoesNotWrite(t *testing.T) {
	path, dir := writeBoard(t)
	approveStoryboard(t, path)
	writeFile(t, filepath.Join(dir, "board", "s3.png"), "new")
	// Change shot s3 after the approval.
	b, _ := os.ReadFile(path)
	changed := strings.Replace(string(b), "end card", "end card, longer", 1)
	if changed == string(b) {
		t.Fatal("the test did not change the storyboard")
	}
	writeFile(t, path, changed)
	doc := Open(path).DocPath
	before, mtime := fileState(t, doc)
	listing := func() []string {
		entries, _ := os.ReadDir(filepath.Dir(doc))
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}
	files := listing()
	time.Sleep(20 * time.Millisecond)
	st := ReadStatus(path)
	if st.Approved || st.Cleared != "s3 changed" {
		t.Fatalf("status = %+v", st)
	}
	after, mtime2 := fileState(t, doc)
	if before != after || !mtime.Equal(mtime2) {
		t.Error("ReadStatus changed the review file")
	}
	if !reflect.DeepEqual(files, listing()) {
		t.Errorf("ReadStatus changed the folder: %v -> %v", files, listing())
	}
}

func TestReadStatusRedoAndOpenNote(t *testing.T) {
	path, _ := writeBoard(t)
	items, _ := Items(KindStoryboard, path)
	var s2 string
	for _, it := range items {
		if it.ID == "s2" {
			s2 = it.SHA
		}
	}
	_, err := Open(path).Update(func(d *Doc) error {
		d.Kind = "storyboard"
		d.Comments = append(d.Comments,
			Comment{ID: "c_01", Item: "s2", ItemSHA: s2, Verdict: "redo", Status: "open"},
			Comment{ID: "c_02", Item: "s1", Text: "too slow", Status: "open"},
			Comment{ID: "c_03", Item: "s1", Text: "done", Status: "resolved"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	st := ReadStatus(path)
	if st.Approved {
		t.Error("approved")
	}
	if !reflect.DeepEqual(st.Redo, []string{"s2"}) {
		t.Errorf("redo = %v", st.Redo)
	}
	if !reflect.DeepEqual(st.Open, []string{"c_02"}) {
		t.Errorf("open = %v", st.Open)
	}
}

func TestReadStatusUnknown(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "storyboard.json")
	writeFile(t, broken, "{not json")
	if st := ReadStatus(broken); !st.ApprovalUnknown || st.Approved || st.Reason == "" {
		t.Errorf("broken target: %+v", st)
	}

	path, _ := writeBoard(t)
	approveStoryboard(t, path)
	doc := Open(path).DocPath
	b, _ := os.ReadFile(doc)
	writeFile(t, doc, strings.Replace(string(b), `"cav-review/1"`, `"cav-review/2"`, 1))
	if st := ReadStatus(path); !st.ApprovalUnknown || st.Approved || !strings.Contains(st.Reason, "newer cav") {
		t.Errorf("newer review file: %+v", st)
	}

	// A target that does not load, with an approval on file, is never approved.
	path2, dir2 := writeBoard(t)
	approveStoryboard(t, path2)
	writeFile(t, filepath.Join(dir2, "storyboard.json"), "{")
	if st := ReadStatus(path2); !st.ApprovalUnknown || st.Approved {
		t.Errorf("target that does not load: %+v", st)
	}
}

func TestReadStatusVideoOpenNotes(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "final.mp4")
	writeFile(t, video, "video")
	_, err := Open(video).Update(func(d *Doc) error {
		d.Comments = append(d.Comments,
			Comment{ID: "c_01", Text: "fix the logo", Status: "open"},
			Comment{ID: "c_02", Text: "ok", Status: "resolved"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	st := ReadStatus(video)
	if st.Kind != KindVideo || st.Approved || st.ApprovalUnknown || !reflect.DeepEqual(st.Open, []string{"c_01"}) {
		t.Errorf("status = %+v", st)
	}
}
