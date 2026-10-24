package imagegen

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
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

// Ideogram's vectorizer is asynchronous, and the SVG URL refuses a download that
// does not carry the API key. A bare GET is the bug this guards.
func TestIdeogramVectorizePollsAndSendsTheKey(t *testing.T) {
	var polls int
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Api-Key") != "ideo-key" {
			t.Errorf("%s %s: Api-Key %q", r.Method, r.URL.Path, r.Header.Get("Api-Key"))
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/v2/svg/vectorize/ideogram-vector-1":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("mode") != "detailed" || r.FormValue("private") != "true" {
				t.Errorf("form mode=%q private=%q", r.FormValue("mode"), r.FormValue("private"))
			}
			if len(r.MultipartForm.File["image"]) != 1 {
				t.Errorf("image parts: %d", len(r.MultipartForm.File["image"]))
			}
			w.Write([]byte(`{"generation_id":"gen-1"}`))
		case r.Method == "GET" && r.URL.Path == "/v2/generations/gen-1":
			polls++
			if polls == 1 {
				w.Write([]byte(`{"generation_id":"gen-1","status":"pending","created":"2026-10-24T00:00:00Z"}`))
				return
			}
			w.Write([]byte(`{"generation_id":"gen-1","status":"completed","created":"2026-10-24T00:00:00Z","data":[{"object_type":"svg.generation","mime_type":"image/svg+xml","url":"https://ideogram.ai/api/images/ephemeral/logo.svg","width":64,"height":64}]}`))
		case r.Method == "GET" && r.URL.Path == "/api/images/ephemeral/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	img, err := Vectorize(context.Background(), &services.Choice{Service: "ideogram", Key: "ideo-key"}, writeSquare(t))
	if err != nil {
		t.Fatal(err)
	}
	if img.Provider != "ideogram" || img.Model != "ideogram-vector-1" || img.MIME != "image/svg+xml" || !strings.Contains(string(img.Data), "<path") {
		t.Fatalf("%+v", img)
	}
	if polls < 2 {
		t.Fatalf("polls: %d", polls)
	}
}
