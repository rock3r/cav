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
	"maps"
	"math"
	"math/big"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rock3r/cav/internal/services"
)

// yue2 makes a take on a YuE2 server: cav's wrapper (tools/yue2-server) or YuE2-Turbo. Both speak the job API of YuE2-Turbo's src/yue2/service.py (read at commit
// 7c88813 on 2026-10-11): POST /v1/jobs, then GET /v1/jobs/{id} until the status is
// terminal, then GET /v1/jobs/{id}/audio (FLAC).
//
// With lyrics, cav sends a plain job that both servers take: YuE2 writes a song and sings
// them. A song gets no length cut: the score has the vocal notes but not the syllables, so
// a cut could stop a sung line in the middle, and the lyrics after the cut would still be
// in the request. The lyrics set its length.
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
//
// With a reference track (Request.Ref), cav makes a cover: it uploads the track to POST
// /v1/covers, a multipart form of at most 40 MiB with the fields of YuE2-Turbo's covers route
// (audio, style, lyrics, seed; README and service.py read on 2026-10-11). SheetSage2 on the
// server transcribes the track's melody, without chords, and YuE2 generates from it with
// cot=melody. It does not copy the singer. With lyrics, YuE2 sings them to the melody.
// Without lyrics, cav also sends "instrumental" and "seconds" to its wrapper, so the wrapper
// moves the melody to an instrument. With Turbo, the cover job then takes the place of the
// planning job: cav moves the transcribed melody to the Ins voice itself. When the server cannot transcribe, the error is ErrNoCover and the caller
// describes the track in words instead.
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
	sung := strings.TrimSpace(r.Lyrics) != ""
	if !sung && !slices.Contains(extensions, "instrumental") {
		return yue2Turbo(ctx, base, h, ep.Model, seed.Int64(), r)
	}
	// The brief goes in as YuE2's style; with --board it already holds the tempo.
	body := map[string]any{"style": strings.TrimSpace(r.Prompt), "lyrics": yue2Sections, "cot": "full",
		"seed": seed.Int64(), "n": 1, "instrumental": true}
	if sung {
		body["lyrics"] = strings.TrimSpace(r.Lyrics)
		delete(body, "instrumental")
	} else if r.Seconds > 0 {
		body["seconds"] = math.Max(5, math.Min(600, r.Seconds))
	}
	sub := yue2Submit{route: "/v1/jobs", payload: body}
	if r.Ref != "" {
		fields := map[string]string{"style": body["style"].(string), "lyrics": body["lyrics"].(string),
			"seed": strconv.FormatInt(seed.Int64(), 10)}
		if body["instrumental"] == true {
			fields["instrumental"] = "true"
		}
		if sec, ok := body["seconds"].(float64); ok {
			fields["seconds"] = strconv.FormatFloat(sec, 'f', -1, 64)
		}
		if sub, err = yue2CoverSubmit(fields, r.Ref); err != nil {
			return nil, err
		}
	}
	job, jobURL, err := yue2Run(ctx, base, h, sub)
	if err != nil {
		return nil, err
	}
	tr, err := yue2Download(ctx, base, h, job, jobURL, ep.Model, seed.Int64(), r.Seconds, sung)
	if err != nil {
		return nil, err
	}
	if r.Ref != "" {
		tr.Cover = true
		if job.Result != nil && job.Result.Cover != nil {
			cv := job.Result.Cover
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

// yue2Submit is one job to submit: a JSON body to /v1/jobs, or a cover's multipart form to
// /v1/covers.
type yue2Submit struct {
	route   string
	payload any
	ctype   string // set for a form
}

func (s yue2Submit) cover() bool { return s.route == "/v1/covers" }

// yue2Turbo makes an instrumental take on a server with no extensions (YuE2-Turbo): one job
// plans the score, cav turns it into an instrumental, a second job generates the audio.
func yue2Turbo(ctx context.Context, base string, h map[string]string, model string, seed int64, r Request) (*Track, error) {
	start := time.Now()
	style := strings.TrimSpace(r.Prompt)
	// Turbo takes a style of up to 2000 characters, and the instrumental style is longer
	// than the prompt: check it now, before the planning job.
	if n := utf8.RuneCountInString(instrumentalStyle(style)); n > yue2MaxStyle {
		return nil, fmt.Errorf("yue2: the prompt is too long for an instrumental on YuE2-Turbo: with the words cav adds it is %d characters, and Turbo takes %d", n, yue2MaxStyle)
	}
	sub := yue2Submit{route: "/v1/jobs", payload: map[string]any{"style": style, "lyrics": yue2Sections,
		"cot": "full", "seed": seed, "n": 1}}
	if r.Ref != "" {
		// A cover: SheetSage2 transcribes the track, and its melody replaces the plan.
		var err error
		sub, err = yue2CoverSubmit(map[string]string{"style": style, "lyrics": yue2Sections,
			"seed": strconv.FormatInt(seed, 10)}, r.Ref)
		if err != nil {
			return nil, err
		}
	}
	plan, planURL, err := yue2Run(ctx, base, h, sub)
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
	job, jobURL, err := yue2Run(ctx, base, h, yue2Submit{route: "/v1/jobs", payload: map[string]any{"style": in.style,
		"lyrics": in.lyrics, "abc": in.abc, "cot": in.cot, "seed": seed, "n": 1}})
	if err != nil {
		return nil, err
	}
	if job.Result != nil {
		job.Result.Instrumental = &yue2Score{NominalSeconds: in.nominal, PlannedSeconds: in.planned, BPM: in.bpm}
		job.Result.Timing.Job = time.Since(start).Seconds()
		job.Result.Timing.TwoJobs = true
	}
	// Turbo does not say which model it loaded (YUE2_MODEL): the endpoint's model names it.
	tr, err := yue2Download(ctx, base, h, job, jobURL, model, seed, r.Seconds, false)
	if err != nil {
		return nil, err
	}
	if r.Ref != "" {
		tr.Cover = true
		tr.Notes = append(tr.Notes, fmt.Sprintf("a cover of %s: SheetSage2 on the server transcribed its melody, and cav moved it to an instrument; the singer and the arrangement are not copied", r.Ref))
	}
	return tr, nil
}

// yue2Ready waits for the server to load the model, then returns the extensions it lists.
func yue2Ready(ctx context.Context, base string) ([]string, error) {
	for {
		status, extensions, err := yue2Health(ctx, base)
		if err != nil || status == "ready" {
			return extensions, err
		}
		// 503 while loading: come back.
		if err := sleep(ctx, yue2Retry); err != nil {
			return nil, err
		}
	}
}

// yue2Health asks /health/ready once. It returns "ready" with the extensions, "loading", or
// an error when the server failed to load the model or did not answer.
func yue2Health(ctx context.Context, base string) (string, []string, error) {
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
			return "", nil, fmt.Errorf("yue2: the server could not load YuE2: %s", clip(cmp.Or(ready.Error, "see the server's log"), 300))
		}
		return "loading", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("yue2: %w", err)
	}
	json.Unmarshal(raw, &ready)
	return "ready", ready.Extensions, nil
}

