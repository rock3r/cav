package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/sa3"
)

// makeSA3Runtime fakes the pinned Stable Audio 3 checkout in the test's CAV_HOME.
func makeSA3Runtime(t *testing.T) {
	t.Helper()
	dir := filepath.Join(sa3.RuntimeDir(), "optimized", "mlx")
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "scripts", "sa3_mlx.py"), []byte("#"), 0o644)
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("mlx\n"), 0o644)
}

// makeSA3Weights fakes the downloaded sm models in the test's Hugging Face cache, so sa3
// counts as set up.
func makeSA3Weights(t *testing.T) {
	t.Helper()
	makeSA3WeightsOnly(t, "dit_sm-sfx_f16.npz", "dit_sm-music_f16.npz", "same_s_decoder_f32.npz",
		"same_s_encoder_f32.npz", "t5gemma_f16.npz")
}

// makeSA3WeightsOnly fakes a cache holding just the named files.
func makeSA3WeightsOnly(t *testing.T, files ...string) {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	snap := filepath.Join(cache, "models--stabilityai--stable-audio-3-optimized", "snapshots", "rev1", "MLX")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		os.WriteFile(filepath.Join(snap, f), []byte("w"), 0o644)
	}
}

// fakeUVSA3 puts a uv script on PATH that writes a fake WAV to sa3's --out.
func fakeUVSA3(t *testing.T) {
	t.Helper()
	fakeUVOnPath(t, `#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--out" ]; then out="$a"; fi
  prev="$a"
done
printf 'RIFFfake' > "$out"
`)
}

func setupSA3(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("sa3 runs on Apple silicon only")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	makeSA3Runtime(t)
	fakeUVSA3(t)
}

// cav sfx gen --service sa3 makes the sound locally and records the take with the
// Stability Community License and the seed, so the take can be made again.
func TestSFXGenSA3RecordsTheTake(t *testing.T) {
	setupSA3(t)
	a := &app{json: true}
	var err error
	out := captureStdout(t, func() {
		err = cmdSources(a, "sfx", "audio", "sfx", []string{"gen", "metal clank", "--service", "sa3", "--seconds", "2", "--seed", "7"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, e := os.Stat("sfx/metal-clank.wav"); e != nil {
		t.Fatalf("the take is not at sfx/metal-clank.wav: %v", e)
	}
	var data map[string]any
	if e := json.Unmarshal([]byte(out), &data); e != nil {
		t.Fatalf("not JSON: %v\n%s", e, out)
	}
	asset := data["asset"].(map[string]any)
	if asset["licence"] != "Stability-Community" || asset["commercialOk"] != true {
		t.Fatalf("licence: %v %v", asset["licence"], asset["commercialOk"])
	}
	gen := asset["generated"].(map[string]any)
	if gen["provider"] != "sa3" || gen["model"] != "sm-sfx" || gen["seed"] != 7.0 {
		t.Fatalf("generated: %v", gen)
	}
}

// --loop is an ElevenLabs feature; sa3 cannot loop, so the flags together are refused
// before any model runs.
func TestSFXGenSA3RefusesLoop(t *testing.T) {
	setupSA3(t)
	err := cmdSources(&app{}, "sfx", "audio", "sfx", []string{"gen", "hum", "--service", "sa3", "--loop"})
	if err == nil || !strings.Contains(err.Error(), "--loop is an ElevenLabs feature") {
		t.Fatalf("got %v", err)
	}
}

// With no ElevenLabs key but the sa3 model already downloaded, a plain cav sfx gen uses
// sa3 instead of failing.
func TestSFXGenFallsBackToSA3WithoutAKey(t *testing.T) {
	setupSA3(t)
	makeSA3Weights(t)
	t.Setenv("CAV_NO_KEYS", "1")
	a := &app{json: true}
	var err error
	out := captureStdout(t, func() {
		err = cmdSources(a, "sfx", "audio", "sfx", []string{"gen", "short beep"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if e := json.Unmarshal([]byte(out), &data); e != nil {
		t.Fatalf("not JSON: %v\n%s", e, out)
	}
	gen := data["asset"].(map[string]any)["generated"].(map[string]any)
	if gen["provider"] != "sa3" {
		t.Fatalf("provider: %v", gen["provider"])
	}
}

// With no key and no sa3 setup, the error names both ways forward. This runs on every
// platform: the missing key stops the command first.
func TestSFXGenWithoutAKeyPointsAtSA3(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("HF_HUB_CACHE", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	t.Chdir(t.TempDir())
	err := cmdSources(&app{}, "sfx", "audio", "sfx", []string{"gen", "beep"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.hint, "cav sfx gen --service sa3") {
		t.Fatalf("got %v", err)
	}
}

// The keyless fallback runs sm-sfx, so a cached music model alone must not trigger it:
// it would start a 1.6 GB download the user did not ask for.
func TestSFXGenDoesNotFallBackWhenOnlyTheMusicModelIsCached(t *testing.T) {
	setupSA3(t)
	makeSA3WeightsOnly(t, "dit_sm-music_f16.npz", "same_s_decoder_f32.npz", "t5gemma_f16.npz")
	t.Setenv("CAV_NO_KEYS", "1")
	err := cmdSources(&app{}, "sfx", "audio", "sfx", []string{"gen", "beep"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.hint, "cav sfx gen --service sa3") {
		t.Fatalf("got %v", err)
	}
}

// A --model names the model sa3 must be ready with: with only the small model cached,
// --model medium cannot join the order (it would otherwise start a 4.9 GB download).
func TestMusicGenModelMustBeCachedForSA3(t *testing.T) {
	setupSA3(t)
	makeSA3Weights(t)
	t.Setenv("CAV_NO_KEYS", "1")
	err := cmdMusic(&app{}, []string{"gen", "chill beat", "--seconds", "5", "--model", "medium"})
	if err == nil || !strings.Contains(err.Error(), "sa3 medium") {
		t.Fatalf("got %v", err)
	}
}

// With no music keys at all, a downloaded sa3 model joins the order: cav music gen works
// with no account, and the take is a local WAV.
func TestMusicGenPicksSA3WithoutAKey(t *testing.T) {
	setupSA3(t)
	makeSA3Weights(t)
	t.Setenv("CAV_NO_KEYS", "1")
	a := &app{json: true}
	var err error
	out := captureStdout(t, func() {
		err = cmdMusic(a, []string{"gen", "chill beat", "--seconds", "5"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if e := json.Unmarshal([]byte(out), &data); e != nil {
		t.Fatalf("not JSON: %v\n%s", e, out)
	}
	if data["service"] != "sa3" {
		t.Fatalf("service: %v", data["service"])
	}
	takes := data["takes"].([]any)
	if len(takes) != 1 || !strings.HasSuffix(takes[0].(string), ".wav") {
		t.Fatalf("takes: %v", takes)
	}
	if _, e := os.Stat(takes[0].(string)); e != nil {
		t.Fatalf("the take is not there: %v", e)
	}
}
