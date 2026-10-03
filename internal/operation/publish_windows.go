package operation

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

// MoveFileEx WRITE_THROUGH is the Windows namespace publication barrier; unlike
// Unix, Windows does not expose directory fsync through os.File.Sync.
// https://learn.microsoft.com/windows/win32/api/winbase/nf-winbase-movefileexw
func publishCheckpoint(from, to string) error {
	old, err := checkpointWindowsPath(from)
	if err != nil {
		return err
	}
	next, err := checkpointWindowsPath(to)
	if err != nil {
		return err
	}
	r, _, callErr := moveFileExW.Call(uintptr(unsafe.Pointer(old)), uintptr(unsafe.Pointer(next)), 1|8) // REPLACE_EXISTING | WRITE_THROUGH
	if r == 0 {
		return &os.LinkError{Op: "durable rename", Old: from, New: to, Err: callErr}
	}
	return nil
}

func checkpointWindowsPath(path string) (*uint16, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(abs, `\\?\`) {
		if strings.HasPrefix(abs, `\\`) {
			abs = `\\?\UNC\` + abs[2:]
		} else {
			abs = `\\?\` + abs
		}
	}
	return syscall.UTF16PtrFromString(abs)
}

func publishCheckpointDirectory(from, to string) error {
	return publishCheckpoint(from, to)
}

func persistExistingCheckpointDirectory(dir string) error {
	// A competing directory creator must complete its WRITE_THROUGH move first.
	// Do not report a failed publication as durable based only on an existing path.
	return syscall.ERROR_ALREADY_EXISTS
}
