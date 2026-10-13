package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
)

// skipPermissionTests skips tests that need a read-only file or folder: Windows has no
// such modes, and root can write anyway.
func skipPermissionTests(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("permissions and shell-script fakes")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write read-only files")
	}
}

// readOnly makes path read-only until the test ends.
func readOnly(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o444)
	if fi.IsDir() {
		mode = 0o555
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, fi.Mode().Perm()|0o200) })
}

// comfyForTest starts a fake ComfyUI server that returns a PNG and calls onImage before
// it sends the image. It configures cav to use it for the image job. It returns the
// number of requests the server got.
func comfyForTest(t *testing.T, onImage func()) *atomic.Int32 {
	t.Helper()
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	const id = "d86e5d8c-e3c5-419c-b328-e9fca3048d2f"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/prompt":
			w.Write([]byte(`{"prompt_id": "` + id + `", "number": 1, "node_errors": {}}`))
		case "/history/" + id:
			w.Write([]byte(`{"` + id + `": {"outputs": {"9": {"images": [{"filename": "cav_1.png", "subfolder": "", "type": "output"}]}}}}`))
		case "/view":
			if onImage != nil {
				onImage()
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(pic.Bytes())
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	wf := filepath.Join(t.TempDir(), "workflow.json")
	os.WriteFile(wf, []byte(`{"6": {"inputs": {"text": "{{prompt}}"}}}`), 0o644)
	cfg, _ := json.Marshal(services.Config{
		Order:     map[string][]string{"image": {"comfyui"}},
		Endpoints: map[string]services.Endpoint{"comfyui": {BaseURL: srv.URL, Model: wf}},
	})
	if err := os.WriteFile(services.Path(), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	return &hits
}

// fakeToolOnPath puts a shell script with the given name first on PATH.
func fakeToolOnPath(t *testing.T, name, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func wantCLIError(t *testing.T, what string, err error, parts ...string) {
	t.Helper()
	var ce *cliError
	if !errors.As(err, &ce) {
		t.Fatalf("%s: got %v", what, err)
	}
	for _, p := range parts {
		if !strings.Contains(ce.msg+" | "+ce.hint, p) {
			t.Errorf("%s: %q does not say %q", what, ce.msg+" | "+ce.hint, p)
		}
	}
}

// cav gen image, cutout and vector refuse an output in a read-only folder, or a
// read-only existing file, before the service runs.
func TestGenRefusesAnOutputItCannotWrite(t *testing.T) {
	skipPermissionTests(t)
	dir := t.TempDir()
	t.Chdir(dir)
	hits := comfyForTest(t, nil)
	ran := filepath.Join(dir, "ran")
	t.Setenv("RAN", ran)
	fakeToolOnPath(t, "uv", "#!/bin/sh\necho uv >> \"$RAN\"\nexit 1\n")
	fakeToolOnPath(t, "vtracer", "#!/bin/sh\necho vtracer >> \"$RAN\"\nexit 1\n")
	os.WriteFile("in.png", []byte("\x89PNG\r\n\x1a\nfake"), 0o644)
	os.Mkdir("locked", 0o755)
	os.Mkdir("assets", 0o755)
	os.WriteFile("old.png", []byte("mine"), 0o644)
	os.WriteFile("old.svg", []byte("mine"), 0o644)
	readOnly(t, "locked")
	readOnly(t, "assets")
	readOnly(t, "old.png")
	readOnly(t, "old.svg")

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"image", "a fox", "-o", "locked/fox.png"}, "cannot write in locked"},
		{[]string{"image", "a fox", "-o", "old.png"}, "cannot write old.png"},
		{[]string{"image", "a fox"}, "cannot write in assets"},
		{[]string{"cutout", "in.png", "--service", "rembg", "-o", "locked/fox.png"}, "cannot write in locked"},
		{[]string{"cutout", "in.png", "--service", "rembg", "-o", "old.png"}, "cannot write old.png"},
		{[]string{"vector", "in.png", "--service", "vtracer", "-o", "locked/fox.svg"}, "cannot write in locked"},
		{[]string{"vector", "in.png", "--service", "vtracer", "-o", "old.svg"}, "cannot write old.svg"},
	}
	for _, c := range cases {
		wantCLIError(t, strings.Join(c.args, " "), cmdGen(&app{}, c.args), c.want)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the image service got %d requests", n)
	}
	if b, err := os.ReadFile(ran); err == nil {
		t.Errorf("a service ran: %s", b)
	}
	for _, f := range []string{"old.png", "old.svg"} {
		if b, _ := os.ReadFile(f); string(b) != "mine" {
			t.Errorf("the check changed %s", f)
		}
	}
	if left, _ := filepath.Glob("locked/*"); len(left) > 0 {
		t.Errorf("files left in the locked folder: %v", left)
	}
}

