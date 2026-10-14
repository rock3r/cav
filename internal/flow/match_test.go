package flow

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func touch(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func setFold(t *testing.T, v bool) {
	t.Helper()
	old := foldCase
	foldCase = v
	t.Cleanup(func() { foldCase = old })
}

func TestCheck(t *testing.T) {
	setFold(t, false)
	root := t.TempDir()
	touch(t, root, "brief.json", "style/a.png", "style/b.png", "style/.hidden.png", "style/c.jpg",
		"board/s1.png", "board/deep/s2.png", "only-dot/.DS_Store", "x/aXb",
		"renders/final.mp4", "renders/final.review.json", "renders/review/final/f1.png",
		"snaps/review/f.png", "snaps/note.review.json")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	// ? is not a valid character in a Windows file name.
	if runtime.GOOS != "windows" {
		touch(t, root, "x/a?b")
	}
	starMiddle := []string{"x/a?b", "x/aXb"}
	if runtime.GOOS == "windows" {
		starMiddle = []string{"x/aXb"}
	}
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{"file", "brief.json", []string{"brief.json"}},
		{"missing file", "nope.json", nil},
		{"a folder is not a file", "style", nil},
		{"file is not a folder", "brief.json/", nil},
		{"folder with files", "style/", []string{"style/"}},
		{"nested file counts", "board/", []string{"board/"}},
		{"empty folder", "empty/", nil},
		{"only a hidden file", "only-dot/", nil},
		{"missing folder", "nope/", nil},
		{"pattern", "style/*.png", []string{"style/a.png", "style/b.png"}},
		{"pattern does not cross folders", "board/*.png", []string{"board/s1.png"}},
		{"star in a folder name", "*/s1.png", []string{"board/s1.png"}},
		{"dot file is not matched", "style/*", []string{"style/a.png", "style/b.png", "style/c.jpg"}},
		{"dot file is matched when asked", "style/.*.png", []string{"style/.hidden.png"}},
		{"? is plain", "x/a?b", []string{"x/a?b"}},
		{"? is not a wildcard", "x/a?*", []string{"x/a?b"}},
		{"star in the middle", "x/a*b", starMiddle},
		{"review file named in full", "renders/final.review.json", nil},
		{"review folder named in full", "renders/review/", nil},
		{"file in the review folder named in full", "renders/review/final/f1.png", nil},
		{"review file is not an output", "renders/*", []string{"renders/final.mp4"}},
		{"snapshots do not make a folder", "snaps/", nil},
		{"review folder is not matched", "renders/*/", nil},
		{"pattern with no folder", "nope/*.png", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.Contains(tc.output, "?") {
				t.Skip("? is not valid in a Windows file name")
			}
			got, ok, err := Check(root, tc.output)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Check(%q) = %v, want %v", tc.output, got, tc.want)
			}
			if ok != (len(tc.want) > 0) {
				t.Errorf("Check(%q) ok = %v", tc.output, ok)
			}
		})
	}
}

func TestCheckCaseFolding(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "Style/A.PNG", "Brief.JSON")
	for _, tc := range []struct {
		fold   bool
		output string
		want   []string
	}{
		{true, "style/*.png", []string{"Style/A.PNG"}},
		{true, "brief.json", []string{"Brief.JSON"}},
		{false, "style/*.png", nil},
		{false, "Style/*.png", nil},
		{false, "Style/*.PNG", []string{"Style/A.PNG"}},
	} {
		setFold(t, tc.fold)
		got, _, err := Check(root, tc.output)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("fold %v: Check(%q) = %v, want %v", tc.fold, tc.output, got, tc.want)
		}
	}
}

func TestCheckNeverWrites(t *testing.T) {
	setFold(t, false)
	root := t.TempDir()
	touch(t, root, "a/b.png")
	if _, _, err := Check(root, "a/*.png"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Errorf("root changed: %v", entries)
	}
}
