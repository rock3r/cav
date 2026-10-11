package music

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/services"
)

// yue2 makes a take on a YuE2 server: cav's wrapper (tools/yue2-server) or, later,
// YuE2-Turbo. Both speak the job API of YuE2-Turbo's src/yue2/service.py (read on
// 2026-10-11): POST /v1/jobs, then GET /v1/jobs/{id} until the status is terminal, then GET
// /v1/jobs/{id}/audio (FLAC).
//
// With lyrics, cav sends a plain job: YuE2 writes a song and sings them.
//
// Without lyrics, cav asks for "instrumental": true. That field is an extension of cav's
// wrapper: the wrapper makes an instrumental the way the YuE2 repository's instrumental
// helper does (plan a score, move its Vocal notes to the Ins voice, generate again from the
// changed score). YuE2-Turbo has no such field and refuses unknown fields.
//
// YuE2 has no length setting: its planner decides how many bars to write. Its audio follows
// the planned score closely, though, so for an instrumental cav sends "seconds" (another
// extension) and the wrapper cuts the planned score to about that length at a phrase or
// section boundary before it generates. A song with lyrics gets no "seconds": the score has
// the vocal notes but not the syllables, so a cut could stop a sung line in the middle, and
// the lyrics after the cut would still be in the request. The lyrics set its length. cav
// reports the length that came back.
//
// With a reference track (Request.Ref), cav makes a cover: it uploads the track to POST
// /v1/covers, a multipart form of at most 40 MiB with the fields of YuE2-Turbo's covers route
// (audio, style, lyrics, seed; README and service.py read on 2026-10-11). SheetSage2 on the
// server transcribes the track's melody, without chords, and YuE2 generates from it with
// cot=melody. It does not copy the singer. Without lyrics, cav also sends "instrumental"
// and "seconds", so cav's wrapper moves the melody to an instrument; with lyrics, YuE2 sings
// them to the melody. When the server cannot transcribe, the error is ErrNoCover and the
// caller describes the track in words instead.
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
	sung := strings.TrimSpace(r.Lyrics) != ""
	if sung {
		body["lyrics"] = strings.TrimSpace(r.Lyrics)
		delete(body, "instrumental")
	} else if r.Seconds > 0 {
		body["seconds"] = math.Max(5, math.Min(600, r.Seconds))
	}
	// One key per take: a retried submit then finds the same job instead of starting a second.
	idem := make([]byte, 16)
	rand.Read(idem)
	submitHeaders := map[string]string{"Idempotency-Key": hex.EncodeToString(idem)}
	for k, v := range h {
		submitHeaders[k] = v
	}
	route, payload := "/v1/jobs", any(body)
	if r.Ref != "" {
		form, ctype, err := yue2CoverForm(body, r.Ref)
		if err != nil {
			return nil, err
		}
		route, payload, submitHeaders["Content-Type"] = "/v1/covers", form, ctype
	}
	cover := r.Ref != ""
	var job yue2Job
	for {
		err := services.Do(ctx, "POST", base+route, submitHeaders, payload, &job)
		var he *services.HTTPError
		if cover && errors.As(err, &he) && (he.Status == 501 || he.Status == 404) {
			// cav 1.3's wrapper answered 501; a server without the route, 404.
			return nil, fmt.Errorf("%w: the YuE2 server has no covers route (%s)", ErrNoCover, clip(detail(he.Body), 200))
		}
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
			if cover && ready.Status == "ready" {
				// The model is loaded, so the 503 means covers are off, and waiting does not help.
				return nil, fmt.Errorf("%w: %s", ErrNoCover, clip(detail(he.Body), 300))
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
		if job.Error != nil && job.Error.Code == "transcription_failed" {
			return nil, fmt.Errorf("%w: %s", ErrNoCover, clip(msg, 300))
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
	s := seed.Int64()
	tr := &Track{Data: data, Ext: ".flac", Provider: "yue2", Model: model, Seed: &s, Cover: cover}
	if job.Result != nil {
		tr.Notes = job.Result.notes(r.Seconds, sung, seed.Int64())
		if cv := job.Result.Cover; cv != nil {
			n := fmt.Sprintf("a cover of %s: SheetSage2 transcribed its melody on %s in %.0f s", r.Ref, cmp.Or(cv.Device, "the server"), cv.Seconds)
			if cv.ScoreSeconds > 0 {
				n += fmt.Sprintf(" (a %.1f s score at %g BPM)", cv.ScoreSeconds, cv.BPM)
			}
			tr.Notes = append(tr.Notes, n+"; the singer and the arrangement are not copied")
			for _, w := range cv.Warnings {
				tr.Notes = append(tr.Notes, "SheetSage2: "+w)
			}
		}
	}
	return tr, nil
}

// ErrNoCover means the YuE2 server could not make a cover: covers are off on it, or
// SheetSage2 could not transcribe the track.
var ErrNoCover = errors.New("yue2: no cover")

// yue2CoverLimit is the size of a cover form that YuE2-Turbo and cav's wrapper accept.
const yue2CoverLimit = 40 << 20

// yue2CoverForm builds the multipart form for POST /v1/covers from a job body.
func yue2CoverForm(body map[string]any, ref string) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("style", body["style"].(string))
	w.WriteField("lyrics", body["lyrics"].(string))
	w.WriteField("seed", strconv.FormatInt(body["seed"].(int64), 10))
	if body["instrumental"] == true {
		w.WriteField("instrumental", "true")
	}
	if sec, ok := body["seconds"].(float64); ok {
		w.WriteField("seconds", strconv.FormatFloat(sec, 'f', -1, 64))
	}
	if err := attach(w, "audio", ref); err != nil {
		return nil, "", err
	}
	w.Close()
	if buf.Len() > yue2CoverLimit {
		return nil, "", fmt.Errorf("yue2: %s makes a %d MB upload; covers take at most 40 MiB", filepath.Base(ref), buf.Len()>>20)
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// detail is the "detail" message of a FastAPI-style error body, or the body itself.
func detail(body string) string {
	var d struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal([]byte(body), &d) == nil && d.Detail != "" {
		return d.Detail
	}
	return body
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
	Cover *struct { // cav's wrapper: how SheetSage2 transcribed a cover's track
		Device       string   `json:"device"`
		Seconds      float64  `json:"transcription_seconds"`
		ScoreSeconds float64  `json:"score_seconds"`
		BPM          float64  `json:"bpm"`
		Warnings     []string `json:"warnings"`
	} `json:"cover"`
}

// notes says how long the take came out against the request, and which part of the
// generation stopped at its token limit.
func (res *yue2Result) notes(want float64, sung bool, seed int64) []string {
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
		if sung {
			length += fmt.Sprintf("; you asked for %.1f s, but the lyrics set the length of a song: write fewer or shorter sections, or trim or fade it", want)
		} else {
			length += fmt.Sprintf("; you asked for %.1f s: YuE2 cuts only at a phrase, so trim or fade it", want)
		}
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
