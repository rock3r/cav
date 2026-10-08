// Package music generates instrumental tracks with an API: ElevenLabs Music (with a
// composition plan whose sections follow the storyboard's shots), Stable Audio, or an
// ACE-Step 1.5 server the user runs.
//
// Request shapes: ElevenLabs from its API reference read on 2026-10-08. The Stable Audio
// endpoint (v2beta text-to-audio) could not be confirmed against the current reference on
// that date and may need updating. Neither was run against the live service.
package music

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

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
	case "acestep":
		return acestep(ctx, c, ch.Key, r)
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

// acestep runs text2music on an ACE-Step 1.5 API server: submit the task, ask for its
// result until it is done, then download the audio. API docs (docs/en/API.md) read on
// 2026-10-08.
func acestep(ctx context.Context, c *services.Config, key string, r Request) (*Track, error) {
	ep := c.Endpoints["acestep"]
	base := strings.TrimRight(ep.BaseURL, "/")
	h := map[string]string{}
	if key != "" {
		h["Authorization"] = "Bearer " + key
	}
	model := r.Model
	if model == "" {
		model = ep.Model
	}
	prompt := r.Prompt
	if len(r.Sections) > 0 {
		var parts []string
		at := 0.0
		for _, s := range r.Sections {
			parts = append(parts, fmt.Sprintf("%s at %.0fs", s.Name, at))
			at += s.Seconds
		}
		prompt += ". Structure: " + strings.Join(parts, "; ")
	}
	body := map[string]any{"prompt": prompt + ", instrumental", "lyrics": "[Instrumental]", "audio_format": "wav",
		"audio_duration": math.Max(10, math.Min(600, r.Seconds)), "batch_size": 1, "thinking": true}
	if bpm := bpmIn(r.Prompt); bpm > 0 {
		body["bpm"] = bpm
	}
	if model != "" {
		body["model"] = model
	}
	type envelope struct {
		Data  json.RawMessage `json:"data"`
		Code  int             `json:"code"`
		Error *string         `json:"error"`
	}
	call := func(method, path string, in any) (json.RawMessage, error) {
		var e envelope
		if err := services.Do(ctx, method, base+path, h, in, &e); err != nil {
			return nil, fmt.Errorf("acestep: %w", err)
		}
		if e.Error != nil && *e.Error != "" {
			return nil, fmt.Errorf("acestep: %s", *e.Error)
		}
		return e.Data, nil
	}
	raw, err := call("POST", "/release_task", body)
	if err != nil {
		return nil, err
	}
	var task struct {
		ID string `json:"task_id"`
	}
	if err := json.Unmarshal(raw, &task); err != nil || task.ID == "" {
		return nil, fmt.Errorf("acestep: no task id in %s", clip(string(raw), 200))
	}
	// The server runs tasks on its own queue and has no push channel: ask until it is done.
	for {
		raw, err := call("POST", "/query_result", map[string]any{"task_id_list": []string{task.ID}})
		if err != nil {
			return nil, err
		}
		var res []struct {
			Status int    `json:"status"`
			Result string `json:"result"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, fmt.Errorf("acestep: unexpected result %s", clip(string(raw), 200))
		}
		if len(res) > 0 && res[0].Status == 2 {
			return nil, fmt.Errorf("acestep: the task failed: %s", clip(res[0].Result, 300))
		}
		if len(res) > 0 && res[0].Status == 1 {
			var files []struct {
				File  string `json:"file"`
				Model string `json:"dit_model"`
			}
			if err := json.Unmarshal([]byte(res[0].Result), &files); err != nil || len(files) == 0 || files[0].File == "" {
				return nil, fmt.Errorf("acestep: no audio in %s", clip(res[0].Result, 200))
			}
			data, _, err := services.DoRaw(ctx, "GET", base+files[0].File, h, nil)
			if err != nil {
				return nil, fmt.Errorf("acestep: downloading the audio: %w", err)
			}
			m := files[0].Model
			if m == "" {
				m = model
			}
			return &Track{Data: data, Ext: ".wav", Provider: "acestep", Model: m}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// bpmIn finds "<n> BPM" in a prompt (cav music gen --board adds it).
func bpmIn(prompt string) int {
	f := strings.Fields(strings.ToLower(prompt))
	for i := 1; i < len(f); i++ {
		if strings.TrimRight(f[i], ",.;") == "bpm" {
			if n, err := strconv.ParseFloat(strings.TrimRight(f[i-1], ","), 64); err == nil && n >= 30 && n <= 300 {
				return int(math.Round(n))
			}
		}
	}
	return 0
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// SoundEffect generates a sound effect with ElevenLabs (0.5-30 s; loop needs the v2 model,
// which is the default). API reference read on 2026-10-08.
func SoundEffect(ctx context.Context, key, text string, seconds float64, loop bool) (*Track, error) {
	body := map[string]any{"text": text, "loop": loop, "prompt_influence": 0.5}
	if seconds > 0 {
		body["duration_seconds"] = math.Max(0.5, math.Min(30, seconds))
	}
	data, ctype, err := services.DoRaw(ctx, "POST", "https://api.elevenlabs.io/v1/sound-generation?output_format=mp3_44100_128",
		map[string]string{"xi-api-key": key}, body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs sound effects: %w", err)
	}
	if strings.Contains(ctype, "json") {
		return nil, fmt.Errorf("elevenlabs answered with JSON, not audio: %s", clip(string(data), 300))
	}
	return &Track{Data: data, Ext: ".mp3", Provider: "elevenlabs", Model: "eleven_text_to_sound_v2"}, nil
}
