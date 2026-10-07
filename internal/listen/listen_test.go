package listen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

func TestParseLoudnessFromRealFFmpegOutput(t *testing.T) {
	// Recorded from ffmpeg 7 with ebur128=peak=true on a 9 s test tone.
	b, err := os.ReadFile("testdata/ebur128.txt")
	if err != nil {
		t.Fatal(err)
	}
	l, err := parseLoudness(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if l.Integrated != -8.6 || l.Range != 0 || l.TruePeak != 1.6 {
		t.Fatalf("got %+v", l)
	}
	adv := strings.Join(l.Advice(), "\n")
	for _, want := range []string{"loud", "true peak", "loudness range"} {
		if !strings.Contains(adv, want) {
			t.Errorf("advice lacks %q: %s", want, adv)
		}
	}
	if len((Loudness{Integrated: -14, TruePeak: -1.5, Range: 6}).Advice()) != 0 {
		t.Error("a track at -14 LUFS, -1.5 dBTP, 6 LU needs no advice")
	}
}

type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Host", req.URL.Host)
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestCritiqueRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1beta/models":
			w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-flash"},{"name":"models/gemini-3-flash"},{"name":"models/gemini-3-flash-image"}]}`))
		case r.URL.Path == "/v1beta/models/gemini-3-flash:generateContent":
			var body struct {
				Contents []struct {
					Parts []map[string]any `json:"parts"`
				} `json:"contents"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Contents[0].Parts[0]["inline_data"] == nil {
				t.Error("gemini: no audio part")
			}
			w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"The drop lands at 0:04.0."}]}}]}`))
		case r.URL.Path == "/v1/chat/completions":
			var body struct {
				Messages []struct {
					Content []map[string]any `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Messages[0].Content[0]["type"] != "input_audio" {
				t.Errorf("qwen: first part %v", body.Messages[0].Content[0])
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"Warm pads, steady kick."}}]}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { services.HTTPClient = old }()

	text, err := Critique(context.Background(), &services.Config{}, &services.Choice{Service: "gemini", Key: "k"}, []byte("mp3"), "p")
	if err != nil || !strings.Contains(text, "0:04.0") {
		t.Fatalf("gemini: %q %v", text, err)
	}
	c := &services.Config{Endpoints: map[string]services.Endpoint{"qwen-omni": {BaseURL: "https://qwen.example/v1"}}}
	text, err = Critique(context.Background(), c, &services.Choice{Service: "qwen-omni"}, []byte("mp3"), "p")
	if err != nil || text != "Warm pads, steady kick." {
		t.Fatalf("qwen: %q %v", text, err)
	}
}
