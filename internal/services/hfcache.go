package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MfluxDefaultModel is the model mflux runs when the config names none.
const MfluxDefaultModel = "flux2-klein-4b"

// mfluxRepos maps the mflux models cav knows to their Hugging Face repository and the size
// of their first download. Read in mflux 0.22.0 (models/common/config/model_config.py).
var mfluxRepos = map[string]struct{ Repo, Size string }{
	"flux2-klein-4b": {"black-forest-labs/FLUX.2-klein-4B", "about 16 GB"},
	"flux2-klein-9b": {"black-forest-labs/FLUX.2-klein-9B", "more than 16 GB"},
}

// HFHubCache returns the Hugging Face hub cache folder, found the way huggingface_hub
// finds it: HF_HUB_CACHE, HUGGINGFACE_HUB_CACHE, HF_HOME/hub, XDG_CACHE_HOME/huggingface/hub,
// then ~/.cache/huggingface/hub.
func HFHubCache() string {
	for _, v := range []string{"HF_HUB_CACHE", "HUGGINGFACE_HUB_CACHE"} {
		if p := os.Getenv(v); p != "" {
			return expandHome(p)
		}
	}
	if p := os.Getenv("HF_HOME"); p != "" {
		return filepath.Join(expandHome(p), "hub")
	}
	if p := os.Getenv("XDG_CACHE_HOME"); p != "" {
		return filepath.Join(expandHome(p), "huggingface", "hub")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "huggingface", "hub")
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}

// MfluxWeights says whether the weights of an mflux model are in the Hugging Face cache, and
// how large the first download is. known is false for a model cav has no repository for.
func MfluxWeights(model string) (cached, known bool, size string) {
	m, ok := mfluxRepos[model]
	if !ok {
		return false, false, "several GB"
	}
	snaps := filepath.Join(HFHubCache(), "models--"+strings.ReplaceAll(m.Repo, "/", "--"), "snapshots")
	entries, err := os.ReadDir(snaps)
	if err != nil {
		return false, true, m.Size
	}
	for _, e := range entries {
		if e.IsDir() && fluxSnapshotComplete(filepath.Join(snaps, e.Name())) {
			return true, true, m.Size
		}
	}
	return false, true, m.Size
}

// fluxSnapshotComplete checks a snapshot the way mflux does before it skips the download:
// the transformer, text encoder and VAE each hold a weight file that is really on disk
// (a link to a finished blob), and every shard a weight index names is there.
func fluxSnapshotComplete(dir string) bool {
	for _, sub := range []string{"transformer", "text_encoder", "vae"} {
		files, _ := filepath.Glob(filepath.Join(dir, sub, "*.safetensors"))
		found := false
		for _, f := range files {
			if _, err := os.Stat(f); err == nil { // follows the link to the blob
				found = true
				break
			}
		}
		if !found {
			return false
		}
		idx, err := os.ReadFile(filepath.Join(dir, sub, "model.safetensors.index.json"))
		if err != nil {
			continue
		}
		var index struct {
			WeightMap map[string]string `json:"weight_map"`
		}
		if json.Unmarshal(idx, &index) != nil {
			return false
		}
		for _, shard := range index.WeightMap {
			if _, err := os.Stat(filepath.Join(dir, sub, shard)); err != nil {
				return false
			}
		}
	}
	return true
}

// mfluxReady reports whether mflux can run without its large first download.
func mfluxReady(c *Config) error {
	model := MfluxDefaultModel
	if c != nil && c.Models["mflux"] != "" {
		model = c.Models["mflux"]
	}
	cached, known, size := MfluxWeights(model)
	if cached || !known {
		return nil
	}
	return fmt.Errorf("the %s weights are not downloaded yet (%s): run cav gen image --service mflux once", model, size)
}

// SA3DefaultModel is the Stable Audio 3 model cav runs when the config names none.
const SA3DefaultModel = "sm-music"

// sa3Files maps the SA3 models cav knows to the weight files a run needs in the
// stabilityai/stable-audio-3-optimized repository (its weights manifest, read on
// 2026-10-18): the model's DiT and decoder, plus the shared T5Gemma text encoder. A
// --ref run (init audio) also needs the codec's encoder, which the runtime downloads on
// demand; SA3RefEncoder tracks it. The size is the first text-to-audio download,
// T5Gemma included; the encoder's size is separate.
var sa3Files = map[string]struct {
	DiT         string
	Decoder     string
	Encoder     string
	Size        string
	EncoderSize string
}{
	"sm-sfx":   {DiT: "MLX/dit_sm-sfx_f16.npz", Decoder: "MLX/same_s_decoder_f32.npz", Encoder: "MLX/same_s_encoder_f32.npz", Size: "about 1.6 GB", EncoderSize: "about 205 MB"},
	"sm-music": {DiT: "MLX/dit_sm-music_f16.npz", Decoder: "MLX/same_s_decoder_f32.npz", Encoder: "MLX/same_s_encoder_f32.npz", Size: "about 1.6 GB", EncoderSize: "about 205 MB"},
	"medium":   {DiT: "MLX/dit_medium_f16.npz", Decoder: "MLX/same_l_decoder_f32.npz", Encoder: "MLX/same_l_encoder_f32.npz", Size: "about 4.9 GB", EncoderSize: "about 1.6 GB"},
}

// SA3Weights says whether the weights of a Stable Audio 3 model are in the Hugging Face
// cache, and how large the first download is. known is false for a model cav has no
// repository for.
func SA3Weights(model string) (cached, known bool, size string) {
	m, ok := sa3Files[model]
	if !ok {
		return false, false, "several GB"
	}
	return sa3SnapshotHas(m.DiT, m.Decoder, "MLX/t5gemma_f16.npz"), true, m.Size
}

// SA3RefEncoder says whether the codec encoder a --ref (init audio) run of a Stable
// Audio 3 model needs is in the cache, and how large its one-time download is. The
// runtime fetches it on demand, so a text-to-audio first run leaves it missing.
func SA3RefEncoder(model string) (cached bool, size string) {
	m, ok := sa3Files[model]
	if !ok {
		return false, "several hundred MB"
	}
	return sa3SnapshotHas(m.Encoder), m.EncoderSize
}

// sa3SnapshotHas reports whether one snapshot of the stable-audio-3-optimized
// repository holds all the named files. os.Stat follows the snapshot's links to the
// blobs, so a half-downloaded file (a link whose blob is missing) does not count.
func sa3SnapshotHas(files ...string) bool {
	snaps := filepath.Join(HFHubCache(), "models--stabilityai--stable-audio-3-optimized", "snapshots")
	entries, err := os.ReadDir(snaps)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		complete := true
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(snaps, e.Name(), f)); err != nil {
				complete = false
				break
			}
		}
		if complete {
			return true
		}
	}
	return false
}

// sa3Ready reports whether sa3 can run without its large first download.
func sa3Ready(c *Config) error {
	model := SA3DefaultModel
	if c != nil && c.Models["sa3"] != "" {
		model = c.Models["sa3"]
	}
	cached, known, size := SA3Weights(model)
	if cached || !known {
		return nil
	}
	return fmt.Errorf("the sa3 %s weights are not downloaded yet (%s): run cav music gen --service sa3 once", model, size)
}