// keptAndRecorded checks that the image is at kept in the working folder and recorded
// in the manifest with the given provider.
func keptAndRecorded(t *testing.T, err error, kept, provider string) {
	t.Helper()
	wantCLIError(t, "save after the call", err, "kept at "+kept)
	if _, serr := os.Stat(kept); serr != nil {
		t.Fatal(serr)
	}
	m, lerr := library.Load(library.Root())
	if lerr != nil {
		t.Fatal(lerr)
	}
	if e := entryFor(m, kept); e == nil || e.Generated == nil || !strings.HasPrefix(e.Generated.Provider, provider) {
		t.Errorf("kept image entry %+v", e)
	}
}

// When the folder becomes read-only during a paid call, cav gen image keeps the image
// under a free name in the working folder and records it in the manifest.
func TestGenImageKeepsTheImageWhenSavingFails(t *testing.T) {
	skipPermissionTests(t)
	t.Chdir(t.TempDir())
	os.Mkdir("out", 0o755)
	t.Cleanup(func() { os.Chmod("out", 0o755) })
	comfyForTest(t, func() { os.Chmod("out", 0o555) })
	// A file already named fox.png is left alone.
	os.WriteFile("fox.png", []byte("mine"), 0o644)
	err := cmdGen(&app{}, []string{"image", "a fox", "-o", "out/fox.png"})
	keptAndRecorded(t, err, "fox-2.png", "comfyui")
	if b, _ := os.ReadFile("fox.png"); string(b) != "mine" {
		t.Fatal("the kept image replaced an existing file")
	}
}

func TestGenCutoutKeepsTheImageWhenSavingFails(t *testing.T) {
	skipPermissionTests(t)
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	os.Mkdir("out", 0o755)
	os.WriteFile("in.png", []byte("\x89PNG\r\n\x1a\nfake"), 0o644)
	t.Setenv("LOCK", filepath.Join(dir, "out"))
	t.Cleanup(func() { os.Chmod(filepath.Join(dir, "out"), 0o755) })
	// The fake uv locks the output folder, then copies the input (second-to-last
	// argument) to the output (last).
	fakeToolOnPath(t, "uv", "#!/bin/sh\nchmod 555 \"$LOCK\"\nprev=; last=; for a; do prev=$last; last=$a; done\ncp \"$prev\" \"$last\"\n")
	err := cmdGen(&app{}, []string{"cutout", "in.png", "--service", "rembg", "-o", "out/fox.png"})
	keptAndRecorded(t, err, "fox.png", "rembg")
}

func TestGenVectorKeepsTheImageWhenSavingFails(t *testing.T) {
	skipPermissionTests(t)
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	os.Mkdir("out", 0o755)
	os.WriteFile("in.png", []byte("\x89PNG\r\n\x1a\nfake"), 0o644)
	t.Setenv("LOCK", filepath.Join(dir, "out"))
	t.Cleanup(func() { os.Chmod(filepath.Join(dir, "out"), 0o755) })
	// The fake vtracer locks the output folder, then writes an SVG to --output (last).
	fakeToolOnPath(t, "vtracer", "#!/bin/sh\nchmod 555 \"$LOCK\"\nlast=; for a; do last=$a; done\necho '<svg xmlns=\"http://www.w3.org/2000/svg\"/>' > \"$last\"\n")
	err := cmdGen(&app{}, []string{"vector", "in.png", "--service", "vtracer", "-o", "out/fox.svg"})
	keptAndRecorded(t, err, "fox.svg", "vtracer")
}
