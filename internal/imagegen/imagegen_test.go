package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
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

// These tests check that each request matches the vendor documentation (read 2026-10-08)
// and that each documented answer shape is read. They do not reach the live services.

type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Host", req.URL.Host)
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func tinyPNG() []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	return b.Bytes()
}

func serve(t *testing.T, h http.HandlerFunc) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: redirect{u}}
	t.Cleanup(func() { services.HTTPClient = old })
}

func refFile(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "ref.png")
	os.WriteFile(p, tinyPNG(), 0o644)
	return p
}

func TestGeminiRequestAndAnswer(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(tinyPNG())
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Host") != "generativelanguage.googleapis.com" || r.URL.Path != "/v1beta/models/gemini-3.1-flash-image:generateContent" || r.Header.Get("x-goog-api-key") != "g-key" {
			t.Errorf("unexpected request %s %s", r.Header.Get("X-Host"), r.URL.Path)
		}
		var body struct {
			Contents []struct {
				Parts []map[string]any `json:"parts"`
			} `json:"contents"`
			GenerationConfig struct {
				ResponseModalities []string          `json:"responseModalities"`
				ImageConfig        map[string]string `json:"imageConfig"`
			} `json:"generationConfig"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		parts := body.Contents[0].Parts
		if len(parts) != 2 || parts[0]["text"] == nil || parts[1]["inline_data"] == nil {
			t.Errorf("parts: %v", parts)
		}
		if body.GenerationConfig.ImageConfig["aspectRatio"] != "16:9" || body.GenerationConfig.ResponseModalities[0] != "IMAGE" {
			t.Errorf("config: %+v", body.GenerationConfig)
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"here"},{"inlineData":{"mimeType":"image/png","data":"` + b64 + `"}}]}}]}`))
	})
	im, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "gemini", Key: "g-key"}, Request{Prompt: "a frame", Refs: []string{refFile(t)}})
	if err != nil || im.MIME != "image/png" || !bytes.Equal(im.Data, tinyPNG()) || im.Model != "gemini-3.1-flash-image" {
		t.Fatalf("got %+v, %v", im, err)
	}
}

func TestOpenAIGenerationsAndEdits(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(tinyPNG())
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["background"] != "transparent" || body["size"] != "1536x864" || body["model"] != "gpt-image-2.5-flare" {
				t.Errorf("generations body: %v", body)
			}
		case "/v1/images/edits":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if len(r.MultipartForm.File["image[]"]) != 1 || r.FormValue("prompt") == "" {
				t.Errorf("edits form: %v", r.MultipartForm.Value)
			}
		default:
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"b64_json":"` + b64 + `"}]}`))
	})
	ch := &services.Choice{Service: "openai", Key: "o-key"}
	if _, err := Generate(context.Background(), &services.Config{}, ch, Request{Prompt: "logo", Alpha: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(context.Background(), &services.Config{}, ch, Request{Prompt: "logo", Refs: []string{refFile(t)}}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRouterAndRecraft(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(tinyPNG())
	var removed bool
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-Host") + r.URL.Path {
		case "openrouter.ai/api/v1/images":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			refs, _ := body["input_references"].([]any)
			if body["aspect_ratio"] != "1:1" || len(refs) != 1 {
				t.Errorf("openrouter body: %v", body)
			}
			w.Write([]byte(`{"data":[{"b64_json":"` + b64 + `","media_type":"image/png"}]}`))
		case "external.api.recraft.ai/v1/images/generations":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["response_format"] != "b64_json" {
				t.Errorf("recraft body: %v", body)
			}
			w.Write([]byte(`{"data":[{"b64_json":"` + b64 + `"}]}`))
		case "external.api.recraft.ai/v1/images/removeBackground":
			removed = true
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `name="image"`) {
				t.Error("removeBackground did not send the image field")
			}
			w.Write([]byte(`{"image":{"b64_json":"` + b64 + `"}}`))
		default:
			t.Errorf("unexpected %s%s", r.Header.Get("X-Host"), r.URL.Path)
		}
	})
	if _, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "openrouter", Key: "k"}, Request{Prompt: "x", Aspect: "1:1", Refs: []string{refFile(t)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "recraft", Key: "k"}, Request{Prompt: "x", Alpha: true}); err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("--alpha with Recraft must remove the background")
	}
}

func TestFindImageReportsTextAnswers(t *testing.T) {
	_, err := findImage(context.Background(), []byte(`{"candidates":[{"content":{"parts":[{"text":"I cannot draw that"}]}}]}`))
	if err == nil || !strings.Contains(err.Error(), "I cannot draw that") {
		t.Fatalf("got %v", err)
	}
}

func TestGreyboxCard(t *testing.T) {
	data, err := Greybox(Card{Width: 640, Height: 360, Title: "s1", Lines: []string{"a long line of words that wraps over more than one row"}, Footer: "beats 0-4", Palette: []string{"#0b0d12", "#f5f7fa", "#ff4d2e"}})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 640 {
		t.Fatalf("decode: %v", err)
	}
	r, g, b, _ := img.At(1, 1).RGBA()
	if r>>8 != 0x0b || g>>8 != 0x0d || b>>8 != 0x12 {
		t.Fatalf("background is not the palette's first colour")
	}
}
