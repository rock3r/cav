package imagegen

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

// fakeUV puts a shell script named uv on PATH. It records its arguments in uv.args and
// copies the input image (the second-to-last argument) to the output (the last one).
func fakeUV(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + filepath.Join(dir, "uv.args") + "\"\n" +
		"prev=; last=; for a; do prev=$last; last=$a; done\ncp \"$prev\" \"$last\"\n"
	if err := os.WriteFile(filepath.Join(dir, "uv"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// The command cav runs for rembg, checked with a fake uv, so the test needs neither uv
// nor the 1 GB model.
func TestRembgRunsThePinnedPackageAndModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	dir := fakeUV(t)
	in := writeSquare(t)
	img, err := RemoveBackground(context.Background(), &services.Choice{Service: "rembg"}, in)
	if err != nil {
		t.Fatal(err)
	}
	if img.Provider != "rembg" || img.Model != "birefnet-general" || img.MIME != "image/png" || len(img.Data) == 0 {
		t.Fatalf("image %s %s %s, %d bytes", img.Provider, img.Model, img.MIME, len(img.Data))
	}
	b, err := os.ReadFile(filepath.Join(dir, "uv.args"))
	if err != nil {
		t.Fatal(err)
	}
	a := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{"run", "--no-project", "--quiet", "--python", "3.12", "--with", "rembg[cpu]==2.0.85"}
	if len(a) != len(want)+4 {
		t.Fatalf("uv args %q", a)
	}
	for i, w := range want {
		if a[i] != w {
			t.Fatalf("uv arg %d is %q, want %q (all: %q)", i, a[i], w, a)
		}
	}
	script, model, gotIn, out := a[len(want)], a[len(want)+1], a[len(want)+2], a[len(want)+3]
	if !strings.HasPrefix(filepath.Base(script), "remove_background-") || model != "birefnet-general" || gotIn != in || !strings.HasSuffix(out, ".png") {
		t.Errorf("script %s, model %s, input %s, output %s", script, model, gotIn, out)
	}
	if _, err := RemoveBackground(context.Background(), &services.Choice{Service: "openai"}, in); err == nil {
		t.Error("openai cut out an existing image")
	}
}
