// Package ghfetch downloads a pinned GitHub source tarball into a cache directory.
// The destination only ever appears complete: the tarball is extracted beside it and
// moved into place with one rename, so a killed run never leaves a partial tree.
package ghfetch

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Fetch downloads url (a codeload tar.gz) into dest when ready() is false, and returns
// dest. ready also decides whether a concurrent install that won the rename is usable.
func Fetch(ctx context.Context, client *http.Client, url, dest string, ready func() bool) (string, error) {
	if ready() {
		return dest, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download the source: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download the source: %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	// Stage beside the target: os.Rename cannot cross filesystems.
	tmp, err := os.MkdirTemp(filepath.Dir(dest), ".cav-fetch")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := untar(resp.Body, tmp); err != nil {
		return "", fmt.Errorf("extract the source: %w", err)
	}
	top, err := os.ReadDir(tmp)
	if err != nil || len(top) != 1 {
		return "", fmt.Errorf("extract the source: unexpected layout")
	}
	// The target only ever appears through this rename, so it is never partial. If the
	// rename fails because a concurrent install finished first, that install is fine.
	if err := os.Rename(filepath.Join(tmp, top[0].Name()), dest); err != nil {
		if ready() {
			return dest, nil
		}
		return "", err
	}
	return dest, nil
}

// untar extracts a .tar.gz of regular files and directories into dir, refusing anything
// that would escape it.
func untar(r io.Reader, dir string) error {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p := filepath.Join(dir, filepath.FromSlash(h.Name))
		if !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			f.Close()
			if err != nil {
				return err
			}
		}
	}
}
