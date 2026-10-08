// Package videogen makes short moving shots for the animatic with Veo 3.1 through the
// Gemini API, starting from a storyboard frame.
//
// Request shapes follow the Gemini API Veo guide read on 2026-10-08: POST
// models/<model>:predictLongRunning with "instances" (prompt, image) and "parameters", poll
// the returned operation until done, then download
// response.generateVideoResponse.generatedSamples[0].video.uri with the key. They are tested
// against recorded shapes, not against the live service (no keys were available).
package videogen

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/services"
)

const base = "https://generativelanguage.googleapis.com/v1beta"

// DefaultModels, used when the config names none (cav config model veo <id>). Lite is the
// cheapest Veo 3.1; veo-3.1-fast-generate-preview and veo-3.1-generate-preview also work.
var DefaultModels = map[string]string{
	"veo": "veo-3.1-lite-generate-preview",
}

// Request is one shot to make.
type Request struct {
	Prompt     string
	Image      string  // first frame (path); empty makes the shot from the prompt alone
	Aspect     string  // 16:9 or 9:16
	Seconds    float64 // the shot's length; Veo makes 4, 6 or 8 seconds
	Resolution string  // 720p (default) or 1080p (8 seconds only)
	Model      string
}

// Clip is the result: an MP4 at 24 fps, with Veo's own audio track.
type Clip struct {
	Data     []byte
	Seconds  int
	Provider string
	Model    string
}

// Duration picks the Veo length that covers a shot: 4, 6 or 8 seconds. A longer shot gets
// 8 seconds and holds its last frame.
func Duration(seconds float64) int {
	for _, d := range []int{4, 6} {
		if seconds <= float64(d) {
			return d
		}
	}
	return 8
}

// Poll is how long Generate waits between operation checks (the guide uses 10 seconds).
var Poll = 10 * time.Second

func Generate(ctx context.Context, c *services.Config, ch *services.Choice, r Request) (*Clip, error) {
	if ch.Service != "veo" {
		return nil, fmt.Errorf("%s does not make video", ch.Service)
	}
	model := r.Model
	if model == "" {
		model = c.Models["veo"]
	}
	if model == "" {
		model = DefaultModels["veo"]
	}
	cl, err := veo(ctx, ch.Key, model, r)
	if err != nil {
		return nil, fmt.Errorf("veo (%s): %w", model, err)
	}
	cl.Provider, cl.Model = "veo", model
	return cl, nil
}

func veo(ctx context.Context, key, model string, r Request) (*Clip, error) {
	h := map[string]string{"x-goog-api-key": key}
	inst := map[string]any{"prompt": r.Prompt}
	if r.Image != "" {
		data, err := os.ReadFile(r.Image)
		if err != nil {
			return nil, err
		}
		mime := http.DetectContentType(data)
		if !strings.HasPrefix(mime, "image/") {
			return nil, fmt.Errorf("%s is not an image (%s)", r.Image, mime)
		}
		inst["image"] = map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": base64.StdEncoding.EncodeToString(data)}}
	}
	aspect := r.Aspect
	if aspect != "9:16" {
		aspect = "16:9"
	}
	secs := Duration(r.Seconds)
	res := r.Resolution
	if res == "" {
		res = "720p"
	}
	if res != "720p" {
		secs = 8 // 1080p and 4k are 8 seconds only
	}
	// The guide lists durationSeconds as "4", "6" or "8". A string is also accepted where
	// the field is a number, so send it as the guide writes it.
	params := map[string]any{"aspectRatio": aspect, "durationSeconds": fmt.Sprint(secs), "resolution": res}
	body := map[string]any{"instances": []any{inst}, "parameters": params}
	var op operation
	if err := services.Do(ctx, "POST", base+"/models/"+model+":predictLongRunning", h, body, &op); err != nil {
		return nil, err
	}
	if op.Name == "" {
		return nil, fmt.Errorf("no operation in the answer")
	}
	for !op.Done {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(Poll):
		}
		name := op.Name
		op = operation{}
		if err := services.Do(ctx, "GET", base+"/"+name, h, nil, &op); err != nil {
			return nil, err
		}
		if op.Name == "" {
			op.Name = name
		}
	}
	if op.Error != nil {
		return nil, fmt.Errorf("%s (code %d)", op.Error.Message, op.Error.Code)
	}
	resp := op.Response.GenerateVideoResponse
	if len(resp.GeneratedSamples) == 0 || resp.GeneratedSamples[0].Video.URI == "" {
		// raiMediaFilteredReasons is not in the guide; it is read only when present.
		if len(resp.RAIMediaFilteredReasons) > 0 {
			return nil, fmt.Errorf("the video was filtered: %s", strings.Join(resp.RAIMediaFilteredReasons, "; "))
		}
		return nil, fmt.Errorf("no video in the answer")
	}
	// The download URI needs the key and redirects to the file.
	data, _, err := services.DoRaw(ctx, "GET", resp.GeneratedSamples[0].Video.URI, h, nil)
	if err != nil {
		return nil, fmt.Errorf("downloading the video: %w", err)
	}
	return &Clip{Data: data, Seconds: secs}, nil
}

type operation struct {
	Name  string `json:"name"`
	Done  bool   `json:"done"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Response struct {
		GenerateVideoResponse struct {
			GeneratedSamples []struct {
				Video struct {
					URI string `json:"uri"`
				} `json:"video"`
			} `json:"generatedSamples"`
			RAIMediaFilteredReasons []string `json:"raiMediaFilteredReasons"`
		} `json:"generateVideoResponse"`
	} `json:"response"`
}
