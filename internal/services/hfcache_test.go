package services

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeFluxCache writes a FLUX.2 klein 4B snapshot laid out like huggingface_hub's cache:
// snapshot files are links to blobs. Leaving out a blob makes the link dangle.
func fakeFluxCache(t *testing.T, cache string, missing string) {
	t.Helper()
	repo := filepath.Join(cache, "models--black-forest-labs--FLUX.2-klein-4B")
	snap := filepath.Join(repo, "snapshots", "e7b7dc27")
	files := []string{
		"transformer/diffusion_pytorch_model.safetensors",
		"text_encoder/model-00001-of-00002.safetensors",
		"text_encoder/model-00002-of-00002.safetensors",
		"vae/diffusion_pytorch_model.safetensors",
	}
	for i, f := range files {
		blob := filepath.Join(repo, "blobs", string(rune('a'+i)))
		os.MkdirAll(filepath.Dir(blob), 0o755)
		if f != missing {
			os.WriteFile(blob, []byte("w"), 0o644)
		}
		os.MkdirAll(filepath.Join(snap, filepath.Dir(f)), 0o755)
		if err := os.Symlink(blob, filepath.Join(snap, f)); err != nil && f != missing {
			// Windows without the symlink privilege: huggingface_hub copies the file then.
			os.WriteFile(filepath.Join(snap, f), []byte("w"), 0o644)
		}
	}
	os.WriteFile(filepath.Join(snap, "text_encoder", "model.safetensors.index.json"),
		[]byte(`{"weight_map":{"a":"model-00001-of-00002.safetensors","b":"model-00002-of-00002.safetensors"}}`), 0o644)
}

func TestMfluxWeightsInTheCache(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	if cached, known, _ := MfluxWeights("flux2-klein-4b"); cached || !known {
		t.Fatalf("empty cache: cached %v known %v", cached, known)
	}
	fakeFluxCache(t, cache, "text_encoder/model-00002-of-00002.safetensors")
	if cached, _, _ := MfluxWeights("flux2-klein-4b"); cached {
		t.Fatal("a missing text encoder shard counted as cached")
	}
	os.RemoveAll(cache)
	fakeFluxCache(t, cache, "")
	if cached, _, size := MfluxWeights("flux2-klein-4b"); !cached || size != "about 16 GB" {
		t.Fatalf("complete snapshot: cached %v size %s", cached, size)
	}
	if _, known, _ := MfluxWeights("z-image-turbo"); known {
		t.Fatal("cav has no repository for z-image-turbo")
	}
}

func TestHFHubCacheFollowsHuggingFaceVariables(t *testing.T) {
	for _, v := range []string{"HF_HUB_CACHE", "HUGGINGFACE_HUB_CACHE", "HF_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(v, "")
	}
	home, _ := os.UserHomeDir()
	if got := HFHubCache(); got != filepath.Join(home, ".cache", "huggingface", "hub") {
		t.Fatalf("default: %s", got)
	}
	t.Setenv("XDG_CACHE_HOME", "/x")
	if got := HFHubCache(); got != filepath.Join("/x", "huggingface", "hub") {
		t.Fatalf("XDG_CACHE_HOME: %s", got)
	}
	t.Setenv("HF_HOME", "/h")
	if got := HFHubCache(); got != filepath.Join("/h", "hub") {
		t.Fatalf("HF_HOME: %s", got)
	}
	t.Setenv("HF_HUB_CACHE", "/c")
	if got := HFHubCache(); got != "/c" {
		t.Fatalf("HF_HUB_CACHE: %s", got)
	}
}

