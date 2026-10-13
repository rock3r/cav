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
