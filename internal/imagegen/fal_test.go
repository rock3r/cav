package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/services"
)

// The fixtures in testdata/fal_*.json are the answers shown in fal's queue docs and model
// pages (read 2026-10-08), trimmed. They are not recordings of live calls.

func fixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// falServer answers like fal's queue: submit, one IN_PROGRESS status, then COMPLETED, then
// the result, then the image on fal's CDN. It records each submitted body by endpoint.
func falServer(t *testing.T, status string) map[string]map[string]any {
	bodies := map[string]map[string]any{}
	polls := 0
	old := FalPoll
	FalPoll = time.Millisecond
	t.Cleanup(func() { FalPoll = old })
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		host := r.Header.Get("X-Host")
		if host == "queue.fal.run" && r.Header.Get("Authorization") != "Key fal-key" {
			t.Errorf("%s %s: auth header %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		switch {
		case host == "queue.fal.run" && r.Method == "POST":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			bodies[strings.TrimPrefix(r.URL.Path, "/")] = body
			w.Write(fixture(t, "fal_submit.json"))
		case host == "queue.fal.run" && strings.HasSuffix(r.URL.Path, "/status"):
			polls++
			if polls == 1 {
				w.Write(fixture(t, "fal_status_in_progress.json"))
				return
			}
			polls = 0
			w.Write(fixture(t, status))
		case host == "queue.fal.run" && r.URL.Path == "/fal-ai/flux-2/requests/764cabcf-b745-4b3e-ae38-1200304cf45b":
			w.Write(fixture(t, "fal_result.json"))
		case host == "v3b.fal.media":
			w.Header().Set("Content-Type", "image/png")
			w.Write(tinyPNG())
		default:
			t.Errorf("unexpected %s %s%s", r.Method, host, r.URL.Path)
		}
	})
	return bodies
}

func TestFalTextToImageQueue(t *testing.T) {
	bodies := falServer(t, "fal_status_completed.json")
	im, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "fal", Key: "fal-key"}, Request{Prompt: "a frame"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(im.Data, tinyPNG()) || im.MIME != "image/png" || im.Model != "fal-ai/flux-2" || im.Provider != "fal" {
		t.Fatalf("got %+v", im)
	}
	body := bodies["fal-ai/flux-2"]
	if body["prompt"] != "a frame" || body["image_size"] != "landscape_16_9" || body["num_images"] != float64(1) || body["output_format"] != "png" {
		t.Fatalf("flux-2 body: %v", body)
	}
	if _, ok := body["image_urls"]; ok {
		t.Fatal("no refs, so no image_urls")
	}
}

func TestFalRefsGoToTheEditModelAsDataURIs(t *testing.T) {
	bodies := falServer(t, "fal_status_completed.json")
	ref := refFile(t)
	for _, model := range []string{"fal-ai/flux-2", "fal-ai/bytedance/seedream/v4.5/text-to-image", "xai/grok-imagine-image", "fal-ai/ideogram/v3"} {
		c := &services.Config{Models: map[string]string{"fal": model}}
		if _, err := Generate(context.Background(), c, &services.Choice{Service: "fal", Key: "fal-key"}, Request{Prompt: "x", Aspect: "9:16", Refs: []string{ref}}); err != nil {
			t.Fatalf("%s: %v", model, err)
		}
	}
	check := func(endpoint string, want map[string]any) {
		t.Helper()
		body, ok := bodies[endpoint]
		if !ok {
			t.Fatalf("%s was not called (called: %v)", endpoint, keys(bodies))
		}
		urls, _ := body["image_urls"].([]any)
		if len(urls) != 1 || !strings.HasPrefix(urls[0].(string), "data:image/png;base64,") {
			t.Errorf("%s image_urls: %v", endpoint, body["image_urls"])
		}
		for k, v := range want {
			got, _ := json.Marshal(body[k])
			exp, _ := json.Marshal(v)
			if string(got) != string(exp) {
				t.Errorf("%s %s: got %s, want %s", endpoint, k, got, exp)
			}
		}
	}
	check("fal-ai/flux-2/edit", map[string]any{"image_size": "portrait_16_9"})
	// Seedream needs at least 2560x1440 pixels, so it gets an explicit size.
	check("fal-ai/bytedance/seedream/v4.5/edit", map[string]any{"image_size": map[string]int{"width": 1536, "height": 2736}})
	check("xai/grok-imagine-image/edit", map[string]any{"aspect_ratio": "9:16"})
	check("fal-ai/ideogram/v3", map[string]any{"image_size": "portrait_16_9"})
}

func TestFalReportsAFailedRequest(t *testing.T) {
	falServer(t, "fal_status_failed.json")
	_, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "fal", Key: "fal-key"}, Request{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "content_policy_violation") || !strings.Contains(err.Error(), "fal (fal-ai/flux-2)") {
		t.Fatalf("got %v", err)
	}
}

func TestAspectSize(t *testing.T) {
	for aspect, want := range map[string][2]int{"16:9": {1360, 768}, "1:1": {1024, 1024}, "21:9": {1568, 672}, "junk": {1360, 768}} {
		w, h := aspectSize(aspect, 1024*1024)
		if w != want[0] || h != want[1] || w%16 != 0 || h%16 != 0 {
			t.Errorf("%s: got %dx%d, want %dx%d", aspect, w, h, want[0], want[1])
		}
	}
}

func TestGrokAspectUsesTheNearestAcceptedRatio(t *testing.T) {
	for in, want := range map[string]string{"16:9": "16:9", "21:9": "20:9", "4:5": "3:4", "5:4": "4:3", "9:16": "9:16", "junk": "16:9", "3:1": "20:9", "1:1.9": "1:2"} {
		if got := grokAspect(in); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}

func TestFalCapsReferencesPerModel(t *testing.T) {
	bodies := falServer(t, "fal_status_completed.json")
	var notes []string
	old := Warn
	Warn = func(m string) { notes = append(notes, m) }
	t.Cleanup(func() { Warn = old })
	refs := []string{refFile(t), refFile(t), refFile(t), refFile(t), refFile(t)}
	for _, model := range []string{"xai/grok-imagine-image", "fal-ai/flux-2", "fal-ai/bytedance/seedream/v4.5/text-to-image"} {
		c := &services.Config{Models: map[string]string{"fal": model}}
		if _, err := Generate(context.Background(), c, &services.Choice{Service: "fal", Key: "fal-key"}, Request{Prompt: "x", Refs: refs}); err != nil {
			t.Fatalf("%s: %v", model, err)
		}
	}
	for endpoint, want := range map[string]int{"xai/grok-imagine-image/edit": 3, "fal-ai/flux-2/edit": 4, "fal-ai/bytedance/seedream/v4.5/edit": 5} {
		if got := len(bodies[endpoint]["image_urls"].([]any)); got != want {
			t.Errorf("%s: sent %d references, want %d", endpoint, got, want)
		}
	}
	if len(notes) != 2 || !strings.Contains(notes[0], "takes 3 reference images") || !strings.Contains(notes[1], "takes 4") {
		t.Fatalf("notes: %q", notes)
	}
}

func keys(m map[string]map[string]any) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}