// yue2Run submits one job, waits for it to finish and returns it with its URL. When ctx
// ends first, it cancels the job.
func yue2Run(ctx context.Context, base string, h map[string]string, sub yue2Submit) (*yue2Job, string, error) {
	// One key per job: a retried submit then finds the same job instead of starting a second.
	idem := make([]byte, 16)
	rand.Read(idem)
	submitHeaders := map[string]string{"Idempotency-Key": hex.EncodeToString(idem)}
	for k, v := range h {
		submitHeaders[k] = v
	}
	if sub.ctype != "" {
		submitHeaders["Content-Type"] = sub.ctype
	}
	var job yue2Job
	for {
		err := services.Do(ctx, "POST", base+sub.route, submitHeaders, sub.payload, &job)
		var he *services.HTTPError
		if sub.cover() && errors.As(err, &he) && (he.Status == 501 || he.Status == 404) {
			// cav 1.3's wrapper answered 501; a server without the route, 404.
			return nil, "", fmt.Errorf("%w: the YuE2 server has no covers route (%s)", ErrNoCover, clip(detail(he.Body), 200))
		}
		if errors.As(err, &he) && he.Status == 503 {
			// 503 means the model is loading, or failed to load, or covers are off.
			status, _, herr := yue2Health(ctx, base)
			if herr != nil {
				return nil, "", herr
			}
			if sub.cover() && status == "ready" {
				// The model is loaded, so the 503 means covers are off, and waiting does not help.
				return nil, "", fmt.Errorf("%w: %s", ErrNoCover, clip(detail(he.Body), 300))
			}
			if err := sleep(ctx, yue2Retry); err != nil {
				return nil, "", err
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
	// stage is the last stage cav saw before the job ended: the server sets "finished" then.
	stage := job.Stage
	for job.Status == "queued" || job.Status == "running" {
		if job.Stage != "" {
			stage = job.Stage
		}
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
		if job.Error != nil && job.Error.Code == "transcription_failed" {
			return nil, "", fmt.Errorf("%w: %s", ErrNoCover, clip(msg, 300))
		}
		// YuE2-Turbo reports a failed transcription as an ordinary failure, with a generic
		// message. A cover job that never got past transcribing failed there.
		if sub.cover() && (stage == "" || stage == "queued" || stage == "claimed" || stage == "transcribing") {
			return nil, "", fmt.Errorf("%w: the cover job failed before it generated anything (%s)", ErrNoCover, clip(msg, 300))
		}
		return nil, "", fmt.Errorf("yue2: the job failed: %s", clip(msg, 300))
	default:
		return nil, "", fmt.Errorf("yue2: the job ended as %q", job.Status)
	}
	return &job, jobURL, nil
}

// yue2Download fetches the audio of a finished job.
// The model is the server's answer, then the given name, then YuE2-3B.
func yue2Download(ctx context.Context, base string, h map[string]string, job *yue2Job, jobURL, model string, seed int64, want float64, sung bool) (*Track, error) {
	audio := jobURL + "/audio"
	if job.Result != nil && job.Result.AudioURL != "" {
		audio = base + job.Result.AudioURL
	}
	data, _, err := services.DoRaw(ctx, "GET", audio, h, nil)
	if err != nil {
		return nil, fmt.Errorf("yue2: downloading the audio: %w", err)
	}
	if job.Result != nil && job.Result.Model != "" {
		model = job.Result.Model
	}
	model = cmp.Or(model, "YuE2-3B")
	tr := &Track{Data: data, Ext: ".flac", Provider: "yue2", Model: model, Seed: &seed}
	if job.Result != nil {
		tr.Notes = job.Result.notes(want, sung, seed)
	}
	return tr, nil
}

// ErrNoCover means the YuE2 server could not make a cover: covers are off on it, or
// SheetSage2 could not transcribe the track.
var ErrNoCover = errors.New("yue2: no cover")

// yue2CoverLimit is the size of a cover form that YuE2-Turbo and cav's wrapper accept.
const yue2CoverLimit = 40 << 20

// yue2CoverSubmit builds the multipart form for POST /v1/covers: the fields, then the track.
func yue2CoverSubmit(fields map[string]string, ref string) (yue2Submit, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		w.WriteField(k, fields[k])
	}
	if err := attach(w, "audio", ref); err != nil {
		return yue2Submit{}, err
	}
	w.Close()
	if buf.Len() > yue2CoverLimit {
		return yue2Submit{}, fmt.Errorf("yue2: %s makes a %d MB upload; covers take at most 40 MiB", filepath.Base(ref), buf.Len()>>20)
	}
	return yue2Submit{route: "/v1/covers", payload: buf.Bytes(), ctype: w.FormDataContentType()}, nil
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

// yue2MaxStyle is the longest style YuE2-Turbo takes (GenerateRequest.style).
const yue2MaxStyle = 2000

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
	Cover        *struct {  // cav's wrapper: how SheetSage2 transcribed a cover's track
		Device       string   `json:"device"`
		Seconds      float64  `json:"transcription_seconds"`
		ScoreSeconds float64  `json:"score_seconds"`
		BPM          float64  `json:"bpm"`
		Warnings     []string `json:"warnings"`
	} `json:"cover"`
}

// yue2Score is the length of the instrumental score: as YuE2 planned it, and as cut.
type yue2Score struct {
	NominalSeconds float64 `json:"nominal_seconds"`
	PlannedSeconds float64 `json:"planned_seconds"`
	BPM            float64 `json:"bpm"`
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
