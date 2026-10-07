package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
