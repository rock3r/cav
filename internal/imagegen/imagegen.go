// Package imagegen makes stills (storyboard frames, assets with transparency, SVG) with
// whichever service the config picks: Gemini, OpenAI, OpenRouter, Recraft, or local tools.
//
// Request shapes follow each vendor's documentation as read on 2026-10-08. They are tested
// against recorded shapes, not against the live services (no keys were available).
package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rock3r/cav/internal/services"
)

// Request is one image to make.
type Request struct {
	Prompt string
	Refs   []string // reference images (paths), for a consistent look
	Aspect string   // 16:9, 1:1, 9:16, 4:3, ...
	Alpha  bool     // transparent background
	Vector bool     // SVG
	Model  string   // overrides the configured or default model
}

// Image is the result.
type Image struct {
	Data     []byte
	MIME     string // image/png, image/svg+xml, ...
	Provider string
	Model    string
}

// Ext returns the file extension for the image.
func (im *Image) Ext() string {
	switch {
	case strings.Contains(im.MIME, "svg"):
		return ".svg"
	case strings.Contains(im.MIME, "jpeg"):
		return ".jpg"
	case strings.Contains(im.MIME, "webp"):
		return ".webp"
	}
	return ".png"
}

// Default models, used when the config names none (cav config model <service> <id>).
var DefaultModels = map[string]string{
	"gemini":     "gemini-3.1-flash-image",
	"openai":     "gpt-image-2.5-flare",
	"openrouter": "google/gemini-2.5-flash-image",
	"recraft":    "recraftv4_1",
	"mflux":      "flux2-klein-4b",
}

func model(c *services.Config, service string, r Request) string {
	if r.Model != "" {
		return r.Model
	}
	if m := c.Models[service]; m != "" {
		return m
	}
	return DefaultModels[service]
}

// Generate makes the image with the chosen service.
func Generate(ctx context.Context, c *services.Config, ch *services.Choice, r Request) (*Image, error) {
	if r.Aspect == "" {
		r.Aspect = "16:9"
	}
	m := model(c, ch.Service, r)
	var im *Image
	var err error
	switch ch.Service {
	case "gemini":
		im, err = gemini(ctx, ch.Key, m, r)
	case "openai":
		im, err = openai(ctx, ch.Key, m, r)
	case "openrouter":
		im, err = openrouter(ctx, ch.Key, m, r)
	case "recraft":
		if r.Vector && !strings.HasSuffix(m, "_vector") {
			m = "recraftv3_vector"
		}
		im, err = recraft(ctx, ch.Key, m, r)
	case "mflux":
		im, err = mflux(ctx, m, r)
	case "comfyui":
		m = filepath.Base(c.Endpoints["comfyui"].Model)
		im, err = comfyui(ctx, c.Endpoints["comfyui"], r)
	default:
		return nil, fmt.Errorf("%s does not generate images", ch.Service)
	}
	if err != nil {
		return nil, fmt.Errorf("%s (%s): %w", ch.Service, m, err)
	}
	im.Provider, im.Model = ch.Service, m
	return im, nil
}

func readRef(path string) (data []byte, mime string, err error) {
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	mime = http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", fmt.Errorf("%s is not an image (%s)", path, mime)
	}
	return data, mime, nil
}

// ---------- Gemini (generateContent with image output) ----------

func gemini(ctx context.Context, key, model string, r Request) (*Image, error) {
	parts := []map[string]any{{"text": r.Prompt}}
	for _, p := range r.Refs {
		data, mime, err := readRef(p)
		if err != nil {
			return nil, err
		}
		parts = append(parts, map[string]any{"inline_data": map[string]any{"mime_type": mime, "data": base64.StdEncoding.EncodeToString(data)}})
	}
	body := map[string]any{
		"contents": []any{map[string]any{"parts": parts}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"IMAGE"},
			"imageConfig":        map[string]any{"aspectRatio": r.Aspect},
		},
	}
	raw, _, err := services.DoRaw(ctx, "POST", "https://generativelanguage.googleapis.com/v1beta/models/"+model+":generateContent",
		map[string]string{"x-goog-api-key": key}, body)
	if err != nil {
		return nil, err
	}
	return findImage(ctx, raw)
}

// ---------- OpenAI Image API ----------

// openaiSize maps an aspect ratio to a size the gpt-image models accept (multiples of 16).
func openaiSize(aspect string) string {
	switch aspect {
	case "1:1":
		return "1024x1024"
	case "9:16":
		return "864x1536"
	case "4:3":
		return "1280x960"
	case "3:4":
		return "960x1280"
	case "3:2":
		return "1536x1024"
	case "2:3":
		return "1024x1536"
	case "21:9":
		return "1792x768"
	}
	return "1536x864"
}

