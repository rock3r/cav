// Package music generates instrumental tracks with an API: ElevenLabs Music (with a
// composition plan whose sections follow the storyboard's shots), Stable Audio, Lyria
// through the Gemini API, or an ACE-Step 1.5 server the user runs.
//
// Request shapes: ElevenLabs from its API reference read on 2026-10-08; Lyria from the Gemini
// API music generation guide read on 2026-10-08; Stable Audio (stable-audio-2 text-to-audio
// and audio-to-audio) from Stability's OpenAPI spec read on 2026-10-09. None was run against
// the live service.
//
// A reference track (Request.Ref) goes to the services that take audio: Stable Audio
// (audio-to-audio) and ACE-Step (its reference_audio style input). Lyria takes only text and
// images, and ElevenLabs takes audio only through an upload endpoint, so for those the
// caller describes the reference in words instead (see SendsAudio).
package music

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
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
	Ref      string  // optional reference track (a local file) for services that take audio
	Keep     float64 // how much of Ref to keep, 0-1 (Stable Audio only)
}

// SendsAudio reports whether the service takes a reference track as audio.
func SendsAudio(service string) bool { return service == "stability" || service == "acestep" }

// Track is the result.
type Track struct {
	Data     []byte
	Ext      string
	Provider string
	Model    string
}

