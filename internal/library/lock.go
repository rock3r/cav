package library

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	mrand "math/rand/v2"
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
	l, err := lock(LockPath(root))
	if err != nil {
		return err
	}
	defer l.unlock()
	m, err := Load(root)
	if err != nil {
		return err
	}
	if err := change(m); err != nil {
		return err
	}
	// A command that was paused for longer than lockStale (a suspended laptop, a stopped
	// process) may have lost its lock to another command. It then must not save over that
	// command's changes.
	if !l.owned() {
		return fmt.Errorf("cav waited so long that another cav command took over %s; run the command again", l.path)
	}
	return m.save(root)
}

// heldLock is a lock this process made. Its file holds a random token, so the process can
// tell its own lock from a lock that another command made after it removed this one.
type heldLock struct {
	path, token string
}

func (l *heldLock) owned() bool {
	b, err := os.ReadFile(l.path)
	return err == nil && string(b) == l.token
}

// unlock removes the lock file only when it is still this process's lock.
func (l *heldLock) unlock() {
	if l.owned() {
		os.Remove(l.path)
	}
}

func lock(path string) (*heldLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(lockWait)
	denied := 0
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			l := &heldLock{path: path, token: fmt.Sprintf("%d %s\n", os.Getpid(), randomHex())}
			_, err = f.WriteString(l.token)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				os.Remove(path)
				return nil, fmt.Errorf("lock the manifest: %w", err)
			}
			return l, nil
		}
		switch busy(path, err) {
		case notBusy:
			return nil, fmt.Errorf("lock the manifest: %w", err)
		case maybeBusy:
			// Windows: the lock may have gone between the open and the look. Try a few
			// times before cav calls the folder unwritable.
			if denied++; denied > 20 {
				return nil, fmt.Errorf("lock the manifest: %w", err)
			}
		default:
			denied = 0
		}
		if info, serr := os.Stat(path); serr == nil && time.Since(info.ModTime()) > lockStale && breakStale(path, info) {
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another cav command has held %s for more than %s; if no cav command is running, delete that file (and %s.break, if it is there) and try again", path, lockWait, path)
		}
		time.Sleep(lockPoll + mrand.N(lockPoll))
	}
}

// breakStale removes a left-over lock and tells whether it did. It holds a second lock file,
// path+".break", while it checks the lock again: only the same old lock that the caller saw is
// removed. Without that, two commands that both saw the old lock could remove a new lock that
// one of them, or a third command, has just made.
//
// A command holds the .break file for less than a millisecond, so cav never removes it on its
// own: removing it safely would need a third lock. If a command stops while it holds it, the
// waiting commands time out with an error that names it.
func breakStale(path string, seen fs.FileInfo) bool {
	guard := path + ".break"
	g, err := os.OpenFile(guard, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
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

const (
	notBusy = iota
	isBusy
	maybeBusy
)

// busy tells whether the lock file could not be made because another process has it. On
// Windows a lock file that is being deleted while another process looks at it gives "access
// denied" for a short time, so that counts as busy too while the file is there. Without the
// file it is maybeBusy: the lock may just have gone, or the folder may be unwritable.
func busy(path string, err error) int {
	if errors.Is(err, fs.ErrExist) {
		return isBusy
	}
	if runtime.GOOS != "windows" || !errors.Is(err, fs.ErrPermission) {
		return notBusy
	}
	if _, serr := os.Stat(path); errors.Is(serr, fs.ErrNotExist) {
		return maybeBusy
	}
	return isBusy
}

func randomHex() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
