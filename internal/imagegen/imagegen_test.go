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
	"reflect"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

// These tests do not reach the live services. Gemini, OpenAI, OpenRouter and Recraft are
// checked against real answers from 2026-10-13 (#11); fal against its documentation.

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

// The fixtures named below are real answers from 2026-10-13 (#11), when every paid service
// was called once with real keys. The *_request.json fixtures are the requests those
// services accepted. Only the base64 images were replaced with tiny ones of the same type
// (Gemini sends JPEG, Recraft's cutout WebP, its vectors SVG); thoughtSignature lost its
// value too.

// sameJSON fails when body is not the same JSON as testdata/name, a request that the real
// service accepted.
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

// answerData is the base64 image in a fixture, decoded.
func answerData(t *testing.T, name string, path ...any) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(fixture(t, name), &v); err != nil {
		t.Fatal(err)
	}
	for _, p := range path {
		switch k := p.(type) {
		case string:
			v = v.(map[string]any)[k]
		case int:
			v = v.([]any)[k]
		}
	}
	b, err := base64.StdEncoding.DecodeString(v.(string))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const foxPrompt = "a flat geometric fox head logo, orange and white on a plain light grey background"

// testdata/gemini_image.json: Gemini answers JPEG, next to a thoughtSignature.
func TestGeminiRequestAndAnswer(t *testing.T) {
	refs := false
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Host") != "generativelanguage.googleapis.com" || r.URL.Path != "/v1beta/models/gemini-3.1-flash-image:generateContent" || r.Header.Get("x-goog-api-key") != "g-key" {
			t.Errorf("unexpected request %s %s", r.Header.Get("X-Host"), r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if !refs {
			sameJSON(t, b, "gemini_image_request.json")
		} else {
			var body struct {
				Contents []struct {
					Parts []map[string]any `json:"parts"`
				} `json:"contents"`
			}
			json.Unmarshal(b, &body)
			parts := body.Contents[0].Parts
			if len(parts) != 2 || parts[0]["text"] == nil || parts[1]["inline_data"] == nil {
				t.Errorf("parts: %v", parts)
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.Write(fixture(t, "gemini_image.json"))
	})
	ch := &services.Choice{Service: "gemini", Key: "g-key"}
	im, err := Generate(context.Background(), &services.Config{}, ch, Request{Prompt: foxPrompt})
	want := answerData(t, "gemini_image.json", "candidates", 0, "content", "parts", 0, "inlineData", "data")
	if err != nil || im.MIME != "image/jpeg" || im.Ext() != ".jpg" || !bytes.Equal(im.Data, want) || im.Model != "gemini-3.1-flash-image" {
		t.Fatalf("got %+v, %v", im, err)
	}
	refs = true
	if _, err := Generate(context.Background(), &services.Config{}, ch, Request{Prompt: "a frame", Refs: []string{refFile(t)}}); err != nil {
		t.Fatal(err)
	}
}

// testdata/openai_generations.json (with --alpha), openai_edits.json (with --ref) and
// openai_edits_octet_stream.json: what OpenAI said when cav sent a reference as
// application/octet-stream. This fake says the same, so the test fails if cav does again.
func TestOpenAIGenerationsAndEdits(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer o-key" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/images/generations":
			b, _ := io.ReadAll(r.Body)
			sameJSON(t, b, "openai_generations_request.json")
			w.Write(fixture(t, "openai_generations.json"))
		case "/v1/images/edits":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			files := r.MultipartForm.File["image[]"]
			if len(files) != 1 || r.FormValue("prompt") != foxPrompt || r.FormValue("model") != "gpt-image-2.5-flare" ||
				r.FormValue("size") != "1536x864" || r.FormValue("n") != "1" || r.FormValue("output_format") != "png" {
				t.Errorf("edits form: %v", r.MultipartForm.Value)
			}
			if len(files) == 1 && files[0].Header.Get("Content-Type") != "image/png" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(fixture(t, "openai_edits_octet_stream.json"))
				return
			}
			w.Write(fixture(t, "openai_edits.json"))
		default:
			t.Errorf("path %s", r.URL.Path)
		}
	})
	ch := &services.Choice{Service: "openai", Key: "o-key"}
	im, err := Generate(context.Background(), &services.Config{}, ch, Request{Prompt: foxPrompt, Alpha: true})
	if err != nil || !bytes.Equal(im.Data, answerData(t, "openai_generations.json", "data", 0, "b64_json")) || im.MIME != "image/png" {
		t.Fatalf("generations: %+v, %v", im, err)
	}
	im, err = Generate(context.Background(), &services.Config{}, ch, Request{Prompt: foxPrompt, Refs: []string{refFile(t)}})
	if err != nil || !bytes.Equal(im.Data, answerData(t, "openai_edits.json", "data", 0, "b64_json")) {
		t.Fatalf("edits: %+v, %v", im, err)
	}
}

