package imagegen

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/rock3r/cav/internal/services"
)

const quiverDefaultModel = "arrow-2"

// quiver generates an SVG from a prompt. Arrow writes the markup itself, so the
// result is shapes an agent can import, not a pixel trace. Words come back as
// paths, not as <text>. Checked against the live API on 2026-10-25.
func quiver(ctx context.Context, key, model string, r Request) (*Image, error) {
	if model == "" {
		model = quiverDefaultModel
	}
	body := map[string]any{
		"model":             model,
		"n":                 1,
		"prompt":            r.Prompt,
		"temperature":       0,
		"max_output_tokens": 8000,
		"attributes":        map[string]any{"viewBox": quiverViewBox(r.Aspect)},
	}
	var refs []any
	for _, p := range r.Refs {
		data, _, err := readRef(p)
		if err != nil {
			return nil, err
		}
		refs = append(refs, map[string]any{"base64": base64.StdEncoding.EncodeToString(data)})
	}
	if len(refs) > 0 {
		body["references"] = refs
	}
	return quiverSVG(ctx, key, "https://api.quiver.ai/v1/svgs/generations", body, model)
}

// quiverVectorize asks Arrow to redraw the raster as SVG. It is not a geometric
// trace: a thin mark can come back as a different shape.
func quiverVectorize(ctx context.Context, key, model, in string) (*Image, error) {
	if model == "" {
		model = quiverDefaultModel
	}
	data, err := os.ReadFile(in)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"model":             model,
		"temperature":       0,
		"max_output_tokens": 8000,
		"image":             map[string]any{"base64": base64.StdEncoding.EncodeToString(data)},
	}
	return quiverSVG(ctx, key, "https://api.quiver.ai/v1/svgs/vectorizations", body, model)
}

func quiverSVG(ctx context.Context, key, endpoint string, body map[string]any, model string) (*Image, error) {
	var ans struct {
		Data []struct {
			SVG      string `json:"svg"`
			MIMEType string `json:"mime_type"`
		} `json:"data"`
	}
	if err := services.Do(ctx, "POST", endpoint, map[string]string{"Authorization": "Bearer " + key}, body, &ans); err != nil {
		return nil, err
	}
	if len(ans.Data) == 0 || !strings.Contains(ans.Data[0].SVG, "<svg") {
		return nil, fmt.Errorf("quiverai returned no SVG")
	}
	mime := ans.Data[0].MIMEType
	if mime == "" {
		mime = "image/svg+xml"
	}
	return &Image{Data: []byte(ans.Data[0].SVG), MIME: mime, Provider: "quiverai", Model: model}, nil
}

// quiverViewBox is a canvas in the requested ratio, so a generated logo lands at a
// size Cavalry can import 1:1.
func quiverViewBox(aspect string) map[string]any {
	w, h := 1920.0, 1080.0
	switch aspect {
	case "1:1":
		w, h = 1024, 1024
	case "9:16":
		w, h = 1080, 1920
	case "4:3":
		w, h = 1440, 1080
	case "3:4":
		w, h = 1080, 1440
	case "4:5":
		w, h = 1080, 1350
	case "21:9":
		w, h = 2520, 1080
	}
	return map[string]any{"minX": 0, "minY": 0, "width": w, "height": h}
}
