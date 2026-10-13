package imagegen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/pyrun"
	"github.com/rock3r/cav/internal/services"
)

// RembgPackage is the rembg release cav runs, with the CPU build of ONNX Runtime. It is
// pinned so that an update upstream cannot change the cutouts without a cav release.
const RembgPackage = "rembg[cpu]==2.0.85"

// RembgModel is BiRefNet's general model. rembg downloads it (about 1 GB) on its first use
// into ~/.rembg/models (or $REMBG_HOME/models).
const RembgModel = "birefnet-general"

// CutoutServices are the image.alpha services that can cut the background out of an
// image that already exists. OpenAI makes transparent images only while it generates them.
var CutoutServices = []string{"recraft", "rembg"}

// RemoveBackground makes the background of the image at in transparent with the chosen
// service: recraft (API) or rembg (local, through uv).
func RemoveBackground(ctx context.Context, ch *services.Choice, in string) (*Image, error) {
	switch ch.Service {
	case "recraft":
		data, err := os.ReadFile(in)
		if err != nil {
			return nil, err
		}
		return RecraftEdit(ctx, ch.Key, "removeBackground", data)
	case "rembg":
		return rembg(ctx, in)
	}
	return nil, fmt.Errorf("%s does not cut out images", ch.Service)
}

// rembg runs remove_background.py through uv. Model downloads and errors stream to stderr.
func rembg(ctx context.Context, in string) (*Image, error) {
	script, err := assets.Python.ReadFile("python/remove_background.py")
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(in)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "cav-cutout")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "out.png")
	fmt.Fprintln(os.Stderr, "cutting out the background with rembg (the first run downloads the model, about 1 GB)…")
	if _, err := pyrun.Run(ctx, filepath.Join(config.CacheDir(), "python"), "remove_background.py", script,
		[]string{RembgPackage}, []string{RembgModel, abs, out}, os.Stderr); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("rembg stopped: %w (the first run downloads the model; run it again to resume)", ctx.Err())
		}
		return nil, fmt.Errorf("rembg: %w", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return nil, fmt.Errorf("rembg wrote no image: %w", err)
	}
	return &Image{Data: data, MIME: "image/png", Provider: "rembg", Model: RembgModel}, nil
}
