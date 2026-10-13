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
	"testing"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
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
	// in.png is not in the manifest, so cav cannot know its licence.
	e := entryFor(m, "in-cutout.png")
	if e == nil || e.Generated == nil || e.Generated.Provider != "rembg" || e.Generated.Model != "birefnet-general" || e.Licence != "unknown" || e.CommercialOK {
		t.Errorf("cutout entry %+v", e)
	}
	if !hasProblem(library.Check(library.Root(), m, &library.Project{}), "in-cutout.png", "unknown-licence") {
		t.Error("credits check does not ask about the cutout of an unrecorded image")
	}

	// OpenAI is refused before cav reads its key: with keys off, a key lookup would fail
	// with "no key" instead.
	t.Setenv("CAV_NO_KEYS", "1")
	err = cmdGen(&app{}, []string{"cutout", "in.png", "--service", "openai"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "only while it generates") {
		t.Errorf("--service openai: %v", err)
	}

	// A missing input fails before cav picks a service (and reads its key).
	err = cmdGen(&app{}, []string{"cutout", "nothere.png", "--service", "recraft"})
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "nothere.png") {
		t.Errorf("missing input: %v", err)
	}

	// An order without a cutout service is respected: cav does not add one back.
	c := &services.Config{Order: map[string][]string{"image.alpha": {"openai"}}}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	err = cmdGen(&app{}, []string{"cutout", "in.png"})
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "no service that cuts out") || !strings.Contains(ce.hint, "openai,rembg") {
		t.Errorf("order without a cutout service: %v", err)
	}
	if err := cmdGen(&app{}, []string{"cutout", "in.png", "--service", "rembg"}); err != nil {
		t.Errorf("--service rembg with that order: %v", err)
	}
}

func entryFor(m *library.Manifest, path string) *library.Entry {
	for i := range m.Assets {
		if m.Assets[i].Path == path {
			return &m.Assets[i]
		}
	}
	return nil
}

func hasProblem(ps []library.Problem, path, kind string) bool {
	for _, p := range ps {
		if p.Path == path && p.Kind == kind {
			return true
		}
	}
	return false
}

