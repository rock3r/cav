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
	"slices"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/services"
)

// yue2 makes an instrumental take on a YuE2 server: cav's wrapper (tools/yue2-server) or
// YuE2-Turbo. Both speak the job API of YuE2-Turbo's src/yue2/service.py (read at commit
// 7c88813 on 2026-10-11): POST /v1/jobs, then GET /v1/jobs/{id} until the status is
// terminal, then GET /v1/jobs/{id}/audio (FLAC).
//
// YuE2 writes songs with vocals. An instrumental takes three steps, the way the YuE2
// repository's instrumental helper does it: YuE2 plans a score, the Vocal notes of the
// score move to the Ins voice, and YuE2 generates audio from the changed score. YuE2 has no
// length setting either, but its audio follows the score closely, so cav cuts the score to
// about the length it wants before the last step.
//
// cav's wrapper does all of this on the server: cav sends "instrumental": true and
// "seconds", two extensions that /health/ready lists in "extensions". YuE2-Turbo has no
// extensions and refuses unknown fields, so with Turbo cav runs two jobs: the first plans
// the score (cav downloads it from /v1/jobs/{id}/score and drops that job's audio), cav
// converts and cuts the score itself (yue2abc.go), and the second job generates from it as
// "abc". Turbo writes a minute of audio in about ten seconds, so the extra job costs
// little. On the wrapper, which is slower, the extensions skip the first job's audio.
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
	extensions, err := yue2Ready(ctx, base)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(extensions, "instrumental") {
		return yue2Turbo(ctx, base, h, seed.Int64(), r)
	}
	// The brief goes in as YuE2's style; with --board it already holds the tempo.
	body := map[string]any{"style": strings.TrimSpace(r.Prompt), "lyrics": yue2Sections, "cot": "full",
		"seed": seed.Int64(), "n": 1, "instrumental": true}
	if r.Seconds > 0 {
		body["seconds"] = math.Max(5, math.Min(600, r.Seconds))
	}
	job, jobURL, err := yue2Run(ctx, base, h, body)
	if err != nil {
		return nil, err
	}
	return yue2Download(ctx, base, h, job, jobURL, seed.Int64(), r.Seconds)
}

// yue2Turbo makes an instrumental take on a server with no extensions (YuE2-Turbo): one job
// plans the score, cav turns it into an instrumental, a second job generates the audio.
func yue2Turbo(ctx context.Context, base string, h map[string]string, seed int64, r Request) (*Track, error) {
	start := time.Now()
	style := strings.TrimSpace(r.Prompt)
	plan, planURL, err := yue2Run(ctx, base, h, map[string]any{"style": style, "lyrics": yue2Sections,
		"cot": "full", "seed": seed, "n": 1})
	if err != nil {
		return nil, err
	}
	if plan.Result != nil && plan.Result.Truncated["abc"] {
		return nil, fmt.Errorf("yue2: YuE2 planned a truncated score; run the command again for another seed")
	}
	scoreURL := planURL + "/score"
	if plan.Result != nil && plan.Result.ScoreURL != "" {
		scoreURL = base + plan.Result.ScoreURL
	}
	planned, _, err := services.DoRaw(ctx, "GET", scoreURL, h, nil)
	if err != nil {
		return nil, fmt.Errorf("yue2: downloading the planned score: %w", err)
	}
	seconds := 0.0
	if r.Seconds > 0 {
		seconds = math.Max(5, math.Min(600, r.Seconds))
	}
	in, err := makeInstrumental(string(planned), style, seconds)
	if err != nil {
		return nil, fmt.Errorf("yue2: making the planned score instrumental: %w", err)
	}
	job, jobURL, err := yue2Run(ctx, base, h, map[string]any{"style": in.style, "lyrics": in.lyrics,
		"abc": in.abc, "cot": in.cot, "seed": seed, "n": 1})
	if err != nil {
		return nil, err
	}
	if job.Result != nil {
		job.Result.Instrumental = &yue2Score{NominalSeconds: in.nominal, PlannedSeconds: in.planned, BPM: in.bpm}
		job.Result.Timing.Job = time.Since(start).Seconds()
		job.Result.Timing.TwoJobs = true
	}
	return yue2Download(ctx, base, h, job, jobURL, seed, r.Seconds)
}

