// Package music generates instrumental tracks with an API: ElevenLabs Music (with a
// composition plan whose sections follow the storyboard's shots) or Stable Audio.
//
// Request shapes: ElevenLabs from its API reference read on 2026-10-08. The Stable Audio
// endpoint (v2beta text-to-audio) could not be confirmed against the current reference on
// that date and may need updating. Neither was run against the live service.
package music

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"mime/multipart"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/services"
)

// Section is one part of a planned track.
type Section struct {
	Name     string   `json:"name"`
	Styles   []string `json:"styles,omitempty"`
	Seconds  float64  `json:"seconds"`
	Negative []string `json:"negative,omitempty"`
}

// Request is one track to make.
type Request struct {
	Prompt   string
	Seconds  float64
	Sections []Section // optional plan; their lengths should add up to Seconds
	Model    string
}

// Track is the result.
type Track struct {
	Data     []byte
	Ext      string
	Provider string
	Model    string
}

func Generate(ctx context.Context, c *services.Config, ch *services.Choice, r Request) (*Track, error) {
	switch ch.Service {
	case "elevenlabs":
		return elevenlabs(ctx, c, ch.Key, r)
	case "stability":
		return stability(ctx, c, ch.Key, r)
	}
	return nil, fmt.Errorf("%s does not make music", ch.Service)
}

func elevenlabs(ctx context.Context, c *services.Config, key string, r Request) (*Track, error) {
	model := r.Model
	if model == "" {
		model = c.Models["elevenlabs"]
	}
	if model == "" {
		model = "music_v1"
	}
	body := map[string]any{"model_id": model}
	if len(r.Sections) > 0 {
		global := []string{r.Prompt, "instrumental"}
		if model == "music_v1" {
			var secs []any
			for _, s := range r.Sections {
				secs = append(secs, map[string]any{
					"section_name": s.Name, "positive_local_styles": nonNil(s.Styles), "negative_local_styles": nonNil(s.Negative),
					"duration_ms": clampMS(s.Seconds), "lines": []string{},
				})
			}
			body["composition_plan"] = map[string]any{"positive_global_styles": global, "negative_global_styles": []string{"vocals", "singing"}, "sections": secs}
		} else {
			var chunks []any
			for _, s := range r.Sections {
				chunks = append(chunks, map[string]any{"text": "[" + s.Name + "]", "duration_ms": clampMS(s.Seconds),
					"positive_styles": append(append([]string{}, global...), s.Styles...), "negative_styles": append([]string{"vocals"}, s.Negative...)})
			}
			body["composition_plan"] = map[string]any{"chunks": chunks}
		}
	} else {
		body["prompt"] = r.Prompt
		body["music_length_ms"] = int(math.Max(3000, math.Min(600000, r.Seconds*1000)))
		body["force_instrumental"] = true
	}
	data, ctype, err := services.DoRaw(ctx, "POST", "https://api.elevenlabs.io/v1/music?output_format=mp3_48000_192", map[string]string{"xi-api-key": key}, body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs (%s): %w", model, err)
	}
	if strings.Contains(ctype, "json") {
		return nil, fmt.Errorf("elevenlabs answered with JSON, not audio: %s", clip(string(data), 300))
	}
	return &Track{Data: data, Ext: ".mp3", Provider: "elevenlabs", Model: model}, nil
}

func clampMS(sec float64) int { return int(math.Max(3000, math.Min(120000, sec*1000))) }

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func stability(ctx context.Context, c *services.Config, key string, r Request) (*Track, error) {
	model := r.Model
	if model == "" {
		model = c.Models["stability"]
	}
	if model == "" {
		model = "stable-audio-2.5"
	}
	prompt := r.Prompt
	if len(r.Sections) > 0 {
		// Stable Audio takes one prompt: describe the sections in order.
		var parts []string
		at := 0.0
		for _, s := range r.Sections {
			parts = append(parts, fmt.Sprintf("%s at %.0fs (%s)", s.Name, at, strings.Join(s.Styles, ", ")))
			at += s.Seconds
		}
		prompt += ". Structure: " + strings.Join(parts, "; ")
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("prompt", prompt+". Instrumental, no vocals.")
	w.WriteField("duration", strconv.Itoa(int(math.Max(1, math.Min(190, math.Ceil(r.Seconds))))))
	w.WriteField("model", model)
	w.WriteField("output_format", "mp3")
	w.Close()
	data, ctype, err := services.DoRaw(ctx, "POST", "https://api.stability.ai/v2beta/audio/stable-audio-2/text-to-audio",
		map[string]string{"Authorization": "Bearer " + key, "Accept": "audio/*", "Content-Type": w.FormDataContentType()}, buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("stability (%s): %w", model, err)
	}
	if strings.Contains(ctype, "json") {
		return nil, fmt.Errorf("stability answered with JSON, not audio: %s", clip(string(data), 300))
	}
	return &Track{Data: data, Ext: ".mp3", Provider: "stability", Model: model}, nil
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