func openai(ctx context.Context, key, model string, r Request) (*Image, error) {
	h := map[string]string{"Authorization": "Bearer " + key}
	if len(r.Refs) == 0 {
		body := map[string]any{"model": model, "prompt": r.Prompt, "size": openaiSize(r.Aspect), "n": 1, "output_format": "png"}
		if r.Alpha {
			body["background"] = "transparent"
		}
		raw, _, err := services.DoRaw(ctx, "POST", "https://api.openai.com/v1/images/generations", h, body)
		if err != nil {
			return nil, err
		}
		return findImage(ctx, raw)
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := map[string]string{"model": model, "prompt": r.Prompt, "size": openaiSize(r.Aspect), "n": "1", "output_format": "png"}
	if r.Alpha {
		fields["background"] = "transparent"
	}
	for k, v := range fields {
		w.WriteField(k, v)
	}
	for _, p := range r.Refs {
		data, _, err := readRef(p)
		if err != nil {
			return nil, err
		}
		fw, _ := w.CreateFormFile("image[]", filepath.Base(p))
		fw.Write(data)
	}
	w.Close()
	h["Content-Type"] = w.FormDataContentType()
	raw, _, err := services.DoRaw(ctx, "POST", "https://api.openai.com/v1/images/edits", h, buf.Bytes())
	if err != nil {
		return nil, err
	}
	return findImage(ctx, raw)
}

// ---------- OpenRouter Image API ----------

func openrouter(ctx context.Context, key, model string, r Request) (*Image, error) {
	body := map[string]any{"model": model, "prompt": r.Prompt, "n": 1, "aspect_ratio": r.Aspect, "output_format": "png"}
	var refs []any
	for _, p := range r.Refs {
		data, mime, err := readRef(p)
		if err != nil {
			return nil, err
		}
		refs = append(refs, map[string]any{"type": "image_url", "image_url": map[string]any{
			"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}})
	}
	if len(refs) > 0 {
		body["input_references"] = refs
	}
	raw, _, err := services.DoRaw(ctx, "POST", "https://openrouter.ai/api/v1/images", map[string]string{"Authorization": "Bearer " + key}, body)
	if err != nil {
		return nil, err
	}
	return findImage(ctx, raw)
}

// ---------- Recraft ----------

func recraft(ctx context.Context, key, model string, r Request) (*Image, error) {
	h := map[string]string{"Authorization": "Bearer " + key}
	body := map[string]any{"prompt": r.Prompt, "model": model, "size": r.Aspect, "n": 1, "response_format": "b64_json"}
	if !r.Vector {
		body["image_format"] = "png"
	}
	raw, _, err := services.DoRaw(ctx, "POST", "https://external.api.recraft.ai/v1/images/generations", h, body)
	if err != nil {
		return nil, err
	}
	im, err := findImage(ctx, raw)
	if err != nil || !r.Alpha || r.Vector {
		return im, err
	}
	return RecraftEdit(ctx, key, "removeBackground", im.Data)
}

// RecraftEdit runs one of Recraft's image tools (vectorize, removeBackground) on an image.
func RecraftEdit(ctx context.Context, key, tool string, img []byte) (*Image, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("image", "image.png")
	fw.Write(img)
	w.WriteField("response_format", "b64_json")
	w.Close()
	raw, _, err := services.DoRaw(ctx, "POST", "https://external.api.recraft.ai/v1/images/"+tool,
		map[string]string{"Authorization": "Bearer " + key, "Content-Type": w.FormDataContentType()}, buf.Bytes())
	if err != nil {
		return nil, err
	}
	im, err := findImage(ctx, raw)
	if err == nil {
		im.Provider, im.Model = "recraft", tool
	}
	return im, err
}

// ---------- reading answers ----------

// findImage walks a JSON answer for the first image: base64 data next to a mime type
// (Gemini inline data, OpenAI and OpenRouter b64_json) or a URL to fetch (Recraft).
func findImage(ctx context.Context, raw []byte) (*Image, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("unexpected answer: %w", err)
	}
	var b64, mime, url, text string
	var walk func(x any)
	walk = func(x any) {
		if b64 != "" {
			return
		}
		switch t := x.(type) {
		case map[string]any:
			data, _ := t["data"].(string)
			m, _ := t["mimeType"].(string)
			if m == "" {
				m, _ = t["mime_type"].(string)
			}
			if m == "" {
				m, _ = t["media_type"].(string)
			}
			if data != "" && strings.HasPrefix(m, "image/") {
				b64, mime = data, m
				return
			}
			if s, ok := t["b64_json"].(string); ok && s != "" {
				b64, mime = s, m
				return
			}
			if s, ok := t["url"].(string); ok && url == "" && strings.HasPrefix(s, "http") {
				url = s
			}
			if s, ok := t["text"].(string); ok && text == "" {
				text = s
			}
			for _, k := range sortedKeys(t) {
				walk(t[k])
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	if b64 != "" {
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("image data: %w", err)
		}
		if mime == "" {
			mime = sniff(data)
		}
		return &Image{Data: data, MIME: mime}, nil
	}
	if url != "" {
		data, ctype, err := services.DoRaw(ctx, "GET", url, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching the image: %w", err)
		}
		if ctype == "" || !strings.HasPrefix(ctype, "image/") {
			ctype = sniff(data)
		}
		return &Image{Data: data, MIME: ctype}, nil
	}
	if text != "" {
		return nil, fmt.Errorf("no image in the answer; the model said: %s", clip(text, 300))
	}
	return nil, fmt.Errorf("no image in the answer")
}

func sniff(data []byte) string {
	if bytes.Contains(data[:min(len(data), 512)], []byte("<svg")) {
		return "image/svg+xml"
	}
	return http.DetectContentType(data)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Deterministic order; "candidates" and "data" come before "usage" metadata.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
