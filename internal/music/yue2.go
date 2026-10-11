package music

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/services"
)

// yue2 makes an instrumental take on a YuE2 server: cav's wrapper (tools/yue2-server) or,
// later, YuE2-Turbo. Both speak the job API of YuE2-Turbo's src/yue2/service.py (read on
// 2026-10-11): POST /v1/jobs, then GET /v1/jobs/{id} until the status is terminal, then GET
// /v1/jobs/{id}/audio (FLAC).
//
// cav asks for "instrumental": true. That field is an extension of cav's wrapper: YuE2
// writes songs with vocals, and the wrapper makes an instrumental the way the YuE2
// repository's instrumental helper does (plan a score, move its Vocal notes to the Ins
// voice, generate again from the changed score). YuE2-Turbo has no such field and refuses
// unknown fields.
//
// YuE2 has no length setting: its planner decides how many bars to write. Its audio follows
// the planned score closely, though, so cav sends "seconds" (another extension) and the
// wrapper cuts the planned score to about that length at a phrase or section boundary
// before it generates. cav then reports the length that came back.
func yue2(ctx context.Context, c *services.Config, key string, r Request) (*Track, error) {
	ep := c.Endpoints["yue2"]
	base := strings.TrimRight(ep.BaseURL, "/")
	h := map[string]string{}
	if key != "" {
		h["Authorization"] = "Bearer " + key
	}
	// The server loads one model at startup, so the model comes from its answer.
	if r.Model != "" {
		return nil, fmt.Errorf("yue2: the server chooses the model when it starts (its --model flag), not cav")
	}
	seed, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}
	// The brief goes in as YuE2's style; with --board it already holds the tempo.
	body := map[string]any{"style": strings.TrimSpace(r.Prompt), "lyrics": yue2Sections, "cot": "full",
		"seed": seed.Int64(), "n": 1, "instrumental": true}
	if r.Seconds > 0 {
		body["seconds"] = math.Max(5, math.Min(600, r.Seconds))
	}
	// One key per take: a retried submit then finds the same job instead of starting a second.
	idem := make([]byte, 16)
	rand.Read(idem)
	submitHeaders := map[string]string{"Idempotency-Key": hex.EncodeToString(idem)}
	for k, v := range h {
		submitHeaders[k] = v
	}
	var job yue2Job
	for {
		err := services.Do(ctx, "POST", base+"/v1/jobs", submitHeaders, body, &job)
		var he *services.HTTPError
		if errors.As(err, &he) && he.Status == 503 {
			// 503 means the model is loading, or failed to load: only the first passes.
			var ready struct {
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			raw, _, herr := services.DoRaw(ctx, "GET", base+"/health/ready", nil, nil)
			var re *services.HTTPError
			if errors.As(herr, &re) {
				raw = []byte(re.Body)
			}
			json.Unmarshal(raw, &ready)
			if ready.Status == "failed" {
				return nil, fmt.Errorf("yue2: the server could not load YuE2: %s", clip(ready.Error, 300))
			}
		}
		if errors.As(err, &he) && (he.Status == 429 || he.Status == 503) {
			// A full queue (429) or a model still loading (503): both say to come back.
			if err := sleep(ctx, yue2Retry); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("yue2: %w", err)
		}
		break
	}
	if job.ID == "" {
		return nil, fmt.Errorf("yue2: the server answered with no job id")
	}
	jobURL := base + "/v1/jobs/" + url.PathEscape(job.ID)
	for job.Status == "queued" || job.Status == "running" {
		if err := sleep(ctx, yue2Poll); err != nil {
			// Free the GPU: the server would otherwise finish a take nobody collects.
			cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			services.Do(cctx, "POST", jobURL+"/cancel", h, nil, nil)
			cancel()
			return nil, err
		}
		if err := services.Do(ctx, "GET", jobURL, h, nil, &job); err != nil {
			return nil, fmt.Errorf("yue2: %w", err)
		}
	}
	switch job.Status {
	case "succeeded", "truncated":
	case "failed":
		msg := "no reason given"
		if job.Error != nil && job.Error.Message != "" {
			msg = job.Error.Message
		}
		return nil, fmt.Errorf("yue2: the job failed: %s", clip(msg, 300))
	default:
		return nil, fmt.Errorf("yue2: the job ended as %q", job.Status)
	}
	audio := jobURL + "/audio"
	if job.Result != nil && job.Result.AudioURL != "" {
		audio = base + job.Result.AudioURL
	}
	data, _, err := services.DoRaw(ctx, "GET", audio, h, nil)
	if err != nil {
		return nil, fmt.Errorf("yue2: downloading the audio: %w", err)
	}
	model := "YuE2-3B"
	if job.Result != nil && job.Result.Model != "" {
		model = job.Result.Model
	}
	tr := &Track{Data: data, Ext: ".flac", Provider: "yue2", Model: model}
	if job.Result != nil {
		tr.Notes = job.Result.notes(r.Seconds, seed.Int64())
	}
	return tr, nil
}