// testdata/openrouter_images.json and the recraft_*.json fixtures. With --alpha, Recraft
// makes a PNG and removeBackground turns it into a WebP; cav saves a PNG with alpha.
func TestOpenRouterAndRecraft(t *testing.T) {
	var removed bool
	refs := false
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		switch r.Header.Get("X-Host") + r.URL.Path {
		case "openrouter.ai/api/v1/images":
			b, _ := io.ReadAll(r.Body)
			if !refs {
				sameJSON(t, b, "openrouter_images_request.json")
			} else {
				var body map[string]any
				json.Unmarshal(b, &body)
				if in, _ := body["input_references"].([]any); body["aspect_ratio"] != "1:1" || len(in) != 1 {
					t.Errorf("openrouter body: %v", body)
				}
			}
			w.Write(fixture(t, "openrouter_images.json"))
		case "external.api.recraft.ai/v1/images/generations":
			b, _ := io.ReadAll(r.Body)
			if strings.Contains(string(b), "_vector") {
				sameJSON(t, b, "recraft_vector_request.json")
				w.Write(fixture(t, "recraft_vector.json"))
				return
			}
			sameJSON(t, b, "recraft_generations_request.json")
			w.Write(fixture(t, "recraft_generations.json"))
		case "external.api.recraft.ai/v1/images/removeBackground":
			removed = true
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			f := r.MultipartForm.File["image"]
			if len(f) != 1 || f[0].Header.Get("Content-Type") != "image/png" || r.FormValue("response_format") != "b64_json" {
				t.Errorf("removeBackground form: %v %v", r.MultipartForm.File, r.MultipartForm.Value)
			}
			w.Write(fixture(t, "recraft_remove_background.json"))
		default:
			t.Errorf("unexpected %s%s", r.Header.Get("X-Host"), r.URL.Path)
		}
	})
	ch := &services.Choice{Service: "openrouter", Key: "k"}
	im, err := Generate(context.Background(), &services.Config{}, ch, Request{Prompt: foxPrompt})
	if err != nil || im.MIME != "image/png" || !bytes.Equal(im.Data, answerData(t, "openrouter_images.json", "data", 0, "b64_json")) {
		t.Fatalf("openrouter: %+v, %v", im, err)
	}
	refs = true
	if _, err := Generate(context.Background(), &services.Config{}, ch, Request{Prompt: "x", Aspect: "1:1", Refs: []string{refFile(t)}}); err != nil {
		t.Fatal(err)
	}
	ch = &services.Choice{Service: "recraft", Key: "k"}
	im, err = Generate(context.Background(), &services.Config{}, ch, Request{Prompt: foxPrompt, Alpha: true})
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("--alpha with Recraft must remove the background")
	}
	if im.MIME != "image/png" || http.DetectContentType(im.Data) != "image/png" || !hasTransparency(t, im.Data) {
		t.Fatalf("cutout: %s, sniffed %s", im.MIME, http.DetectContentType(im.Data))
	}
	im, err = Generate(context.Background(), &services.Config{}, ch, Request{Prompt: foxPrompt, Vector: true})
	if err != nil || im.MIME != "image/svg+xml" || im.Ext() != ".svg" || im.Model != "recraftv3_vector" {
		t.Fatalf("vector: %+v, %v", im, err)
	}
}

