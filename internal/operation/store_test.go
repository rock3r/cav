package operation

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveCreatesCheckpointParentChainAndReplacesRecord(t *testing.T) {
	home := filepath.Join(t.TempDir(), "new", "home")
	r := &Record{Schema: 1, ID: "durable-fixture", Status: "prepared"}
	if err := Save(home, r); err != nil {
		t.Fatal(err)
	}
	r.Status = "unknown"
	if err := Save(home, r); err != nil {
		t.Fatal(err)
	}
	got, err := Load(home, r.ID)
	if err != nil || got.Status != "unknown" {
		t.Fatalf("record=%+v err=%v", got, err)
	}
	if runtime.GOOS != "windows" {
		for _, dir := range []string{filepath.Dir(home), home, filepath.Join(home, "operations")} {
			st, err := os.Stat(dir)
			if err != nil || st.Mode().Perm() != 0700 {
				t.Fatalf("directory=%s stat=%v err=%v", dir, st, err)
			}
		}
	}
}

func TestSaveDoesNotReportSuccessWhenDirectorySyncFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory-read permission fixture requires unprivileged Unix")
	}
	home := t.TempDir()
	r := &Record{Schema: 1, ID: "sync-fixture", Status: "prepared"}
	if err := Save(home, r); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "operations")
	if err := os.Chmod(dir, 0300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	r.Status = "uncertain"
	// Create/rename remain allowed, while opening the directory for fsync fails.
	if err := Save(home, r); err == nil {
		t.Fatal("publication without its durability barrier reported success")
	}
}
