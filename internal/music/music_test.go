package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	req := Request{Prompt: "synthwave", Seconds: 16, Sections: []Section{{Name: "s1: intro", Seconds: 4}, {Name: "s2: drop", Seconds: 12}}}
	tr, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "elevenlabs", Key: "el"}, req)
	if err != nil || string(tr.Data) != "ID3fake" {
		t.Fatalf("elevenlabs: %v", err)
	}
	secs, _ := plan["sections"].([]any)
	if len(secs) != 2 || secs[1].(map[string]any)["duration_ms"].(float64) != 12000 {
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
