package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/library"
)

// fakeUVOnPath puts a shell script named uv first on PATH.
func fakeUVOnPath(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "uv"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// cav gen cutout skips OpenAI, which cannot cut out an image it did not make, and uses
// rembg through uv (a fake here). The result is recorded in the manifest.
func TestGenCutoutUsesRembgNotOpenAI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "")
	t.Setenv("OPENAI_API_KEY", "k")
	for _, v := range []string{"RECRAFT_API_TOKEN", "RECRAFT_API_KEY"} {
		t.Setenv(v, "")
	}
	// The fake uv copies the input (second-to-last argument) to the output (last).
	fakeUVOnPath(t, "#!/bin/sh\nprev=; last=; for a; do prev=$last; last=$a; done\ncp \"$prev\" \"$last\"\n")
	t.Chdir(t.TempDir())
	if err := os.WriteFile("in.png", []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdGen(&app{}, []string{"cutout", "in.png"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("in-cutout.png"); err != nil {
		t.Fatal(err)
	}
	m, err := library.Load(library.Root())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range m.Assets {
		if strings.HasSuffix(e.Path, "in-cutout.png") && e.Generated != nil && e.Generated.Provider == "rembg" && e.Generated.Model == "birefnet-general" {
			found = true
		}
	}
	if !found {
		t.Errorf("no rembg entry in the manifest: %+v", m.Assets)
	}

	err = cmdGen(&app{}, []string{"cutout", "in.png", "--service", "openai"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "only while it generates") {
		t.Errorf("--service openai: %v", err)
	}
}

// With rembg picked for --alpha, cav needs an image service to make the image first.
// With only greybox usable, it says so instead of cutting out a placeholder card.
func TestGenImageAlphaWithRembgNeedsAnImageService(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	fakeUVOnPath(t, "#!/bin/sh\nexit 1\n")
	t.Chdir(t.TempDir())
	err := cmdGen(&app{}, []string{"image", "a fox", "--alpha"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "only greybox is usable") || !strings.Contains(ce.hint, "cav gen cutout") {
		t.Fatalf("got %v", err)
	}
}