// How often cav asks about a job, and how long it waits when the server is busy or loading.
var yue2Poll, yue2Retry = 3 * time.Second, 5 * time.Second

type yue2Job struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Stage  string `json:"stage"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Result *yue2Result `json:"result"`
}

type yue2Result struct {
	AudioURL     string          `json:"audio_url"`
	Model        string          `json:"model"` // cav's wrapper: the model it loaded
	AudioSeconds float64         `json:"audio_seconds"`
	Truncated    map[string]bool `json:"truncated"`
	Timing       struct {
		E2E float64 `json:"e2e_seconds"`
		Job float64 `json:"job_seconds"` // cav's wrapper: planning included
	} `json:"timing"`
	Instrumental *struct {
		NominalSeconds float64 `json:"nominal_seconds"`
		PlannedSeconds float64 `json:"planned_seconds"`
		BPM            float64 `json:"bpm"`
	} `json:"instrumental"`
}

// notes says how long the take came out against the request, and which part of the
// generation stopped at its token limit.
func (res *yue2Result) notes(want float64, seed int64) []string {
	var out []string
	took := res.Timing.Job
	if took == 0 {
		took = res.Timing.E2E
	}
	length := fmt.Sprintf("seed %d: %.1f s of audio in %.0f s", seed, res.AudioSeconds, took)
	if in := res.Instrumental; in != nil && in.NominalSeconds > 0 {
		if in.PlannedSeconds > 0 && math.Abs(in.PlannedSeconds-in.NominalSeconds) > 0.05 {
			length += fmt.Sprintf(" (YuE2 planned %.1f s at %g BPM; the server cut the score to %.1f s)", in.PlannedSeconds, in.BPM, in.NominalSeconds)
		} else {
			length += fmt.Sprintf(" (the score is %.1f s at %g BPM)", in.NominalSeconds, in.BPM)
		}
	}
	if want > 0 && math.Abs(res.AudioSeconds-want) > 1 {
		length += fmt.Sprintf("; you asked for %.1f s: YuE2 cuts only at a phrase, so trim or fade it", want)
	}
	out = append(out, length)
	for _, part := range []string{"abc", "semantic"} {
		if res.Truncated[part] {
			what := map[string]string{"abc": "the planned score", "semantic": "the audio tokens"}[part]
			out = append(out, "truncated: "+what+" reached the token limit, so the take may stop early; listen to the end")
		}
	}
	return out
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// yue2Sections are the planner's "lyrics" for an instrumental: section tags only, the
// default of the YuE2 instrumental helper. They do not set the length: with only an intro
// and an outro, YuE2 planned 116 s and 148 s in tests on 2026-10-11.
const yue2Sections = "[Intro]\n\n[Verse]\n\n[Chorus]\n\n[Outro]\n"
