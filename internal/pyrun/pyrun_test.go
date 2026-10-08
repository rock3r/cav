package pyrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCachesScriptAndPassesArgs(t *testing.T) {
	if !Available() {
		t.Skip("uv is not installed")
	}
	dir := t.TempDir()
	script := []byte("import sys\nprint('|'.join(sys.argv[1:]))\n")
	out, err := Run(context.Background(), dir, "echo.py", script, nil, []string{"a b", "c"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "a b|c" {
		t.Fatalf("got %q", got)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "echo-*.py"))
	if len(files) != 1 {
		t.Fatalf("cached scripts: %v", files)
	}
	if _, err := Run(context.Background(), dir, "fail.py", []byte("import sys\nsys.stderr.write('boom')\nsys.exit(3)\n"), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want the script's stderr in the error, got %v", err)
	}
}

func TestRunWithoutUV(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Run(context.Background(), t.TempDir(), "x.py", nil, nil, nil, os.Stderr); err != ErrNoUV {
		t.Fatalf("got %v", err)
	}
}
