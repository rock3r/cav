package listen

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
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

// sameJSON fails when body is not the same JSON as testdata/name.
func sameJSON(t *testing.T, body []byte, name string) {
	t.Helper()
	var got, want any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Errorf("request is not JSON: %v", err)
		return
	}
	if err := json.Unmarshal(fixture(t, name), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request differs from %s, which the real service accepted:\n%s", name, body)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The prompt that the real run on 2026-10-13 (#11) sent with --board and --brief.
func TestPromptMatchesTheRealRequest(t *testing.T) {
	var req struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(fixture(t, "gemini_ears_request.json"), &req); err != nil {
		t.Fatal(err)
	}
	facts := "length 16.01 s; tempo 119.8 BPM; 2 sections starting at 0.0s (quiet), 4.0s (loud); loudness -12.9 LUFS integrated, true peak -1.5 dBFS, range 10.1 LU\n" +
		"The picture cuts to: intro at 0.00s: logo builds from shapes, calm; drop at 4.00s: logo spins in, energy rises; end at 12.00s: logo settles, warm ending"
	if got := Prompt("calm intro, energy rises at the cut to the drop, warm ending", facts); got != req.Contents[0].Parts[1].Text {
		t.Fatalf("got\n%s", got)
	}
}

// testdata/gemini_ears.json and qwen_openrouter_chat.json are real answers from 2026-10-13
// (#11): Gemini (gemini-3.8-flash) with a thoughtSignature next to its text, and Qwen3-Omni
// through OpenRouter with a reasoning field next to its content. The *_request.json
// fixtures are the requests they accepted; the audio in them is base64 of "mp3". The
// recording kept only the length of Qwen's answer, so a stand-in text replaces it.
func TestCritiqueRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1beta/models":
			w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-flash"},{"name":"models/gemini-3.8-flash"},{"name":"models/gemini-3.1-flash-image"}]}`))
		case r.URL.Path == "/v1beta/models/gemini-3.8-flash:generateContent":
			if r.Header.Get("x-goog-api-key") != "k" {
				t.Error("gemini: no key header")
			}
			b, _ := io.ReadAll(r.Body)
			sameJSON(t, b, "gemini_ears_request.json")
			w.Write(fixture(t, "gemini_ears.json"))
		case r.Header.Get("X-Host") == "openrouter.ai" && r.URL.Path == "/api/v1/chat/completions":
			if r.Header.Get("Authorization") != "Bearer or-key" {
				t.Errorf("qwen: auth header %q", r.Header.Get("Authorization"))
			}
			b, _ := io.ReadAll(r.Body)
			sameJSON(t, b, "qwen_openrouter_request.json")
			w.Write(fixture(t, "qwen_openrouter_chat.json"))
		default:
			t.Errorf("unexpected %s %s%s", r.Method, r.Header.Get("X-Host"), r.URL.Path)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { services.HTTPClient = old }()

	var greq struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	json.Unmarshal(fixture(t, "gemini_ears_request.json"), &greq)
	geminiPrompt := greq.Contents[0].Parts[1].Text
	var qreq struct {
		Messages []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal(fixture(t, "qwen_openrouter_request.json"), &qreq)
	qwenPrompt := qreq.Messages[0].Content[1].Text

	text, err := Critique(context.Background(), &services.Config{}, &services.Choice{Service: "gemini", Key: "k"}, []byte("mp3"), geminiPrompt)
	if err != nil || !strings.HasPrefix(text, "**1. What you hear**") || !strings.Contains(text, "Enhance the 0:04.0 transition") {
		t.Fatalf("gemini: %q %v", text, err)
	}
	c := &services.Config{Endpoints: map[string]services.Endpoint{"qwen-omni": {BaseURL: "https://openrouter.ai/api/v1/", Model: "qwen/qwen3.8-omni-flash"}}}
	text, err = Critique(context.Background(), c, &services.Choice{Service: "qwen-omni", Key: "or-key"}, []byte("mp3"), qwenPrompt)
	if err != nil || !strings.HasPrefix(text, "1. What you hear: progressive house") || strings.Contains(text, "The user wants") {
		t.Fatalf("qwen: %q %v", text, err)
	}
}