// testdata/recraft_vectorize.json (cav gen vector --service recraft) and
// recraft_remove_background.json (cav gen cutout --service recraft).
func TestRecraftVectorizeAndCutout(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if f := r.MultipartForm.File["image"]; len(f) != 1 || f[0].Header.Get("Content-Type") != "image/png" {
			t.Errorf("%s: image part %v", r.URL.Path, r.MultipartForm.File)
		}
		switch r.URL.Path {
		case "/v1/images/vectorize":
			w.Write(fixture(t, "recraft_vectorize.json"))
		case "/v1/images/removeBackground":
			w.Write(fixture(t, "recraft_remove_background.json"))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	im, err := RecraftEdit(context.Background(), "k", "vectorize", tinyPNG())
	if err != nil || im.MIME != "image/svg+xml" || !bytes.Contains(im.Data, []byte("<path")) || im.Model != "vectorize" {
		t.Fatalf("vectorize: %+v, %v", im, err)
	}
	im, err = RemoveBackground(context.Background(), &services.Choice{Service: "recraft", Key: "k"}, refFile(t))
	if err != nil || im.MIME != "image/png" || !hasTransparency(t, im.Data) {
		t.Fatalf("cutout: %+v, %v", im, err)
	}
}

func hasTransparency(t *testing.T, data []byte) bool {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a == 0 {
				return true
			}
		}
	}
	return false
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

// The comfyui_*.json fixtures are real answers from ComfyUI 0.39.0 (commit 8010b48) running
// comfyui_sd15_workflow.json, the sample workflow in cav guide production, on 2026-10-13.
const comfyID = "d86e5d8c-e3c5-419c-b328-e9fca3048d2f"

func comfyConfig(t *testing.T) *services.Config {
	wf, err := filepath.Abs("testdata/comfyui_sd15_workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	return &services.Config{Endpoints: map[string]services.Endpoint{"comfyui": {BaseURL: "http://127.0.0.1:8188", Model: wf}}}
}

func TestComfyUIShowsWhyTheServerRefusedTheWorkflow(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(fixture(t, "comfyui_prompt_invalid.json"))
	})
	_, err := Generate(context.Background(), comfyConfig(t), &services.Choice{Service: "comfyui"}, Request{Prompt: "fox"})
	if err == nil || !strings.Contains(err.Error(), "node 4 (CheckpointLoaderSimple): ckpt_name: 'missing.safetensors' not in") {
		t.Fatalf("got %v", err)
	}
}

func TestComfyUISaysWhyTheJobFailed(t *testing.T) {
	const id = "746d83e3-9863-4204-9d38-f7b05c00653a"
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prompt":
			w.Write([]byte(`{"prompt_id": "` + id + `", "number": 3, "node_errors": {}}`))
		case "/history/" + id:
			w.Write(fixture(t, "comfyui_history_interrupted.json"))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	_, err := Generate(context.Background(), comfyConfig(t), &services.Choice{Service: "comfyui"}, Request{Prompt: "fox"})
	if err == nil || !strings.Contains(err.Error(), "interrupted at node 3 (KSampler)") {
		t.Fatalf("got %v", err)
	}
	if got := comfyFailure([]json.RawMessage{json.RawMessage(`["execution_error", {"node_id": "8", "node_type": "VAEDecode", "exception_message": "out of memory\n"}]`)}); got != "node 8 (VAEDecode): out of memory" {
		t.Fatalf("execution_error: %q", got)
	}
}

// The sample workflow in cav guide production must stay the one the fixtures came from.
func TestGuideHasTheComfyUISampleWorkflow(t *testing.T) {
	guide, err := os.ReadFile("../../assets/guide/production.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(guide), strings.TrimSpace(string(fixture(t, "comfyui_sd15_workflow.json")))) {
		t.Fatal("cav guide production does not hold testdata/comfyui_sd15_workflow.json")
	}
}

