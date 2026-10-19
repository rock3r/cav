package music

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/sa3"
	"github.com/rock3r/cav/internal/services"
)

// fakeSA3 fakes the pinned runtime in the test's CAV_HOME and puts a uv script on PATH
// that records its arguments in $SA3_ARGS and writes a fake WAV to --out.
func fakeSA3(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("sa3 runs on Apple silicon only")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("HF_HUB_CACHE", t.TempDir())
	dir := filepath.Join(sa3.RuntimeDir(), "optimized", "mlx", "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "sa3_mlx.py"), []byte("#"), 0o644)
	os.WriteFile(filepath.Join(dir, "..", "requirements.txt"), []byte("mlx\n"), 0o644)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("SA3_ARGS", argsFile)
	bin := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$@" > "$SA3_ARGS"
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--out" ]; then out="$a"; fi
  prev="$a"
done
printf 'RIFFfake' > "$out"
`
	if err := os.WriteFile(filepath.Join(bin, "uv"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsFile
}

func sa3Arg(t *testing.T, argsFile, flag string) string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	for i, l := range lines {
		if l == flag && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	return ""
}

// sa3 runs locally through uv: the take is a WAV, its seed is recorded for the manifest,
// and the prompt carries the model's music prefix and the section plan.
func TestSA3GeneratesLocally(t *testing.T) {
	argsFile := fakeSA3(t)
	tr, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "sa3"},
		Request{Prompt: "chill beat", Seconds: 5, Sections: []Section{{Name: "intro", Seconds: 5, Styles: []string{"soft pads"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Ext != ".wav" || tr.Provider != "sa3" || tr.Model != "sm-music" || tr.Seed == nil {
		t.Fatalf("track: %+v", tr)
	}
	want := "TrackType: Music, VocalType: Instrumental, chill beat. Structure: intro at 0s (soft pads)"
	if got := sa3Arg(t, argsFile, "--prompt"); got != want {
		t.Fatalf("prompt:\n got %q\nwant %q", got, want)
	}
	if got := sa3Arg(t, argsFile, "--seconds"); got != "5" {
		t.Fatalf("seconds: %q", got)
	}
}

// A reference track goes to sa3 as init audio; --keep maps to its noise level the same
// way it maps to Stable Audio's strength (1 keeps everything, but not quite: a take of
// nothing new is no take).
func TestSA3ReferenceBecomesInitAudio(t *testing.T) {
	argsFile := fakeSA3(t)
	ref := filepath.Join(t.TempDir(), "ref.wav")
	os.WriteFile(ref, []byte("RIFFref"), 0o644)
	_, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "sa3"},
		Request{Prompt: "x", Seconds: 5, Ref: ref, Keep: 0.4})
	if err != nil {
		t.Fatal(err)
	}
	if got := sa3Arg(t, argsFile, "--init-audio"); got != ref {
		t.Fatalf("init audio: %q", got)
	}
	if got := sa3Arg(t, argsFile, "--init-noise-level"); got != "0.60" {
		t.Fatalf("init noise: %q", got)
	}
}

// sa3 makes only instrumentals.
func TestSA3RefusesLyrics(t *testing.T) {
	if SingsLyrics("sa3") {
		t.Fatal("sa3 must not claim to sing")
	}
	_, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "sa3"},
		Request{Prompt: "x", Lyrics: "[Verse]\nla"})
	if err == nil || !strings.Contains(err.Error(), "sa3 makes only instrumentals") {
		t.Fatalf("got %v", err)
	}
}

// A take with no requested seed gets a fresh random one: two takes of the same prompt
// (--takes 2) must not come out identical.
func TestSA3PicksAFreshSeedPerTake(t *testing.T) {
	fakeSA3(t)
	r := Request{Prompt: "x", Seconds: 1}
	a, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "sa3"}, r)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "sa3"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if a.Seed == nil || b.Seed == nil || *a.Seed == *b.Seed {
		t.Fatalf("seeds %v and %v", a.Seed, b.Seed)
	}
}