// yue2Ready waits for the server to load the model, then returns the extensions it lists.
func yue2Ready(ctx context.Context, base string) ([]string, error) {
	for {
		var ready struct {
			Status     string   `json:"status"`
			Error      string   `json:"error"`
			Extensions []string `json:"extensions"`
		}
		raw, _, err := services.DoRaw(ctx, "GET", base+"/health/ready", nil, nil)
		var he *services.HTTPError
		if errors.As(err, &he) && he.Status == 503 {
			json.Unmarshal([]byte(he.Body), &ready)
			if ready.Status == "failed" {
				if ready.Error == "" {
					ready.Error = "see the server's log"
				}
				return nil, fmt.Errorf("yue2: the server could not load YuE2: %s", clip(ready.Error, 300))
			}
			// 503 while loading: come back.
			if err := sleep(ctx, yue2Retry); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("yue2: %w", err)
		}
		json.Unmarshal(raw, &ready)
		return ready.Extensions, nil
	}
}

// yue2Run submits one job, waits for it to finish and returns it with its URL. When ctx
// ends first, it cancels the job.
func yue2Run(ctx context.Context, base string, h map[string]string, body map[string]any) (*yue2Job, string, error) {
	// One key per job: a retried submit then finds the same job instead of starting a second.
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
			if _, rerr := yue2Ready(ctx, base); rerr != nil {
				return nil, "", rerr
			}
			continue
		}
		if errors.As(err, &he) && he.Status == 429 {
			// A full queue: come back.
			if err := sleep(ctx, yue2Retry); err != nil {
				return nil, "", err
			}
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("yue2: %w", err)
		}
		break
	}
	if job.ID == "" {
		return nil, "", fmt.Errorf("yue2: the server answered with no job id")
	}
	jobURL := base + "/v1/jobs/" + url.PathEscape(job.ID)
	for job.Status == "queued" || job.Status == "running" {
		if err := sleep(ctx, yue2Poll); err != nil {
			// Free the GPU: the server would otherwise finish a take nobody collects.
			cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			services.Do(cctx, "POST", jobURL+"/cancel", h, nil, nil)
			cancel()
			return nil, "", err
		}
		if err := services.Do(ctx, "GET", jobURL, h, nil, &job); err != nil {
			return nil, "", fmt.Errorf("yue2: %w", err)
		}
	}
	switch job.Status {
	case "succeeded", "truncated":
	case "failed":
		msg := "no reason given"
		if job.Error != nil && job.Error.Message != "" {
			msg = job.Error.Message
		}
		return nil, "", fmt.Errorf("yue2: the job failed: %s", clip(msg, 300))
	default:
		return nil, "", fmt.Errorf("yue2: the job ended as %q", job.Status)
	}
	return &job, jobURL, nil
}

// yue2Download fetches the audio of a finished job.
func yue2Download(ctx context.Context, base string, h map[string]string, job *yue2Job, jobURL string, seed int64, want float64) (*Track, error) {
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
	tr := &Track{Data: data, Ext: ".flac", Provider: "yue2", Model: model, Seed: &seed}
	if job.Result != nil {
		tr.Notes = job.Result.notes(want, seed)
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
	ScoreURL     string          `json:"score_url"`
	Model        string          `json:"model"` // cav's wrapper: the model it loaded
	AudioSeconds float64         `json:"audio_seconds"`
	Truncated    map[string]bool `json:"truncated"`
	Timing       struct {
		E2E float64 `json:"e2e_seconds"`
		Job float64 `json:"job_seconds"` // cav's wrapper: planning included
		// TwoJobs is set by cav, not the server: Job then covers the planning job too.
		TwoJobs bool `json:"-"`
	} `json:"timing"`
	Instrumental *yue2Score `json:"instrumental"`
}

// yue2Score is the length of the instrumental score: as YuE2 planned it, and as cut.
type yue2Score struct {
	NominalSeconds float64 `json:"nominal_seconds"`
	PlannedSeconds float64 `json:"planned_seconds"`
	BPM            float64 `json:"bpm"`
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
	cutter := "the server"
	if res.Timing.TwoJobs {
		length += " over two jobs"
		cutter = "cav"
	}
	if in := res.Instrumental; in != nil && in.NominalSeconds > 0 {
		if in.PlannedSeconds > 0 && math.Abs(in.PlannedSeconds-in.NominalSeconds) > 0.05 {
			length += fmt.Sprintf(" (YuE2 planned %.1f s at %g BPM; %s cut the score to %.1f s)", in.PlannedSeconds, in.BPM, cutter, in.NominalSeconds)
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
