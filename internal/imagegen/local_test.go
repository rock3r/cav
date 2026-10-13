package imagegen

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

// writeSquare writes a PNG with a black square on white, which any tracer turns into
// at least one path.
func writeSquare(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x >= 16 && x < 48 && y >= 16 && y < 48 {
				c = color.RGBA{0, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	path := filepath.Join(t.TempDir(), "square.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// The real potrace path: ffmpeg makes the bitmap, potrace traces it. CI sets
// CAV_REQUIRE_TRACERS on the job that installs both, so the test cannot skip there.
func TestPotraceTracesARealImage(t *testing.T) {
	for _, bin := range []string{"potrace", "ffmpeg"} {
		if _, err := exec.LookPath(bin); err != nil {
			if os.Getenv("CAV_REQUIRE_TRACERS") != "" {
				t.Fatalf("%s is not installed, and CAV_REQUIRE_TRACERS is set", bin)
			}
			t.Skipf("%s is not installed", bin)
		}
	}
	img, err := Vectorize(context.Background(), &services.Choice{Service: "potrace"}, writeSquare(t))
	if err != nil {
		t.Fatal(err)
	}
	svg := string(img.Data)
	if img.Provider != "potrace" || img.MIME != "image/svg+xml" || !strings.Contains(svg, "<svg") || !strings.Contains(svg, "<path") {
		t.Fatalf("provider %s, mime %s, svg %.200q", img.Provider, img.MIME, svg)
	}
}

// fakeTool puts a shell script named name on PATH. It records its arguments in
// name.args and writes an SVG to the path after the flag outFlag.
func fakeTool(t *testing.T, dir, name, outFlag string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + filepath.Join(dir, name+".args") + "\"\n" +
		"while [ $# -gt 0 ]; do if [ \"$1\" = \"" + outFlag + "\" ]; then printf '<svg><path d=\"M0 0\"/></svg>' > \"$2\"; fi; shift; done\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// The commands cav runs for vtracer and potrace, checked with fake programs, so the test
// needs neither installed.
func TestLocalTracersRunTheExpectedCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake programs are shell scripts")
	}
	dir := t.TempDir()
	fakeTool(t, dir, "vtracer", "--output")
	fakeTool(t, dir, "potrace", "-o")
	// ffmpeg's output is its last argument; the fake just creates it.
	ffmpeg := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + filepath.Join(dir, "ffmpeg.args") + "\"\nfor a; do last=$a; done\n: > \"$last\"\n"
	if err := os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte(ffmpeg), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	in := writeSquare(t)
	args := func(name string) []string {
		b, err := os.ReadFile(filepath.Join(dir, name+".args"))
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}

	img, err := Vectorize(context.Background(), &services.Choice{Service: "vtracer"}, in)
	if err != nil || img.Provider != "vtracer" || !strings.Contains(string(img.Data), "<path") {
		t.Fatalf("vtracer: %+v, %v", img, err)
	}
	if a := args("vtracer"); len(a) != 4 || a[0] != "--input" || a[1] != in || a[2] != "--output" || !strings.HasSuffix(a[3], ".svg") {
		t.Errorf("vtracer args %q", a)
	}

	img, err = Vectorize(context.Background(), &services.Choice{Service: "potrace"}, in)
	if err != nil || img.Provider != "potrace" || !strings.Contains(string(img.Data), "<path") {
		t.Fatalf("potrace: %+v, %v", img, err)
	}
	f := args("ffmpeg")
	if len(f) < 3 || f[len(f)-3] != "-vf" || f[len(f)-2] != "format=monob" || !strings.HasSuffix(f[len(f)-1], ".pbm") {
		t.Errorf("ffmpeg args %q", f)
	}
	if p := args("potrace"); len(p) != 4 || p[0] != "-s" || p[1] != f[len(f)-1] || p[2] != "-o" || !strings.HasSuffix(p[3], ".svg") {
		t.Errorf("potrace args %q (bitmap %s)", p, f[len(f)-1])
	}

	if _, err := Vectorize(context.Background(), &services.Choice{Service: "greybox"}, in); err == nil {
		t.Error("greybox traced an image")
	}
}
