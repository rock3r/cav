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
	if err != nil || ch.Service != "greybox" {
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
	c.Models = map[string]string{"mflux": "flux2-klein-9b"}
	if ch, _ := Pick(context.Background(), c, "image", ""); ch.Service != "greybox" {
		t.Fatalf("the configured model is not cached: got %s", ch.Service)
	}
}
