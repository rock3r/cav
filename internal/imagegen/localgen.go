package imagegen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/services"
)

// localSize maps an aspect ratio to a size local models handle well (multiples of 16).
func localSize(aspect string) (int, int) {
	switch aspect {
	case "1:1":
		return 1024, 1024
	case "9:16":
		return 576, 1024
	case "4:3":
		return 1024, 768
	case "3:4":
		return 768, 1024
	case "21:9":
		return 1344, 576
	}
	return 1024, 576
}

// MfluxAvailable reports whether mflux can run here: Apple silicon and uv.
func MfluxAvailable() bool {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return false
	}
	_, err := exec.LookPath("uv")
	return err == nil
}

// mflux runs a FLUX-family model on Apple silicon through mflux (MLX). The first run of a
// model downloads its weights from Hugging Face into its own cache.
func mflux(ctx context.Context, model string, r Request) (*Image, error) {
	if !MfluxAvailable() {
		return nil, fmt.Errorf("mflux needs a Mac with Apple silicon and uv")
	}
	tmp, err := os.MkdirTemp("", "cav-mflux")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "out.png")
	w, h := localSize(r.Aspect)
	args := []string{"--python", "3.12", "--from", "mflux", mfluxCommand(model), "--model", model,
		"--prompt", r.Prompt, "--width", strconv.Itoa(w), "--height", strconv.Itoa(h), "--output", out}
	if steps := mfluxSteps[model]; steps > 0 {
		args = append(args, "--steps", strconv.Itoa(steps))
	}
	if len(r.Refs) > 0 {
		// mflux takes one starting image; the first reference sets the composition.
		args = append(args, "--image-path", r.Refs[0], "--image-strength", "0.35")
	}
	cmd := exec.CommandContext(ctx, "uvx", args...)
	cmd.Stderr = os.Stderr // model downloads and steps report progress here
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s stopped: %w (the first run downloads the model; run it again to resume)", mfluxCommand(model), ctx.Err())
		}
		return nil, fmt.Errorf("%s: %w", mfluxCommand(model), err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return nil, fmt.Errorf("mflux wrote no image: %w", err)
	}
	return &Image{Data: data, MIME: "image/png"}, nil
}

// mfluxCommand picks the mflux entry point for a model: each family has its own.
func mfluxCommand(model string) string {
	switch {
	case strings.HasPrefix(model, "flux2-"):
		return "mflux-generate-flux2"
	case model == "z-image-turbo":
		return "mflux-generate-z-image-turbo"
	case model == "z-image":
		return "mflux-generate-z-image"
	case model == "qwen-image-2.1":
		return "mflux-generate-qwen-2.1"
	case strings.HasPrefix(model, "qwen-image"):
		return "mflux-generate-qwen"
	case strings.HasPrefix(model, "krea-2"):
		return "mflux-generate-krea2"
	case model == "ernie-image-turbo":
		return "mflux-generate-ernie-image-turbo"
	case model == "ernie-image":
		return "mflux-generate-ernie-image"
	case strings.HasPrefix(model, "ideogram-4"):
		return "mflux-generate-ideogram4"
	}
	return "mflux-generate"
}

// Distilled models need few steps; others use mflux's default.
var mfluxSteps = map[string]int{"flux2-klein-4b": 4, "flux2-klein-9b": 4, "schnell": 4, "z-image-turbo": 8}

// comfyui queues a workflow on a ComfyUI server and fetches its first output image. The
// workflow is an API-format JSON file whose strings may hold {{prompt}}, {{width}},
// {{height}} and {{seed}}.
func comfyui(ctx context.Context, ep services.Endpoint, r Request) (*Image, error) {
	if ep.Model == "" {
		return nil, fmt.Errorf("set the workflow: cav config endpoint comfyui <base-url> <workflow-api.json>")
	}
	raw, err := os.ReadFile(ep.Model)
	if err != nil {
		return nil, fmt.Errorf("reading the ComfyUI workflow: %w", err)
	}
	w, h := localSize(r.Aspect)
	prompt, _ := json.Marshal(r.Prompt)
	text := strings.NewReplacer(
		`"{{width}}"`, strconv.Itoa(w), `"{{height}}"`, strconv.Itoa(h),
		`"{{seed}}"`, strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 10),
		`{{prompt}}`, strings.Trim(string(prompt), `"`),
	).Replace(string(raw))
	var workflow any
	if err := json.Unmarshal([]byte(text), &workflow); err != nil {
		return nil, fmt.Errorf("the ComfyUI workflow is not valid JSON after filling it in: %w", err)
	}
	base := strings.TrimRight(ep.BaseURL, "/")
	var queued struct {
		ID string `json:"prompt_id"`
	}
	if err := services.Do(ctx, "POST", base+"/prompt", nil, map[string]any{"prompt": workflow, "client_id": "cav"}, &queued); err != nil {
		return nil, fmt.Errorf("comfyui: %w", err)
	}
	// ComfyUI runs the job on its own queue; ask for its history until the outputs appear.
	for {
		var hist map[string]struct {
			Outputs map[string]struct {
				Images []struct {
					Filename  string `json:"filename"`
					Subfolder string `json:"subfolder"`
					Type      string `json:"type"`
				} `json:"images"`
			} `json:"outputs"`
		}
		if err := services.Do(ctx, "GET", base+"/history/"+queued.ID, nil, nil, &hist); err != nil {
			return nil, fmt.Errorf("comfyui: %w", err)
		}
		if job, ok := hist[queued.ID]; ok {
			for _, out := range job.Outputs {
				for _, im := range out.Images {
					q := url.Values{"filename": {im.Filename}, "subfolder": {im.Subfolder}, "type": {im.Type}}
					data, ctype, err := services.DoRaw(ctx, "GET", base+"/view?"+q.Encode(), nil, nil)
					if err != nil {
						return nil, fmt.Errorf("comfyui: %w", err)
					}
					if ctype == "" {
						ctype = sniff(data)
					}
					return &Image{Data: data, MIME: ctype}, nil
				}
			}
			return nil, fmt.Errorf("comfyui finished the workflow without an image output")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
