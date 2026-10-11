package music

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

// fakeTurbo copies the parts of YuE2-Turbo's src/yue2/service.py (read at commit 7c88813 on
// 2026-10-11) that cav uses. Like Turbo, it refuses unknown job fields (pydantic's
// extra="forbid"), wants a Bearer key on every route but the health routes, and has no
// "extensions" in /health/ready. No real Turbo server was tested: it needs Linux, CUDA
// and vLLM.
type fakeTurbo struct {
	t      *testing.T
	key    string
	score  string // the planned score of the first job
	mu     sync.Mutex
	bodies []map[string]any
	trunc  bool // the plan reached the token limit
}

const fakeTurboKey = "turbo-key-0123456789"

func (f *fakeTurbo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		w.Write([]byte(body))
	}
	switch r.URL.Path {
	case "/health/live":
		reply(200, `{"status":"alive"}`)
		return
	case "/health/ready":
		reply(200, `{"status":"ready"}`)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+f.key {
		w.Header().Set("WWW-Authenticate", "Bearer")
		reply(401, `{"detail":"Invalid bearer token"}`)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == "POST" && r.URL.Path == "/v1/jobs":
		var body map[string]any
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			reply(422, `{"detail":"invalid JSON"}`)
			return
		}
		allowed := map[string]bool{"n": true, "style": true, "lyrics": true, "cot": true, "seed": true, "abc": true, "cfg_scale": true}
		for k := range body {
			if !allowed[k] {
				reply(422, `{"detail":[{"type":"extra_forbidden","loc":["body","`+k+`"],"msg":"Extra inputs are not permitted"}]}`)
				return
			}
		}
		for _, k := range []string{"style", "lyrics"} {
			if v, _ := body[k].(string); strings.TrimSpace(v) == "" {
				reply(422, `{"detail":[{"loc":["body","`+k+`"],"msg":"Text must not be blank"}]}`)
				return
			}
		}
		if abc, ok := body["abc"]; ok && (body["cot"] == "off" || strings.TrimSpace(abc.(string)) == "") {
			reply(422, `{"detail":"ABC requires cot=full or melody"}`)
			return
		}
		f.bodies = append(f.bodies, body)
		id := "plan"
		if _, ok := body["abc"]; ok {
			id = "song"
		}
		reply(202, `{"id":"`+id+`","status":"queued","stage":"queued"}`)
	case r.Method == "POST" && r.URL.Path == "/v1/covers":
		// Turbo's covers route: a form with audio, style, lyrics, seed and cfg_scale.
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			reply(422, `{"detail":"not a form"}`)
			return
		}
		body := map[string]any{"cover": true}
		for k, v := range r.MultipartForm.Value {
			if k != "style" && k != "lyrics" && k != "seed" && k != "cfg_scale" {
				f.t.Errorf("cover form field %q: Turbo ignores it", k)
			}
			body[k] = v[0]
		}
		if _, _, err := r.FormFile("audio"); err != nil {
			reply(422, `{"detail":"audio is required"}`)
			return
		}
		f.bodies = append(f.bodies, body)
		reply(202, `{"id":"plan","status":"queued","stage":"queued"}`)
	case r.URL.Path == "/v1/jobs/plan":
		trunc := "false"
		status := "succeeded"
		if f.trunc {
			trunc, status = "true", "truncated"
		}
		reply(200, `{"id":"plan","status":"`+status+`","result":{"audio_url":"/v1/jobs/plan/audio","score_url":"/v1/jobs/plan/score",
			"audio_seconds":57.2,"sample_rate":48000,"truncated":{"abc":`+trunc+`,"semantic":false},"timing":{"e2e_seconds":10}}}`)
	case r.URL.Path == "/v1/jobs/plan/score":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(f.score))
	case r.URL.Path == "/v1/jobs/song":
		reply(200, `{"id":"song","status":"succeeded","result":{"audio_url":"/v1/jobs/song/audio","score_url":"/v1/jobs/song/score",
			"audio_seconds":31.5,"sample_rate":48000,"truncated":{"abc":false,"semantic":false},"timing":{"e2e_seconds":6}}}`)
	case r.URL.Path == "/v1/jobs/song/audio":
		w.Write([]byte("fLaCturbo"))
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		reply(404, `{"detail":"Not found"}`)
	}
}

