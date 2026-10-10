package music

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestElevenLabsPlanAndStableAudio(t *testing.T) {
	var plan map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/music":
			if r.Header.Get("xi-api-key") != "el" || r.URL.Query().Get("output_format") == "" {
				t.Errorf("elevenlabs headers or query")
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			plan, _ = body["composition_plan"].(map[string]any)
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte("ID3fake"))
		case "/v2beta/audio/stable-audio-2/text-to-audio":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("duration") != "17" || r.Header.Get("Accept") != "audio/*" {
				t.Errorf("stability form: duration %q accept %q", r.FormValue("duration"), r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte("ID3fake"))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { services.HTTPClient = old }()

	req := Request{Prompt: "synthwave", Seconds: 16, Sections: []Section{{ID: "act:1", Name: "act:1: intro", Seconds: 4}, {ID: "s2", Name: "s2: drop", Seconds: 12}}}
	tr, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "elevenlabs", Key: "el"}, req)
	if err != nil || string(tr.Data) != "ID3fake" {
		t.Fatalf("elevenlabs: %v", err)
	}
	secs, _ := plan["sections"].([]any)
	if len(secs) != 2 || secs[1].(map[string]any)["duration_ms"].(float64) != 12000 || secs[0].(map[string]any)["section_name"] != "act:1" || secs[1].(map[string]any)["section_name"] != "s2" {
		t.Fatalf("plan sections: %v", plan)
	}
	if _, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "stability", Key: "st"}, Request{Prompt: "x", Seconds: 16.2}); err != nil {
		t.Fatalf("stability: %v", err)
	}
}

func TestSoundEffectRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/v1/sound-generation" || body["text"] != "whoosh" || body["duration_seconds"].(float64) != 30 || body["loop"] != true {
			t.Errorf("request %s %v", r.URL.Path, body)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("ID3"))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { services.HTTPClient = old }()
	if _, err := SoundEffect(context.Background(), "k", "whoosh", 45, true); err != nil {
		t.Fatal(err)
	}
}

func TestACEStepSubmitsQueriesAndDownloads(t *testing.T) {
	queries := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release_task":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["bpm"] != float64(112) || body["audio_duration"] != float64(24) || body["thinking"] != true || !strings.Contains(body["prompt"].(string), "s1: open at 0s") {
				t.Errorf("release_task body: %v", body)
			}
			w.Write([]byte(`{"data":{"task_id":"t1","status":"queued"},"code":200,"error":null}`))
		case "/query_result":
			queries++
			status, result := 0, ""
			if queries > 1 {
				status, result = 1, `[{"file":"/v1/audio?path=%2Ftmp%2Fa.wav","status":1,"dit_model":"acestep-v15-turbo"}]`
			}
			b, _ := json.Marshal(map[string]any{"data": []any{map[string]any{"task_id": "t1", "status": status, "result": result}}, "code": 200})
			w.Write(b)
		case "/v1/audio":
			if r.URL.Query().Get("path") != "/tmp/a.wav" {
				t.Errorf("audio path %q", r.URL.Query().Get("path"))
			}
			w.Write([]byte("RIFFwav"))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := &services.Config{Endpoints: map[string]services.Endpoint{"acestep": {BaseURL: srv.URL}}}
	tr, err := Generate(context.Background(), c, &services.Choice{Service: "acestep"}, Request{Prompt: "warm synthwave, 112 BPM", Seconds: 24,
		Sections: []Section{{Name: "s1: open", Seconds: 12}, {Name: "s2: drop", Seconds: 12}}})
	if err != nil || string(tr.Data) != "RIFFwav" || tr.Model != "acestep-v15-turbo" || tr.Ext != ".wav" {
		t.Fatalf("got %+v, %v", tr, err)
	}
}

// The Lyria fixture follows the Gemini API music guide (read 2026-10-08): audio is the
// base64 data of an "audio" block in a "model_output" step. The other fields are
// illustrative; it is not a recording of a live call.
func TestLyriaRequestAndAnswer(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1beta/interactions" || r.Header.Get("x-goog-api-key") != "g-key" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&body)
		b, _ := os.ReadFile("testdata/lyria_interaction.json")
		w.Write(b)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { services.HTTPClient = old }()

	req := Request{Prompt: "warm synthwave, 112 BPM", Seconds: 16, Sections: []Section{{Name: "s1: intro", Seconds: 4, Styles: []string{"sparse"}}, {Name: "s2: drop", Seconds: 72}}}
	tr, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "lyria", Key: "g-key"}, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(tr.Data), "RIFF") || tr.Ext != ".wav" || tr.Model != "lyria-3.5" || tr.Provider != "lyria" {
		t.Fatalf("got %+v", tr)
	}
	rf, _ := body["response_format"].(map[string]any)
	if body["model"] != "lyria-3.5" || rf["type"] != "audio" || rf["mime_type"] != "audio/wav" {
		t.Fatalf("body: %v", body)
	}
	in, _ := body["input"].(string)
	for _, want := range []string{"warm synthwave, 112 BPM", "Exactly 1:16 long", "Instrumental only, no vocals.", "\n[0:00 - 0:04] s1: intro: sparse", "\n[0:04 - 1:16] s2: drop"} {
		if !strings.Contains(in, want) {
			t.Errorf("input lacks %q:\n%s", want, in)
		}
	}
}

