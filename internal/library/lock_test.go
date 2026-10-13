package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUpdateFromManyGoroutinesKeepsEveryEntry(t *testing.T) {
	root := t.TempDir()
	const n = 64
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Update(root, func(m *Manifest) error {
				m.Put(root, Entry{Path: fmt.Sprintf("a/%03d.png", i), Kind: "image", Licence: "generated"})
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	m, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Assets) != n {
		t.Fatalf("the manifest has %d entries, want %d", len(m.Assets), n)
	}
	if _, err := os.Stat(LockPath(root)); !os.IsNotExist(err) {
		t.Fatalf("the lock file is still there: %v", err)
	}
	assertNoTempFiles(t, root)
}

func TestUpdateRemovesAStaleLock(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LockPath(root), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * lockStale)
	if err := os.Chtimes(LockPath(root), old, old); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := Update(root, func(m *Manifest) error {
		m.Put(root, Entry{Path: "a.png"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("Update waited %s for a stale lock", d)
	}
}

func TestUpdateGivesUpOnAHeldLock(t *testing.T) {
	defer func(w time.Duration) { lockWait = w }(lockWait)
	lockWait = 100 * time.Millisecond
	root := t.TempDir()
	unlock, err := lock(LockPath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	called := false
	err = Update(root, func(*Manifest) error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "another cav command") {
		t.Fatalf("want a clear error about the held lock, got %v", err)
	}
	if called {
		t.Fatal("Update changed the manifest without the lock")
	}
}

func TestUpdateSavesNothingWhenChangeFails(t *testing.T) {
	root := t.TempDir()
	if err := Update(root, func(m *Manifest) error { m.Put(root, Entry{Path: "a.png"}); return nil }); err != nil {
		t.Fatal(err)
	}
	want := fmt.Errorf("no")
	if err := Update(root, func(m *Manifest) error { m.Put(root, Entry{Path: "b.png"}); return want }); err != want {
		t.Fatalf("got %v, want %v", err, want)
	}
	m, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Assets) != 1 {
		t.Fatalf("a failed change was saved: %+v", m.Assets)
	}
}

func TestSaveWritesThroughATemporaryFile(t *testing.T) {
	root := t.TempDir()
	// A temporary file left by a crash does not get in the way, and a save never uses it.
	if err := os.MkdirAll(dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	left := filepath.Join(dir(root), "manifest.json.123.tmp")
	if err := os.WriteFile(left, []byte("{half"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{}
	m.Put(root, Entry{Path: "a.png"})
	if err := m.save(root); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil || len(got.Assets) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if b, _ := os.ReadFile(left); string(b) != "{half" {
		t.Fatal("save wrote into another file's temporary file")
	}
	os.Remove(left)
	assertNoTempFiles(t, root)
	if info, err := os.Stat(ManifestPath(root)); err != nil {
		t.Fatal(err)
	} else if os.PathSeparator == '/' && info.Mode().Perm() != 0o644 {
		t.Fatalf("the manifest has mode %v, want 0644", info.Mode().Perm())
	}
}

func assertNoTempFiles(t *testing.T, root string) {
	t.Helper()
	tmps, _ := filepath.Glob(filepath.Join(dir(root), "*.tmp"))
	if len(tmps) > 0 {
		t.Fatalf("temporary files left behind: %v", tmps)
	}
}
