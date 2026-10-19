package sa3

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeUVOnPath puts a shell script named uv first on PATH. The script records its
// arguments, one per line, in $SA3_ARGS, and writes a fake WAV to the --out path.
func fakeUVOnPath(t *testing.T) string {
	t.Helper()
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

// makeRuntime fakes the pinned checkout in the test's CAV_HOME, so Ensure skips the fetch.
func makeRuntime(t *testing.T) {
	t.Helper()
	dir := filepath.Join(RuntimeDir(), "optimized", "mlx", "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sa3_mlx.py"), []byte("#"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(RuntimeDir(), "optimized", "mlx", "requirements.txt"), []byte("mlx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupAppleSilicon fakes the whole local environment; it skips elsewhere.
func setupAppleSilicon(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("sa3 runs on Apple silicon only")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("HF_HUB_CACHE", t.TempDir())
	makeRuntime(t)
	return fakeUVOnPath(t)
}

// argValue returns the argument after flag in the recorded uv call, one per line.
func argValue(t *testing.T, argsFile, flag string) string {
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

func TestPromptForAddsTheTrackType(t *testing.T) {
	if got := promptFor(SFX, "metal clank"); got != PrefixSFX+"metal clank" {
		t.Fatalf("sfx: %q", got)
	}
	if got := promptFor(Music, "lo-fi beat"); got != PrefixMusic+"lo-fi beat" {
		t.Fatalf("music: %q", got)
	}
	// A prompt that already carries a TrackType is left alone (any case, any spacing).
	p := "  tracktype: Instrument, a lone cello"
	if got := promptFor(Music, p); got != p {
		t.Fatalf("prefixed: %q", got)
	}
}

func TestDefaultModel(t *testing.T) {
	if DefaultModel(SFX) != "sm-sfx" || DefaultModel(Music) != "sm-music" {
		t.Fatalf("defaults: %s %s", DefaultModel(SFX), DefaultModel(Music))
	}
}

// A bad model name, and medium for a sound effect, are refused before the Apple-silicon
// check, so the error is the same on every platform.
func TestGenerateValidatesTheModelFirst(t *testing.T) {
	_, err := Generate(context.Background(), Request{Kind: Music, Prompt: "x", Model: "huge"})
	if err == nil || !strings.Contains(err.Error(), `unknown sa3 model "huge"`) {
		t.Fatalf("unknown model: %v", err)
	}
	_, err = Generate(context.Background(), Request{Kind: SFX, Prompt: "x", Model: "medium"})
	if err == nil || !strings.Contains(err.Error(), "medium is a music model") {
		t.Fatalf("sfx with medium: %v", err)
	}
}

func TestGenerateRunsTheMLXScript(t *testing.T) {
	argsFile := setupAppleSilicon(t)
	res, err := Generate(context.Background(), Request{Kind: SFX, Prompt: "metal clank", Seconds: 2, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Data) != "RIFFfake" || res.Model != "sm-sfx" || res.Seed != 7 {
		t.Fatalf("result: %+v", res)
	}
	if got := argValue(t, argsFile, "--prompt"); got != PrefixSFX+"metal clank" {
		t.Fatalf("prompt: %q", got)
	}
	if got := argValue(t, argsFile, "--dit"); got != "sm-sfx" {
		t.Fatalf("dit: %q", got)
	}
	if got := argValue(t, argsFile, "--decoder"); got != "same-s" {
		t.Fatalf("decoder: %q", got)
	}
	if got := argValue(t, argsFile, "--seconds"); got != "2" {
		t.Fatalf("seconds: %q", got)
	}
	if got := argValue(t, argsFile, "--seed"); got != "7" {
		t.Fatalf("seed: %q", got)
	}
	if !filepath.IsAbs(argValue(t, argsFile, "--out")) {
		t.Fatal("--out is not absolute")
	}
}

func TestGenerateDefaultsAndCapsTheLength(t *testing.T) {
	argsFile := setupAppleSilicon(t)
	// A sound effect with no length is 5 s; music with none is 30 s.
	if _, err := Generate(context.Background(), Request{Kind: SFX, Prompt: "x", Seed: 0}); err != nil {
		t.Fatal(err)
	}
	if got := argValue(t, argsFile, "--seconds"); got != "5" {
		t.Fatalf("sfx default: %q", got)
	}
	if _, err := Generate(context.Background(), Request{Kind: Music, Prompt: "x", Seed: 0}); err != nil {
		t.Fatal(err)
	}
	if got := argValue(t, argsFile, "--seconds"); got != "30" {
		t.Fatalf("music default: %q", got)
	}
	// Medium runs to 380 s; a longer request is capped and noted, not refused.
	res, err := Generate(context.Background(), Request{Kind: Music, Prompt: "x", Model: "medium", Seconds: 500, Seed: 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := argValue(t, argsFile, "--seconds"); got != "380" {
		t.Fatalf("capped: %q", got)
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "380") {
		t.Fatalf("notes: %v", res.Notes)
	}
	if got := argValue(t, argsFile, "--decoder"); got != "same-l" {
		t.Fatalf("medium decoder: %q", got)
	}
}

func TestGeneratePicksARandomSeed(t *testing.T) {
	argsFile := setupAppleSilicon(t)
	res, err := Generate(context.Background(), Request{Kind: Music, Prompt: "x", Seed: -1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Seed < 0 {
		t.Fatalf("seed: %d", res.Seed)
	}
	if got := argValue(t, argsFile, "--seed"); got != strconv.FormatInt(res.Seed, 10) {
		t.Fatalf("the script ran with seed %q, the manifest would record %d", got, res.Seed)
	}
}

func TestGeneratePassesTheInitAudio(t *testing.T) {
	argsFile := setupAppleSilicon(t)
	_, err := Generate(context.Background(), Request{Kind: Music, Prompt: "x", InitAudio: "take.wav", InitNoise: 0.6, Seed: 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := argValue(t, argsFile, "--init-audio"); got != "take.wav" {
		t.Fatalf("init audio: %q", got)
	}
	if got := argValue(t, argsFile, "--init-noise-level"); got != "0.60" {
		t.Fatalf("init noise: %q", got)
	}
}

// On a platform sa3 does not run on, the error says what is needed.
func TestGenerateNeedsAppleSilicon(t *testing.T) {
	if Available() {
		t.Skip("sa3 can run here")
	}
	_, err := Generate(context.Background(), Request{Kind: Music, Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "Apple silicon") {
		t.Fatalf("got %v", err)
	}
}
