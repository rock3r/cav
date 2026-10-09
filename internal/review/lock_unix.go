//go:build unix

package review

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive lock on f, waiting for it, until unlockFile or close.
func lockFile(f *os.File) error   { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }
func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