// A cutout of a licensed image keeps that image's terms and credit, so credits check
// still sees a non-commercial or no-derivatives licence. A cutout of a generated image
// stays generated.
func TestGenCutoutKeepsTheSourceLicence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	fakeUVOnPath(t, "#!/bin/sh\nprev=; last=; for a; do prev=$last; last=$a; done\ncp \"$prev\" \"$last\"\n")
	t.Chdir(t.TempDir())
	for _, f := range []string{"photo.png", "made.png", "mood.png"} {
		if err := os.WriteFile(f, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root := library.Root()
	m := &library.Manifest{}
	m.Put(root, library.Entry{Path: "photo.png", Kind: "image", Source: "openverse", Licence: "CC BY-NC-ND 4.0",
		Author: "Ann", AttributionRequired: true, Attribution: "Photo by Ann", NoDerivatives: true})
	m.Put(root, library.Entry{Path: "made.png", Kind: "image", Source: "gemini", Licence: "generated", CommercialOK: true,
		Generated: &library.Generated{Provider: "gemini"}})
	m.Put(root, library.Entry{Path: "mood.png", Kind: "image", Source: "arena", Licence: "reference-only"})
	if err := m.Save(root); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"photo.png", "made.png", "mood.png"} {
		if err := cmdGen(&app{}, []string{"cutout", f}); err != nil {
			t.Fatal(err)
		}
	}
	m, err := library.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if e := entryFor(m, "photo-cutout.png"); e == nil || e.Generated != nil || e.Licence != "CC BY-NC-ND 4.0" || e.CommercialOK || !e.NoDerivatives || e.Attribution != "Photo by Ann" {
		t.Errorf("cutout of a licensed photo: %+v", e)
	}
	problems := library.Check(root, m, &library.Project{Licence: "commercial"})
	for _, kind := range []string{"nc-in-commercial", "no-derivatives"} {
		if !hasProblem(problems, "photo-cutout.png", kind) {
			t.Errorf("credits check misses %s on the cutout: %+v", kind, problems)
		}
	}
	// A mood-board reference has no licence for use; its cutout is meant for the piece.
	if !hasProblem(problems, "mood-cutout.png", "unknown-licence") {
		t.Errorf("credits check skips the cutout of a reference-only image: %+v", problems)
	}
	if e := entryFor(m, "made-cutout.png"); e == nil || e.Generated == nil || e.Licence != "generated" || !e.CommercialOK {
		t.Errorf("cutout of a generated image: %+v", e)
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
	// An empty Hugging Face cache, so a downloaded mflux model does not make an image.
	for _, v := range []string{"HF_HUB_CACHE", "HF_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(v, t.TempDir())
	}
	fakeUVOnPath(t, "#!/bin/sh\nexit 1\n")
	t.Chdir(t.TempDir())
	err := cmdGen(&app{}, []string{"image", "a fox", "--alpha"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "only greybox is usable") || !strings.Contains(ce.hint, "cav gen cutout") {
		t.Fatalf("got %v", err)
	}
}

// When rembg fails after an image service made the image, cav keeps that image next to
// the output and records it as generated. cav gen cutout on it then keeps its licence.
func TestGenImageAlphaKeepsAndRecordsTheImageWhenTheCutoutFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	const id = "d86e5d8c-e3c5-419c-b328-e9fca3048d2f"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prompt":
			w.Write([]byte(`{"prompt_id": "` + id + `", "number": 1, "node_errors": {}}`))
		case "/history/" + id:
			w.Write([]byte(`{"` + id + `": {"outputs": {"9": {"images": [{"filename": "cav_1.png", "subfolder": "", "type": "output"}]}}}}`))
		case "/view":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pic.Bytes())
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("CAV_HOME", home)
	t.Setenv("CAV_NO_KEYS", "1")
	dir := t.TempDir()
	wf := filepath.Join(dir, "workflow.json")
	os.WriteFile(wf, []byte(`{"6": {"inputs": {"text": "{{prompt}}"}}}`), 0o644)
	cfg, _ := json.Marshal(services.Config{
		Order:     map[string][]string{"image": {"comfyui"}, "image.alpha": {"rembg"}},
		Endpoints: map[string]services.Endpoint{"comfyui": {BaseURL: srv.URL, Model: wf}},
	})
	if err := os.WriteFile(services.Path(), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	fakeUVOnPath(t, "#!/bin/sh\nexit 1\n")
	t.Chdir(dir)
	// A file already named fox-uncut.png is left alone.
	os.WriteFile("fox-uncut.png", []byte("mine"), 0o644)
	err := cmdGen(&app{}, []string{"image", "a fox", "--alpha", "-o", "fox.png"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "kept at fox-uncut-2.png") || !strings.Contains(ce.hint, "cav gen cutout 'fox-uncut-2.png'") {
		t.Fatalf("got %v", err)
	}
	if b, _ := os.ReadFile("fox-uncut.png"); string(b) != "mine" {
		t.Fatal("the kept image replaced an existing file")
	}
	m, err := library.Load(library.Root())
	if err != nil {
		t.Fatal(err)
	}
	if e := entryFor(m, "fox-uncut-2.png"); e == nil || e.Generated == nil || e.Generated.Provider != "comfyui" {
		t.Fatalf("kept image entry %+v", e)
	}
}

// cav gen cutout refuses a folder as -o before it runs a service.
func TestGenCutoutRefusesAFolderOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	fakeUVOnPath(t, "#!/bin/sh\necho ran > \"$CAV_HOME/ran\"\nexit 1\n")
	t.Chdir(t.TempDir())
	os.WriteFile("in.png", []byte("\x89PNG\r\n\x1a\nfake"), 0o644)
	os.Mkdir("assets", 0o755)
	err := cmdGen(&app{}, []string{"cutout", "in.png", "--service", "rembg", "-o", "assets"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "is a folder") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("CAV_HOME"), "ran")); err == nil {
		t.Fatal("the cutout service ran before the output was checked")
	}
}

func TestFreePathReportsErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	os.Mkdir(dir, 0o000)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if _, err := freePath(filepath.Join(dir, "fox-uncut"), ".png"); err == nil && os.Geteuid() != 0 {
		t.Fatal("no error for a folder cav cannot search")
	}
}
