package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/services"
)

// ---------- fal.ai (queue API, one key for many models) ----------
//
// From fal's queue docs and model pages read on 2026-10-08: POST the model's input to
// https://queue.fal.run/<model id> with "Authorization: Key <key>", poll the status_url the
// answer gives until it says COMPLETED, then GET the response_url. Image models answer with
// {"images": [{"url": ..., "content_type": ...}]}. Inputs that take files accept data URIs.

// falEditModel is the model that takes reference images for each text-to-image model cav
// knows. A model missing here gets the references as image_urls unchanged.
var falEditModel = map[string]string{
	"fal-ai/flux-2":     "fal-ai/flux-2/edit",
	"fal-ai/flux-2-pro": "fal-ai/flux-2-pro/edit",
	"fal-ai/bytedance/seedream/v4.5/text-to-image": "fal-ai/bytedance/seedream/v4.5/edit",
	"xai/grok-imagine-image":                       "xai/grok-imagine-image/edit",
	// Ideogram 3 takes image_urls on the same endpoint, as style references.
	"fal-ai/ideogram/v3": "fal-ai/ideogram/v3",
}

// falSizeEnum maps an aspect ratio to the image_size names FLUX.2 and Ideogram accept.
var falSizeEnum = map[string]string{
	"16:9": "landscape_16_9", "9:16": "portrait_16_9",
	"4:3": "landscape_4_3", "3:4": "portrait_4_3", "1:1": "square_hd",
}

// falBody builds the input for one model. The models differ in how they take the size:
// Grok Imagine takes aspect_ratio, Seedream needs at least 2560x1440 pixels, and FLUX.2 and
// Ideogram take a size name or an explicit width and height.
func falBody(model string, r Request) map[string]any {
	body := map[string]any{"prompt": r.Prompt, "num_images": 1}
	w, h := aspectSize(r.Aspect, 1024*1024)
	switch {
	case strings.HasPrefix(model, "xai/grok-imagine"):
		body["aspect_ratio"] = r.Aspect
		body["output_format"] = "png"
	case strings.Contains(model, "seedream"):
		w, h = aspectSize(r.Aspect, 2048*2048)
		body["image_size"] = map[string]int{"width": w, "height": h}
	default:
		if e, ok := falSizeEnum[r.Aspect]; ok {
			body["image_size"] = e
		} else {
			body["image_size"] = map[string]int{"width": w, "height": h}
		}
		if strings.HasPrefix(model, "fal-ai/flux-2") {
			body["output_format"] = "png"
		}
	}
	return body
}

// aspectSize returns a width and height for "W:H" with about the given number of pixels,
// both multiples of 16.
func aspectSize(aspect string, pixels float64) (int, int) {
	ar := 16.0 / 9
	if a, b, ok := strings.Cut(aspect, ":"); ok {
		x, err1 := strconv.ParseFloat(a, 64)
		y, err2 := strconv.ParseFloat(b, 64)
		if err1 == nil && err2 == nil && x > 0 && y > 0 {
			ar = x / y
		}
	}
	h := math.Sqrt(pixels / ar)
	w := h * ar
	return int(math.Round(w/16)) * 16, int(math.Round(h/16)) * 16
}

func fal(ctx context.Context, key, model string, r Request) (*Image, error) {
	endpoint := model
	body := falBody(model, r)
	if len(r.Refs) > 0 {
		if e, ok := falEditModel[model]; ok {
			endpoint = e
		}
		var urls []string
		for _, p := range r.Refs {
			data, mime, err := readRef(p)
			if err != nil {
				return nil, err
			}
			urls = append(urls, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
		}
		body["image_urls"] = urls
	}
	raw, err := FalRun(ctx, key, endpoint, body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Images []struct {
			URL         string `json:"url"`
			ContentType string `json:"content_type"`
		} `json:"images"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Images) == 0 || out.Images[0].URL == "" {
		return findImage(ctx, raw)
	}
	u := out.Images[0].URL
	if strings.HasPrefix(u, "data:") {
		mime, b64, ok := strings.Cut(strings.TrimPrefix(u, "data:"), ";base64,")
		if !ok {
			return nil, fmt.Errorf("unexpected data URI in the answer")
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("image data: %w", err)
		}
		return &Image{Data: data, MIME: mime}, nil
	}
	data, ctype, err := services.DoRaw(ctx, "GET", u, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching the image: %w", err)
	}
	if !strings.HasPrefix(ctype, "image/") {
		ctype = out.Images[0].ContentType
	}
	if !strings.HasPrefix(ctype, "image/") {
		ctype = sniff(data)
	}
	return &Image{Data: data, MIME: ctype}, nil
}

// FalPoll is how long FalRun waits between status checks; tests shorten it.
var FalPoll = 2 * time.Second

// FalRun sends one request through fal's queue and returns the model's JSON answer.
func FalRun(ctx context.Context, key, endpoint string, input any) ([]byte, error) {
	h := map[string]string{"Authorization": "Key " + key}
	var sub struct {
		RequestID   string `json:"request_id"`
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	if err := services.Do(ctx, "POST", "https://queue.fal.run/"+endpoint, h, input, &sub); err != nil {
		return nil, err
	}
	if sub.RequestID == "" && sub.StatusURL == "" {
		return nil, fmt.Errorf("fal gave no request id")
	}
	base := "https://queue.fal.run/" + endpoint + "/requests/" + sub.RequestID
	if sub.StatusURL == "" {
		sub.StatusURL = base + "/status"
	}
	if sub.ResponseURL == "" {
		sub.ResponseURL = base
	}
	for {
		var st struct {
			Status    string `json:"status"`
			Error     string `json:"error"`
			ErrorType string `json:"error_type"`
		}
		if err := services.Do(ctx, "GET", sub.StatusURL, h, nil, &st); err != nil {
			return nil, err
		}
		if st.Error != "" {
			if st.ErrorType != "" {
				return nil, fmt.Errorf("%s (%s)", st.Error, st.ErrorType)
			}
			return nil, fmt.Errorf("%s", st.Error)
		}
		if st.Status == "COMPLETED" {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(FalPoll):
		}
	}
	raw, _, err := services.DoRaw(ctx, "GET", sub.ResponseURL, h, nil)
	return raw, err
}