func TestPickUsesMfluxOnlyWhenItsModelIsCached(t *testing.T) {
	if runtime.GOOS+"/"+runtime.GOARCH != "darwin/arm64" {
		t.Skip("mflux runs only on Apple silicon")
	}
	oldLook := lookPath
	defer func() { lookPath = oldLook }()
	lookPath = func(string) (string, error) { return "/usr/local/bin/uv", nil }
	t.Setenv("CAV_NO_KEYS", "1")
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	c := &Config{}
	ch, err := Pick(context.Background(), c, "image", "")
	if err != nil || ch.Service != "sketch" {
		t.Fatalf("no weights: got %+v %v", ch, err)
	}
	if !strings.Contains(strings.Join(ch.Skipped, "\n"), "mflux: the flux2-klein-4b weights are not downloaded yet (about 16 GB)") {
		t.Fatalf("skip reason missing: %v", ch.Skipped)
	}
	if ch, err := Pick(context.Background(), c, "image", "mflux"); err != nil || ch.Service != "mflux" {
		t.Fatalf("--service mflux must run without the weights: %+v %v", ch, err)
	}
	fakeFluxCache(t, cache, "")
	if ch, err := Pick(context.Background(), c, "image", ""); err != nil || ch.Service != "mflux" {
		t.Fatalf("cached weights: got %+v %v", ch, err)
	}
	if ch, _ := Pick(context.Background(), c.WithModel("mflux", "flux2-klein-9b"), "image", ""); ch.Service != "sketch" {
		t.Fatalf("a --model that is not cached: got %s", ch.Service)
	}
	if c.Models["mflux"] != "" {
		t.Fatal("WithModel changed the config it copied")
	}
	c.Models = map[string]string{"mflux": "flux2-klein-9b"}
	if ch, _ := Pick(context.Background(), c, "image", ""); ch.Service != "sketch" {
		t.Fatalf("the configured model is not cached: got %s", ch.Service)
	}
	if ch, _ := Pick(context.Background(), c.WithModel("mflux", "flux2-klein-4b"), "image", ""); ch.Service != "mflux" {
		t.Fatalf("a cached --model over an uncached configured one: got %s", ch.Service)
	}
}

// fakeSA3Cache writes a stable-audio-3-optimized snapshot laid out like
// huggingface_hub's cache; missing names one file whose blob is left out.
func fakeSA3Cache(t *testing.T, cache, missing string) {
	t.Helper()
	repo := filepath.Join(cache, "models--stabilityai--stable-audio-3-optimized")
	snap := filepath.Join(repo, "snapshots", "77e0398c")
	files := []string{
		"MLX/dit_sm-sfx_f16.npz",
		"MLX/dit_sm-music_f16.npz",
		"MLX/dit_medium_f16.npz",
		"MLX/same_s_decoder_f32.npz",
		"MLX/same_s_encoder_f32.npz",
		"MLX/same_l_decoder_f32.npz",
		"MLX/same_l_encoder_f32.npz",
		"MLX/t5gemma_f16.npz",
	}
	for i, f := range files {
		blob := filepath.Join(repo, "blobs", string(rune('a'+i)))
		os.MkdirAll(filepath.Dir(blob), 0o755)
		if f != missing {
			os.WriteFile(blob, []byte("w"), 0o644)
		}
		os.MkdirAll(filepath.Join(snap, filepath.Dir(f)), 0o755)
		if err := os.Symlink(blob, filepath.Join(snap, f)); err != nil && f != missing {
			// Windows without the symlink privilege: huggingface_hub copies the file then.
			os.WriteFile(filepath.Join(snap, f), []byte("w"), 0o644)
		}
	}
}

func TestSA3WeightsInTheCache(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	if cached, known, _ := SA3Weights("sm-music"); cached || !known {
		t.Fatalf("empty cache: cached %v known %v", cached, known)
	}
	fakeSA3Cache(t, cache, "MLX/t5gemma_f16.npz")
	if cached, _, _ := SA3Weights("sm-music"); cached {
		t.Fatal("a missing shared text encoder counted as cached")
	}
	os.RemoveAll(cache)
	fakeSA3Cache(t, cache, "")
	if cached, _, size := SA3Weights("sm-music"); !cached || size != "about 1.6 GB" {
		t.Fatalf("complete snapshot: cached %v size %s", cached, size)
	}
	if cached, _, _ := SA3Weights("medium"); !cached {
		t.Fatal("medium needs same_l, which is in the snapshot")
	}
	if _, known, _ := SA3Weights("huge"); known {
		t.Fatal("cav has no repository for huge")
	}
}

