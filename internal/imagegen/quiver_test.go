package imagegen

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

func TestQuiverGeneratesAnSVG(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/svgs/generations" || r.Header.Get("Authorization") != "Bearer q-key" {
			t.Errorf("%s %s auth %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "arrow-2" || body["temperature"] != float64(0) || body["n"] != float64(1) {
			t.Errorf("body model/temperature/n: %v %v %v", body["model"], body["temperature"], body["n"])
		}
		vb := body["attributes"].(map[string]any)["viewBox"].(map[string]any)
		if vb["width"] != float64(1080) || vb["height"] != float64(1080) {
			t.Errorf("1:1 viewBox: %v", vb)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"mime_type":"image/svg+xml","svg":"<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 64 64\"><circle cx=\"32\" cy=\"32\" r=\"8\"/></svg>"}]}`)
	})
	im, err := Generate(context.Background(), &services.Config{}, &services.Choice{Service: "quiverai", Key: "q-key"},
		Request{Prompt: "a blue circle", Aspect: "1:1", Vector: true})
	if err != nil {
		t.Fatal(err)
	}
	if im.Provider != "quiverai" || im.Model != "arrow-2" || im.MIME != "image/svg+xml" || !strings.Contains(string(im.Data), "<circle") {
		t.Fatalf("%+v %s", im, im.Data)
	}
}

func TestQuiverViewBoxKeepsAPortraitRatio(t *testing.T) {
	got := quiverViewBox("2:3")
	w, h := got["width"].(float64), got["height"].(float64)
	if w >= h || math.Abs(w/h-2.0/3) > 0.01 {
		t.Fatalf("2:3 became %v, a portrait request must stay portrait", got)
	}
}

func TestQuiverVectorizeSendsTheRaster(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/svgs/vectorizations" {
			t.Errorf("path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		img, _ := body["image"].(map[string]any)
		if body["model"] != "arrow-2" || img["base64"] == "" {
			t.Errorf("model %v image %v", body["model"], img)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"svg":"<svg xmlns=\"http://www.w3.org/2000/svg\"><path d=\"M0 0\"/></svg>"}]}`)
	})
	im, err := Vectorize(context.Background(), &services.Choice{Service: "quiverai", Key: "q-key"}, writeSquare(t))
	if err != nil {
		t.Fatal(err)
	}
	if im.Provider != "quiverai" || !strings.Contains(string(im.Data), "<path") || im.MIME != "image/svg+xml" {
		t.Fatalf("%+v", im)
	}
}

func TestQuiverRefusesAnAnswerWithoutAnSVG(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"svg":"not markup"}]}`)
	})
	if _, err := Vectorize(context.Background(), &services.Choice{Service: "quiverai", Key: "q-key"}, writeSquare(t)); err == nil || !strings.Contains(err.Error(), "no SVG") {
		t.Fatal(err)
	}
}
