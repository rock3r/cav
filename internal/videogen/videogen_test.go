package videogen

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/services"
)

// These tests check the requests against the Gemini API Veo guide (read 2026-10-08). The
// fixtures in testdata/ follow the guide's answer shapes (operation name, done, and
// response.generateVideoResponse.generatedSamples[].video.uri). They are not recordings of
// live calls.

type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Host", req.URL.Host)
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func fixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func frame(t *testing.T) string {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	p := filepath.Join(t.TempDir(), "s1.png")
	os.WriteFile(p, b.Bytes(), 0o644)
	return p
}

func veoServer(t *testing.T, final string) *map[string]any {
	var body map[string]any
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Host") != "generativelanguage.googleapis.com" || r.Header.Get("x-goog-api-key") != "g-key" {
			t.Errorf("%s %s: host %q, key header %q", r.Method, r.URL.Path, r.Header.Get("X-Host"), r.Header.Get("x-goog-api-key"))
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1beta/models/veo-3.1-lite-generate-preview:predictLongRunning":
			json.NewDecoder(r.Body).Decode(&body)
			w.Write(fixture(t, "veo_start.json"))
		case r.Method == "GET" && r.URL.Path == "/v1beta/models/veo-3.1-lite-generate-preview/operations/op123":
			polls++
			if polls == 1 {
				w.Write(fixture(t, "veo_running.json"))
				return
			}
			w.Write(fixture(t, final))
		case r.Method == "GET" && r.URL.Path == "/v1beta/files/abc123:download":
			w.Header().Set("Content-Type", "video/mp4")
			w.Write([]byte("\x00\x00\x00\x18ftypmp42"))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old, oldPoll := services.HTTPClient, Poll
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	Poll = time.Millisecond
	t.Cleanup(func() { services.HTTPClient, Poll = old, oldPoll })
	return &body
}

func TestVeoImageToVideo(t *testing.T) {
	body := veoServer(t, "veo_done.json")
	cl, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "veo", Key: "g-key"},
		Request{Prompt: "the logo spins in", Image: frame(t), Aspect: "16:9", Seconds: 5.2})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(cl.Data, []byte("ftyp")) || cl.Seconds != 6 || cl.Model != "veo-3.1-lite-generate-preview" || cl.Provider != "veo" {
		t.Fatalf("got %+v", cl)
	}
	var shape struct {
		Instances []struct {
			Prompt string `json:"prompt"`
			Image  struct {
				InlineData struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData"`
			} `json:"image"`
		} `json:"instances"`
		Parameters map[string]any `json:"parameters"`
	}
	b, _ := json.Marshal(*body)
	json.Unmarshal(b, &shape)
	if len(shape.Instances) != 1 || shape.Instances[0].Prompt != "the logo spins in" || shape.Instances[0].Image.InlineData.MimeType != "image/png" || shape.Instances[0].Image.InlineData.Data == "" {
		t.Fatalf("instances: %s", b)
	}
	p := shape.Parameters
	if p["aspectRatio"] != "16:9" || p["durationSeconds"] != "6" || p["resolution"] != "720p" {
		t.Fatalf("parameters: %v", p)
	}
}

func TestVeoOperationError(t *testing.T) {
	veoServer(t, "veo_error.json")
	_, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "veo", Key: "g-key"}, Request{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "could not be submitted") {
		t.Fatalf("got %v", err)
	}
}

func TestDurationAndHighResolution(t *testing.T) {
	for secs, want := range map[float64]int{1: 4, 4: 4, 4.01: 6, 6: 6, 7: 8, 20: 8} {
		if got := Duration(secs); got != want {
			t.Errorf("%g s: got %d, want %d", secs, got, want)
		}
	}
	body := veoServer(t, "veo_done.json")
	if _, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "veo", Key: "g-key"}, Request{Prompt: "x", Seconds: 3, Resolution: "1080p", Aspect: "9:16"}); err != nil {
		t.Fatal(err)
	}
	p := (*body)["parameters"].(map[string]any)
	if p["durationSeconds"] != "8" || p["aspectRatio"] != "9:16" {
		t.Fatalf("1080p must be 8 seconds: %v", p)
	}
}