func Generate(ctx context.Context, c *services.Config, ch *services.Choice, r Request) (*Track, error) {
	if r.Ref != "" && !SendsAudio(ch.Service) {
		return nil, fmt.Errorf("%s cannot take a reference track as audio", ch.Service)
	}
	switch ch.Service {
	case "elevenlabs":
		return elevenlabs(ctx, c, ch.Key, r)
	case "stability":
		return stability(ctx, c, ch.Key, r)
	case "acestep":
		return acestep(ctx, c, ch.Key, r)
	case "lyria":
		return lyria(ctx, c, ch.Key, r)
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
					"section_name": shotID(s.Name), "positive_local_styles": nonNil(s.Styles), "negative_local_styles": nonNil(s.Negative),
					"duration_ms": clampMS(s.Seconds), "lines": []string{},
				})
			}
			body["composition_plan"] = map[string]any{"positive_global_styles": global, "negative_global_styles": []string{"vocals", "singing"}, "sections": secs}
		} else {
			var chunks []any
			for _, s := range r.Sections {
				chunks = append(chunks, map[string]any{"text": "[" + shotID(s.Name) + "]", "duration_ms": clampMS(s.Seconds),
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

// shotID keeps the shot id of a "s4a: what happens on screen" section name. ElevenLabs gets
// only the id: its moderation refuses a plan that names a brand or product, and a shot's
// screen description is not musical anyway. The shot's music prompt still goes in its styles.
func shotID(name string) string {
	id, _, _ := strings.Cut(name, ":")
	return strings.TrimSpace(id)
}

func clampMS(sec float64) int { return int(math.Max(3000, math.Min(120000, sec*1000))) }

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// attach adds a local file to a multipart form.
func attach(w *multipart.Writer, field, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reference track: %w", err)
	}
	part, err := w.CreateFormFile(field, filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
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
	endpoint := "text-to-audio"
	if r.Ref != "" {
		// audio-to-audio: strength 0 gives the reference back, 1 ignores it.
		endpoint = "audio-to-audio"
		w.WriteField("strength", strconv.FormatFloat(1-math.Max(0, math.Min(1, r.Keep)), 'f', 2, 64))
		if err := attach(w, "audio", r.Ref); err != nil {
			return nil, err
		}
	}
	w.Close()
	data, ctype, err := services.DoRaw(ctx, "POST", "https://api.stability.ai/v2beta/audio/stable-audio-2/"+endpoint,
		map[string]string{"Authorization": "Bearer " + key, "Accept": "audio/*", "Content-Type": w.FormDataContentType()}, buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("stability (%s): %w", model, err)
	}
	if strings.Contains(ctype, "json") {
		return nil, fmt.Errorf("stability answered with JSON, not audio: %s", clip(string(data), 300))
	}
	return &Track{Data: data, Ext: ".mp3", Provider: "stability", Model: model}, nil
}

// lyria makes a track with the Interactions API. Lyria has no length or section fields: the
// guide says to ask for the length in the prompt and to time sections with "[m:ss - m:ss]"
// lines, so a storyboard plan becomes one such line per shot. lyria-3.5 answers with MP3 by
// default. cav asks for WAV, for editing: response_format is an AudioResponseFormat whose
// mime_type is audio/wav (Interactions OpenAPI spec, read on 2026-10-09). With type "audio"
// alone, Lyria answered MP3 in a live call on 2026-10-09. The clip
// model (lyria-3-clip-preview) always makes 30 seconds.
func lyria(ctx context.Context, c *services.Config, key string, r Request) (*Track, error) {
	model := r.Model
	if model == "" {
		model = c.Models["lyria"]
	}
	if model == "" {
		model = "lyria-3.5"
	}
	body := map[string]any{"model": model, "input": LyriaPrompt(r)}
	if model != "lyria-3-clip-preview" {
		body["response_format"] = map[string]any{"type": "audio", "mime_type": "audio/wav"}
	}
	var out struct {
		Steps []struct {
			Type    string `json:"type"`
			Content []struct {
				Type     string `json:"type"`
				Data     string `json:"data"`
				MimeType string `json:"mime_type"`
				Text     string `json:"text"`
			} `json:"content"`
		} `json:"steps"`
	}
	if err := services.Do(ctx, "POST", "https://generativelanguage.googleapis.com/v1beta/interactions", map[string]string{"x-goog-api-key": key}, body, &out); err != nil {
		return nil, fmt.Errorf("lyria (%s): %w", model, err)
	}
	// The guide says to take the last audio block of the model_output steps.
	var b64, text string
	for _, st := range out.Steps {
		if st.Type != "model_output" {
			continue
		}
		for _, ct := range st.Content {
			switch ct.Type {
			case "audio":
				b64 = ct.Data
			case "text":
				if text == "" {
					text = ct.Text
				}
			}
		}
	}
	if b64 == "" {
		if text != "" {
			return nil, fmt.Errorf("lyria (%s): no audio in the answer; the model said: %s", model, clip(text, 300))
		}
		return nil, fmt.Errorf("lyria (%s): no audio in the answer", model)
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("lyria (%s): audio data: %w", model, err)
	}
	ext := ".mp3"
	if bytes.HasPrefix(data, []byte("RIFF")) {
		ext = ".wav"
	}
	return &Track{Data: data, Ext: ext, Provider: "lyria", Model: model}, nil
}

// LyriaPrompt writes the request as one prompt: the brief, the length, "instrumental only",
// and one timed line per section.
func LyriaPrompt(r Request) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(r.Prompt))
	secs := r.Seconds
	if len(r.Sections) > 0 {
		secs = 0
		for _, s := range r.Sections {
			secs += s.Seconds
		}
	}
	if secs > 0 {
		fmt.Fprintf(&b, ". Exactly %s long", mmss(secs))
	}
	b.WriteString(". Instrumental only, no vocals.")
	at := 0.0
	for _, s := range r.Sections {
		fmt.Fprintf(&b, "\n[%s - %s] %s", mmss(at), mmss(at+s.Seconds), s.Name)
		if len(s.Styles) > 0 {
			b.WriteString(": " + strings.Join(s.Styles, ", "))
		}
		if len(s.Negative) > 0 {
			b.WriteString(" (avoid " + strings.Join(s.Negative, ", ") + ")")
		}
		at += s.Seconds
	}
	return b.String()
}

func mmss(sec float64) string {
	n := int(math.Round(sec))
	return fmt.Sprintf("%d:%02d", n/60, n%60)
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
	// A reference track goes up as the reference_audio file of a multipart form; the server
	// uses it as a style reference for text2music. The other fields go as form values.
	var release any = body
	releaseHeaders := h
	if r.Ref != "" {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		keys := make([]string, 0, len(body))
		for k := range body {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			w.WriteField(k, fmt.Sprint(body[k]))
		}
		if err := attach(w, "reference_audio", r.Ref); err != nil {
			return nil, err
		}
		w.Close()
		release = buf.Bytes()
		releaseHeaders = map[string]string{"Content-Type": w.FormDataContentType()}
		for k, v := range h {
			releaseHeaders[k] = v
		}
	}
	type envelope struct {
		Data  json.RawMessage `json:"data"`
		Code  int             `json:"code"`
		Error *string         `json:"error"`
	}
	call := func(method, path string, headers map[string]string, in any) (json.RawMessage, error) {
		var e envelope
		if err := services.Do(ctx, method, base+path, headers, in, &e); err != nil {
			return nil, fmt.Errorf("acestep: %w", err)
		}
		if e.Error != nil && *e.Error != "" {
			return nil, fmt.Errorf("acestep: %s", *e.Error)
		}
		return e.Data, nil
	}
	raw, err := call("POST", "/release_task", releaseHeaders, release)
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
		raw, err := call("POST", "/query_result", h, map[string]any{"task_id_list": []string{task.ID}})
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