func TestYuE2TurboPlansThenGeneratesFromTheScore(t *testing.T) {
	f := &fakeTurbo{t: t, key: fakeTurboKey, score: mustRead(t, "planned.abc")}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := &services.Config{Endpoints: map[string]services.Endpoint{"yue2": {BaseURL: srv.URL}}}
	oldPoll, oldRetry := yue2Poll, yue2Retry
	yue2Poll, yue2Retry = 0, 0
	defer func() { yue2Poll, yue2Retry = oldPoll, oldRetry }()
	tr, err := Generate(t.Context(), c, &services.Choice{Service: "yue2", Key: fakeTurboKey}, Request{Prompt: "warm synthwave, 112 BPM", Seconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	if string(tr.Data) != "fLaCturbo" || tr.Model != "YuE2-3B" || tr.Seed == nil {
		t.Fatalf("track %+v", tr)
	}
	if len(f.bodies) != 2 {
		t.Fatalf("%d jobs, want a plan and a song", len(f.bodies))
	}
	plan, song := f.bodies[0], f.bodies[1]
	if plan["style"] != "warm synthwave, 112 BPM" || plan["lyrics"] != yue2Sections || plan["cot"] != "full" || plan["abc"] != nil {
		t.Errorf("plan body %#v", plan)
	}
	if song["seed"] != plan["seed"] || song["abc"] != mustRead(t, "fit-30.abc") || song["cot"] != "full" ||
		song["lyrics"] != "[Intro]\n\n[Verse]\n\n[Outro]\n" || !strings.HasPrefix(song["style"].(string), "Instrumental, warm synthwave") {
		t.Errorf("song body %#v", song)
	}
	if len(tr.Notes) != 1 || !strings.Contains(tr.Notes[0], "31.5 s of audio") || !strings.Contains(tr.Notes[0], "over two jobs") ||
		!strings.Contains(tr.Notes[0], "YuE2 planned 56.0 s at 120 BPM; cav cut the score to 32.0 s") {
		t.Errorf("notes %q", tr.Notes)
	}
}

func TestYuE2TurboRefusesTheWrongKeyATruncatedPlanAndALongPrompt(t *testing.T) {
	f := &fakeTurbo{t: t, key: fakeTurboKey, score: mustRead(t, "planned.abc"), trunc: true}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := &services.Config{Endpoints: map[string]services.Endpoint{"yue2": {BaseURL: srv.URL}}}
	oldPoll := yue2Poll
	yue2Poll = 0
	defer func() { yue2Poll = oldPoll }()
	_, err := Generate(t.Context(), c, &services.Choice{Service: "yue2", Key: "wrong-key-0123456789"}, Request{Prompt: "x", Seconds: 30})
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "wrong-key") {
		t.Fatalf("wrong key: %v", err)
	}
	_, err = Generate(t.Context(), c, &services.Choice{Service: "yue2", Key: fakeTurboKey}, Request{Prompt: "x", Seconds: 30})
	if err == nil || !strings.Contains(err.Error(), "truncated score") {
		t.Fatalf("truncated plan: %v", err)
	}
	// A prompt that fits Turbo but not with the instrumental words fails before any job.
	f.bodies = nil
	_, err = Generate(t.Context(), c, &services.Choice{Service: "yue2", Key: fakeTurboKey}, Request{Prompt: strings.Repeat("é", 1990), Seconds: 30})
	if err == nil || !strings.Contains(err.Error(), "too long") || len(f.bodies) != 0 {
		t.Fatalf("long prompt: %v, %d jobs", err, len(f.bodies))
	}
}

// The fake must refuse what Turbo refuses, or the tests above prove nothing.
func TestFakeTurboRefusesTheWrapperExtensions(t *testing.T) {
	f := &fakeTurbo{t: t, key: fakeTurboKey}
	srv := httptest.NewServer(f)
	defer srv.Close()
	err := services.Do(t.Context(), "POST", srv.URL+"/v1/jobs", map[string]string{"Authorization": "Bearer " + fakeTurboKey},
		map[string]any{"style": "x", "lyrics": "[Verse]", "instrumental": true}, nil)
	var he *services.HTTPError
	if !errors.As(err, &he) || he.Status != 422 || !strings.Contains(he.Body, "extra_forbidden") {
		t.Fatalf("instrumental field: %v", err)
	}
}

// A cover on Turbo: the covers job gives the melody (no chords), and cav moves it to Ins.
// The endpoint's model names the take, since Turbo does not report its model.
func TestYuE2TurboCoverAndModelName(t *testing.T) {
	melody := regexp.MustCompile(`"[A-G][^"]*"`).ReplaceAllString(mustRead(t, "planned.abc"), "")
	f := &fakeTurbo{t: t, key: fakeTurboKey, score: melody}
	srv := httptest.NewServer(f)
	defer srv.Close()
	ref := filepath.Join(t.TempDir(), "theme.mp3")
	os.WriteFile(ref, []byte("ID3reference"), 0o644)
	c := &services.Config{Endpoints: map[string]services.Endpoint{"yue2": {BaseURL: srv.URL, Model: "YuE2-3B-ft"}}}
	oldPoll := yue2Poll
	yue2Poll = 0
	defer func() { yue2Poll = oldPoll }()
	tr, err := Generate(t.Context(), c, &services.Choice{Service: "yue2", Key: fakeTurboKey}, Request{Prompt: "solo cello", Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Cover || tr.Model != "YuE2-3B-ft" || len(f.bodies) != 2 || f.bodies[0]["cover"] != true {
		t.Fatalf("track %+v, jobs %v", tr, f.bodies)
	}
	song := f.bodies[1]
	if song["cot"] != "melody" || strings.Contains(song["abc"].(string), `"G"`) || song["seed"].(json.Number).String() != f.bodies[0]["seed"] {
		t.Errorf("song body %#v", song)
	}
	if !strings.Contains(strings.Join(tr.Notes, "\n"), "a cover of "+ref) {
		t.Errorf("notes %q", tr.Notes)
	}
}
