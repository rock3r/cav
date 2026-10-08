// Package pyrun runs cav's embedded Python helpers through uv, which fetches the Python
// version and packages a helper needs into its own cache: nothing is installed globally.
package pyrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Python is the version the helpers are tested with; uv downloads it when missing.
const Python = "3.12"

// ErrNoUV means uv is not installed.
var ErrNoUV = errors.New("uv is not installed: install it from https://docs.astral.sh/uv/ (cav runs its local models through it)")

// Available reports whether uv is on PATH.
func Available() bool { _, err := exec.LookPath("uv"); return err == nil }

// Run writes the script into cacheDir (named by its content) and runs it with the packages.
// stderr streams to progress (model downloads report there); stdout is returned.
func Run(ctx context.Context, cacheDir, name string, script []byte, packages []string, args []string, progress *os.File) ([]byte, error) {
	if !Available() {
		return nil, ErrNoUV
	}
	sum := sha256.Sum256(script)
	path := filepath.Join(cacheDir, strings.TrimSuffix(name, ".py")+"-"+hex.EncodeToString(sum[:])[:10]+".py")
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, script, 0o644); err != nil {
			return nil, err
		}
	}
	argv := []string{"run", "--no-project", "--quiet", "--python", Python}
	for _, p := range packages {
		argv = append(argv, "--with", p)
	}
	argv = append(argv, path)
	argv = append(argv, args...)
	cmd := exec.CommandContext(ctx, "uv", argv...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	if progress != nil {
		cmd.Stderr = progress
	} else {
		cmd.Stderr = &errb
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 && len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		return nil, fmt.Errorf("%s: %v %s", name, err, msg)
	}
	return out.Bytes(), nil
}