func TestComfyUIQueuesFillsAndFetches(t *testing.T) {
	png := tinyPNG()
	var queued map[string]any
	polls := 0
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/prompt":
			json.NewDecoder(r.Body).Decode(&queued)
			w.Write(fixture(t, "comfyui_prompt.json"))
		case r.URL.Path == "/history/"+comfyID:
			polls++
			if polls < 2 {
				w.Write([]byte(`{}`)) // still running: the real server answers {}
				return
			}
			w.Write(fixture(t, "comfyui_history_success.json"))
		case r.URL.Path == "/view" && r.URL.Query().Get("filename") == "cav_00002_.png" && r.URL.Query().Get("type") == "output":
			w.Header().Set("Content-Type", "image/png")
			w.Write(png)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	im, err := Generate(context.Background(), comfyConfig(t), &services.Choice{Service: "comfyui"}, Request{Prompt: `a "quoted" fox`, Aspect: "1:1"})
	if err != nil || !bytes.Equal(im.Data, png) || im.Model != "comfyui_sd15_workflow.json" {
		t.Fatalf("got %+v, %v", im, err)
	}
	nodes := queued["prompt"].(map[string]any)
	if nodes["6"].(map[string]any)["inputs"].(map[string]any)["text"] != `a "quoted" fox` {
		t.Fatalf("prompt not filled: %v", nodes["6"])
	}
	if nodes["5"].(map[string]any)["inputs"].(map[string]any)["width"].(float64) != 1024 {
		t.Fatalf("width not filled as a number: %v", nodes["5"])
	}
	if _, ok := nodes["3"].(map[string]any)["inputs"].(map[string]any)["seed"].(float64); !ok {
		t.Fatalf("seed not filled as a number: %v", nodes["3"])
	}
}

func TestMfluxArgsWithReferences(t *testing.T) {
	r := Request{Prompt: "a fox", Aspect: "16:9", Refs: []string{"a.png", "b.png"}}
	cmd, args := mfluxArgs("flux2-klein-4b", r, "out.png")
	want := "--model flux2-klein-4b --prompt a fox --width 1024 --height 576 --output out.png --steps 4 --image-paths a.png b.png"
	if cmd != "mflux-generate-flux2-edit" || strings.Join(args, " ") != want {
		t.Fatalf("flux2 with refs: got %s %q", cmd, args)
	}
	cmd, args = mfluxArgs("flux2-klein-4b", Request{Prompt: "a fox", Aspect: "16:9"}, "out.png")
	if cmd != "mflux-generate-flux2" || strings.Contains(strings.Join(args, " "), "--image") {
		t.Fatalf("flux2 without refs: got %s %q", cmd, args)
	}
	cmd, args = mfluxArgs("schnell", r, "out.png")
	if cmd != "mflux-generate" || !strings.HasSuffix(strings.Join(args, " "), "--image-path a.png --image-strength 0.35") {
		t.Fatalf("schnell with refs: got %s %q", cmd, args)
	}
}

func TestMfluxCommandPerModel(t *testing.T) {
	for model, want := range map[string]string{
		"flux2-klein-4b": "mflux-generate-flux2",
		"z-image-turbo":  "mflux-generate-z-image-turbo",
		"qwen-image":     "mflux-generate-qwen",
		"dev":            "mflux-generate",
		"schnell":        "mflux-generate",
	} {
		if got := mfluxCommand(model); got != want {
			t.Errorf("%s: got %s, want %s", model, got, want)
		}
	}
}

func TestMfluxWarnsBeforeTheFirstDownload(t *testing.T) {
	t.Setenv("HF_HUB_CACHE", t.TempDir())
	note := mfluxDownloadNote("flux2-klein-4b")
	if !strings.Contains(note, "first run downloads the flux2-klein-4b weights (about 16 GB)") {
		t.Fatalf("got %q", note)
	}
	if note := mfluxDownloadNote("z-image-turbo"); note != "" {
		t.Fatalf("unknown model: got %q", note)
	}
}