// The codec encoder a --ref run needs is tracked on its own: a text-to-audio first run
// leaves it missing while the base weights count as cached.
func TestSA3RefEncoderIsTrackedSeparately(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	fakeSA3Cache(t, cache, "MLX/same_s_encoder_f32.npz")
	if cached, _, _ := SA3Weights("sm-music"); !cached {
		t.Fatal("the base weights are complete without the encoder")
	}
	if cached, size := SA3RefEncoder("sm-music"); cached || size != "about 205 MB" {
		t.Fatalf("missing encoder: cached %v size %s", cached, size)
	}
	os.RemoveAll(cache)
	fakeSA3Cache(t, cache, "")
	if cached, _ := SA3RefEncoder("sm-music"); !cached {
		t.Fatal("complete snapshot: encoder not found")
	}
	if cached, size := SA3RefEncoder("medium"); !cached || size != "about 1.6 GB" {
		t.Fatalf("medium encoder: cached %v size %s", cached, size)
	}
}

func TestPickUsesSA3OnlyWhenItsModelIsCached(t *testing.T) {
	if runtime.GOOS+"/"+runtime.GOARCH != "darwin/arm64" {
		t.Skip("sa3 runs only on Apple silicon")
	}
	oldLook := lookPath
	defer func() { lookPath = oldLook }()
	lookPath = func(string) (string, error) { return "/usr/local/bin/uv", nil }
	t.Setenv("CAV_NO_KEYS", "1")
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	c := &Config{}
	// With no keys, no servers and no weights, no music service works.
	if _, err := Pick(context.Background(), c, "music", ""); err == nil {
		t.Fatal("no weights: sa3 should not be picked")
	} else if !strings.Contains(err.Error(), "sa3: the sa3 sm-music weights are not downloaded yet (about 1.6 GB)") {
		t.Fatalf("skip reason missing: %v", err)
	}
	// An explicit --service sa3 runs anyway: the first run is the download.
	if ch, err := Pick(context.Background(), c, "music", "sa3"); err != nil || ch.Service != "sa3" {
		t.Fatalf("--service sa3 must run without the weights: %+v %v", ch, err)
	}
	fakeSA3Cache(t, cache, "")
	if ch, err := Pick(context.Background(), c, "music", ""); err != nil || ch.Service != "sa3" {
		t.Fatalf("cached weights: got %+v %v", ch, err)
	}
	// A configured model that is not cached keeps sa3 out of the order.
	c.Models = map[string]string{"sa3": "medium"}
	os.RemoveAll(filepath.Join(cache, "models--stabilityai--stable-audio-3-optimized", "blobs"))
	fakeSA3Cache(t, cache, "MLX/same_l_decoder_f32.npz")
	if _, err := Pick(context.Background(), c, "music", ""); err == nil || !strings.Contains(err.Error(), "sa3 medium") {
		t.Fatalf("the configured model is not cached: %v", err)
	}
	// A command's --model does the same through WithModel: the requested model is the
	// one that must be cached.
	if _, err := Pick(context.Background(), (&Config{}).WithModel("sa3", "medium"), "music", ""); err == nil || !strings.Contains(err.Error(), "sa3 medium") {
		t.Fatalf("--model medium is not cached: %v", err)
	}
	if ch, err := Pick(context.Background(), (&Config{}).WithModel("sa3", "sm-music"), "music", ""); err != nil || ch.Service != "sa3" {
		t.Fatalf("--model sm-music is cached: %+v %v", ch, err)
	}
}
