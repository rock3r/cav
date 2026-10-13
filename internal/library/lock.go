package library

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// The lock is a file that only one process can create (O_CREATE|O_EXCL). This works the same
// on macOS, Linux and Windows. A cav command holds it for milliseconds: load, change, save.
var (
	// lockWait is how long Update waits for another cav command to finish with the manifest.
	lockWait = 60 * time.Second
	// lockStale is the age after which a lock file is left over from a cav command that
	// stopped (a crash or a kill) and Update removes it. It is shorter than lockWait, so a
	// command that waits always gets past a left-over lock.
	lockStale = 30 * time.Second
	lockPoll  = 10 * time.Millisecond
)

// LockPath is the lock file that guards the manifest.
func LockPath(root string) string { return ManifestPath(root) + ".lock" }

// Update loads the manifest, lets change edit it and saves it, while it holds an exclusive
// lock. Two cav commands that record an asset at the same moment then both keep their entry.
// When change returns an error, Update saves nothing and returns that error.
func Update(root string, change func(m *Manifest) error) error {
	unlock, err := lock(LockPath(root))
	if err != nil {
		return err
	}
	defer unlock()
	m, err := Load(root)
	if err != nil {
		return err
	}
	if err := change(m); err != nil {
		return err
	}
	return m.save(root)
}

func lock(path string) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !busy(err) {
			return nil, fmt.Errorf("lock the manifest: %w", err)
		}
		if info, serr := os.Stat(path); serr == nil && time.Since(info.ModTime()) > lockStale && breakStale(path, info) {
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another cav command has held %s for more than %s; if no cav command is running, delete that file and try again", path, lockWait)
		}
		time.Sleep(lockPoll + rand.N(lockPoll))
	}
}

// breakStale removes a left-over lock and tells whether it did. It holds a second lock file,
// path+".break", while it checks the lock again: only the same old lock that the caller saw is
// removed. Without that, two commands that both saw the old lock could remove a new lock that
// one of them, or a third command, has just made.
func breakStale(path string, seen fs.FileInfo) bool {
	guard := path + ".break"
	g, err := os.OpenFile(guard, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if info, serr := os.Stat(guard); serr == nil && time.Since(info.ModTime()) > lockStale {
			os.Remove(guard) // left by a command that stopped while it removed a lock
		}
		return false
	}
	g.Close()
	defer os.Remove(guard)
	now, err := os.Stat(path)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist) // already gone: try to lock again at once
	}
	// A new lock has a new time, so the same old time means the same old lock.
	if !now.ModTime().Equal(seen.ModTime()) || time.Since(now.ModTime()) <= lockStale {
		return false
	}
	return os.Remove(path) == nil
}

// busy tells whether the lock file could not be made because another process has it. On
// Windows a lock file that is being deleted while another process looks at it gives "access
// denied" for a short time, so that counts as busy too.
func busy(err error) bool {
	return errors.Is(err, fs.ErrExist) || runtime.GOOS == "windows" && errors.Is(err, fs.ErrPermission)
}
