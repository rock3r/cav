// Package listen turns a track into things an agent can judge without ears: loudness
// numbers, and a written critique from an audio model (Gemini, or Qwen3-Omni on an
// OpenAI-compatible server).
package listen

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/services"
)

// Loudness is the EBU R128 measurement of a whole track.
type Loudness struct {
	Integrated float64 `json:"integratedLUFS"`
	Range      float64 `json:"rangeLU"`
	TruePeak   float64 `json:"truePeakDBFS"`
}

var (
	reI    = regexp.MustCompile(`I:\s+(-?[0-9.]+|-inf)\s+LUFS`)
	reLRA  = regexp.MustCompile(`LRA:\s+(-?[0-9.]+)\s+LU`)
	rePeak = regexp.MustCompile(`Peak:\s+(-?[0-9.]+|-inf)\s+dBFS`)
)

// Measure runs ffmpeg's ebur128 filter with true-peak metering.
func Measure(ctx context.Context, path string) (Loudness, error) {
	out, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostats", "-i", path, "-filter_complex", "ebur128=peak=true", "-f", "null", "-").CombinedOutput()
	if err != nil {
		return Loudness{}, fmt.Errorf("ffmpeg ebur128: %v", err)
	}
	return parseLoudness(string(out))
}

func parseLoudness(s string) (Loudness, error) {
	// The summary comes last; take the last match of each.
	last := func(re *regexp.Regexp) (float64, bool) {
		m := re.FindAllStringSubmatch(s, -1)
		if len(m) == 0 {
			return 0, false
		}
		v := m[len(m)-1][1]
		if v == "-inf" {
			return -99, true
		}
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	}
	var l Loudness
	var ok1, ok2, ok3 bool
	l.Integrated, ok1 = last(reI)
	l.Range, ok2 = last(reLRA)
	l.TruePeak, ok3 = last(rePeak)
	if !ok1 || !ok2 || !ok3 {
		return l, fmt.Errorf("could not read the loudness summary from ffmpeg")
	}
	return l, nil
}

// Advice compares loudness with the usual delivery targets.
func (l Loudness) Advice() []string {
	var out []string
	switch {
	case l.Integrated > -12:
		out = append(out, fmt.Sprintf("loud: %.1f LUFS is above the -14 LUFS that streaming and social platforms turn down to", l.Integrated))
	case l.Integrated < -18:
		out = append(out, fmt.Sprintf("quiet: %.1f LUFS; web and social delivery usually sits near -14 LUFS (broadcast EBU R128: -23)", l.Integrated))
	}
	if l.TruePeak > -1 {
		out = append(out, fmt.Sprintf("true peak %.1f dBFS is above -1 dBFS: it can clip after encoding", l.TruePeak))
	}
	if l.Range < 3 && l.Integrated > -16 {
		out = append(out, fmt.Sprintf("loudness range %.1f LU: little dynamic contrast, so builds and drops will feel flat", l.Range))
	}
	return out
}

// Compact makes a small mono MP3 of the track for an audio model (inline uploads are limited
// in size; the critique does not need full quality).
func Compact(ctx context.Context, in string) ([]byte, error) {
	f, err := os.CreateTemp("", "cav-ears-*.mp3")
	if err != nil {
		return nil, err
	}
	f.Close()
	defer os.Remove(f.Name())
	if b, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-i", in, "-vn", "-ac", "1", "-ar", "32000", "-b:a", "96k", f.Name()).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return os.ReadFile(f.Name())
}

// Prompt builds the instructions for the audio model.
func Prompt(brief, facts string) string {
	var b strings.Builder
	b.WriteString("You are reviewing a music track made for a motion-graphics piece. Listen to the whole track.\n")
	if brief != "" {
		b.WriteString("\nThe brief for the music:\n" + brief + "\n")
	}
	if facts != "" {
		b.WriteString("\nMeasured facts (trust these numbers over your own estimates):\n" + facts + "\n")
	}
	b.WriteString(`
Answer in plain English, in this order:
1. What you hear: genre, instruments, mood, and how the energy moves, with timestamps (m:ss.s).
2. Against the brief: what fits and what does not.
3. Moments a picture edit can hit: drops, hits, risers that land, breaks — each with a timestamp.
4. Problems: clipping, harsh frequencies, muddy low end, abrupt or unfinished ending, artefacts typical of AI-generated audio, vocals where none were wanted.
5. The three changes that would help most.
Keep it under 350 words. Do not invent timestamps you cannot hear.`)
	return b.String()
}

var flashName = regexp.MustCompile(`^models/gemini-([0-9]+(?:\.[0-9]+)?)-flash$`)

// newestFlash picks the newest general Gemini Flash model the key can use (they all take
// audio), falling back to gemini-2.5-flash when the list cannot be read.
func newestFlash(ctx context.Context, key string) string {
	var r struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	best, bestV := "gemini-2.5-flash", 0.0
	if err := services.Do(ctx, "GET", "https://generativelanguage.googleapis.com/v1beta/models?pageSize=200", map[string]string{"x-goog-api-key": key}, nil, &r); err != nil {
		return best
	}
	for _, m := range r.Models {
		if g := flashName.FindStringSubmatch(m.Name); g != nil {
			if v, _ := strconv.ParseFloat(g[1], 64); v > bestV {
				best, bestV = strings.TrimPrefix(m.Name, "models/"), v
			}
		}
	}
	return best
}

// Critique asks the chosen audio model about the track.
func Critique(ctx context.Context, c *services.Config, ch *services.Choice, audio []byte, prompt string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(audio)
	switch ch.Service {
	case "gemini":
		model := c.Models["gemini-ears"]
		if model == "" {
			model = newestFlash(ctx, ch.Key)
		}
		body := map[string]any{"contents": []any{map[string]any{"parts": []any{
			map[string]any{"inline_data": map[string]any{"mime_type": "audio/mp3", "data": b64}},
			map[string]any{"text": prompt},
		}}}}
		var r struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := services.Do(ctx, "POST", "https://generativelanguage.googleapis.com/v1beta/models/"+model+":generateContent",
			map[string]string{"x-goog-api-key": ch.Key}, body, &r); err != nil {
			return "", fmt.Errorf("gemini (%s): %w", model, err)
		}
		var out []string
		for _, cand := range r.Candidates {
			for _, p := range cand.Content.Parts {
				if p.Text != "" {
					out = append(out, p.Text)
				}
			}
		}
		if len(out) == 0 {
			return "", fmt.Errorf("gemini (%s) gave no text", model)
		}
		return strings.TrimSpace(strings.Join(out, "\n")), nil
	case "qwen-omni":
		ep := c.Endpoints["qwen-omni"]
		model := ep.Model
		if model == "" {
			model = "Qwen3-Omni-30B-A3B-Instruct"
		}
		body := map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": b64, "format": "mp3"}},
			map[string]any{"type": "text", "text": prompt},
		}}}}
		h := map[string]string{}
		if ch.Key != "" {
			h["Authorization"] = "Bearer " + ch.Key
		}
		var r struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := services.Do(ctx, "POST", strings.TrimRight(ep.BaseURL, "/")+"/chat/completions", h, body, &r); err != nil {
			return "", fmt.Errorf("qwen-omni (%s): %w", model, err)
		}
		if len(r.Choices) == 0 || r.Choices[0].Message.Content == "" {
			return "", fmt.Errorf("qwen-omni gave no text")
		}
		return strings.TrimSpace(r.Choices[0].Message.Content), nil
	}
	return "", fmt.Errorf("%s cannot listen to audio", ch.Service)
}