func TestLyriaClipModelSendsNoResponseFormat(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(`{"steps":[{"type":"model_output","content":[{"type":"text","text":"I can only make instrumental music."}]}]}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { services.HTTPClient = old }()
	c := &services.Config{Models: map[string]string{"lyria": "lyria-3-clip-preview"}}
	_, err := Generate(context.Background(), c, &services.Choice{Service: "lyria", Key: "k"}, Request{Prompt: "x", Seconds: 30})
	if err == nil || !strings.Contains(err.Error(), "the model said: I can only") {
		t.Fatalf("a text-only answer must be reported: %v", err)
	}
	if _, ok := body["response_format"]; ok || body["model"] != "lyria-3-clip-preview" {
		t.Fatalf("clip body: %v", body)
	}
}

func TestReferenceTrackGoesUpAsAudio(t *testing.T) {
	ref := filepath.Join(t.TempDir(), "pixel-pop-a.mp3")
	os.WriteFile(ref, []byte("ID3ref"), 0o644)
	upload := func(r *http.Request, field string) string {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		f, h, err := r.FormFile(field)
		if err != nil {
			t.Fatalf("no %s file: %v", field, err)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		return h.Filename + ":" + string(b)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2beta/audio/stable-audio-2/audio-to-audio":
			if got := upload(r, "audio"); got != "pixel-pop-a.mp3:ID3ref" {
				t.Errorf("stability audio %q", got)
			}
			if r.FormValue("strength") != "0.70" || r.FormValue("duration") != "20" || r.FormValue("prompt") == "" {
				t.Errorf("stability form: strength %q duration %q", r.FormValue("strength"), r.FormValue("duration"))
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte("ID3var"))
		case "/release_task":
			if got := upload(r, "reference_audio"); got != "pixel-pop-a.mp3:ID3ref" {
				t.Errorf("acestep reference %q", got)
			}
			if r.FormValue("thinking") != "true" || r.FormValue("audio_duration") != "20" || r.FormValue("lyrics") != "[Instrumental]" {
				t.Errorf("acestep form: %v", r.MultipartForm.Value)
			}
			w.Write([]byte(`{"data":{"task_id":"t1"},"code":200,"error":null}`))
		case "/query_result":
			w.Write([]byte(`{"data":[{"task_id":"t1","status":1,"result":"[{\"file\":\"/v1/audio?path=a.wav\"}]"}],"code":200}`))
		case "/v1/audio":
			w.Write([]byte("RIFFvar"))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { services.HTTPClient = old }()

	req := Request{Prompt: "toy piano pop", Seconds: 20, Ref: ref, Keep: 0.3}
	if tr, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "stability", Key: "st"}, req); err != nil || string(tr.Data) != "ID3var" {
		t.Fatalf("stability: %+v %v", tr, err)
	}
	c := &services.Config{Endpoints: map[string]services.Endpoint{"acestep": {BaseURL: srv.URL}}}
	if tr, err := Generate(context.Background(), c, &services.Choice{Service: "acestep"}, req); err != nil || string(tr.Data) != "RIFFvar" {
		t.Fatalf("acestep: %+v %v", tr, err)
	}
}

func TestReferenceTrackRefusedWhereAudioCannotGo(t *testing.T) {
	for _, s := range []string{"lyria", "elevenlabs"} {
		if SendsAudio(s) {
			t.Errorf("%s does not take audio", s)
		}
		_, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: s, Key: "k"}, Request{Prompt: "x", Seconds: 10, Ref: "a.mp3"})
		if err == nil || !strings.Contains(err.Error(), "cannot take a reference track") {
			t.Errorf("%s: %v", s, err)
		}
	}
}

func TestStableAudioStrengthStaysAboveTheMinimum(t *testing.T) {
	ref := filepath.Join(t.TempDir(), "ref.mp3")
	os.WriteFile(ref, []byte("ID3ref"), 0o644)
	var strength string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		strength = r.FormValue("strength")
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("ID3"))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { services.HTTPClient = old }()
	if _, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "stability", Key: "k"}, Request{Prompt: "x", Seconds: 10, Ref: ref, Keep: 1}); err != nil || strength != "0.01" {
		t.Fatalf("--keep 1 must send strength 0.01, sent %q (%v)", strength, err)
	}
}
