//go:build !windows

package operation

import (
	"errors"
	"os"
	"path/filepath"
)

func syncCheckpointDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func publishCheckpoint(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	return syncCheckpointDirectory(filepath.Dir(to))
}

func publishCheckpointDirectory(from, to string) error {
	if err := syncCheckpointDirectory(from); err != nil {
		return err
	}
	return publishCheckpoint(from, to)
}

func persistExistingCheckpointDirectory(dir string) error {
	return errors.Join(syncCheckpointDirectory(dir), syncCheckpointDirectory(filepath.Dir(dir)))
}
