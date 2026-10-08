package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReaperINI(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir()) // reaperINI resolves symlinks
	if err != nil {
		t.Fatal(err)
	}
	// os.UserHomeDir reads HOME, or USERPROFILE on Windows.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	touch := func(p string) string {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mustINI := func(bin, goos string) string {
		t.Helper()
		ini, err := reaperINI(bin, goos)
		if err != nil {
			t.Fatal(err)
		}
		return ini
	}
	installed := filepath.Join(home, "Applications", "REAPER.app", "Contents", "MacOS", "REAPER")
	touch(installed)
	if got, want := mustINI(installed, "darwin"), filepath.Join(home, "Library", "Application Support", "REAPER", "reaper.ini"); got != want {
		t.Errorf("installed mac REAPER: got %s, want %s", got, want)
	}
	portableMac := touch(filepath.Join(home, "portable", "REAPER.app", "Contents", "MacOS", "REAPER"))
	ini := touch(filepath.Join(home, "portable", "reaper.ini"))
	if got := mustINI(portableMac, "darwin"); got != ini {
		t.Errorf("portable mac REAPER: got %s, want %s", got, ini)
	}
	portableWin := touch(filepath.Join(home, "portable-win", "reaper.exe"))
	ini = touch(filepath.Join(home, "portable-win", "reaper.ini"))
	if got := mustINI(portableWin, "windows"); got != ini {
		t.Errorf("portable Windows REAPER: got %s, want %s", got, ini)
	}
	if got := mustINI(portableWin, "linux"); got != "" {
		t.Errorf("linux: got %s, want none", got)
	}

	// With no home folder, the error reaches the caller instead of skipping the setup.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if _, err := reaperINI(installed, "darwin"); err == nil {
		t.Error("mac with no HOME: want an error")
	}
}
