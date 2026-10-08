package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

type toServer struct{ u *url.URL }

func (r toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.u.Scheme, r.u.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestLiveChecksAskFirstAndProveTheJob(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + b64 + `"}}]}}]}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: toServer{u}}
	defer func() { services.HTTPClient = old }()
	t.Setenv("GEMINI_API_KEY", "g-key")

	st := []services.Status{{Service: "gemini", State: "ok"}, {Service: "pexels", State: "skip"}}
	if _, err := runLiveChecks(&app{json: true}, &services.Config{}, st, false); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("--json without --yes must refuse to spend: %v", err)
	}
	if calls != 0 {
		t.Fatal("a paid call was made without confirmation")
	}
	out, err := runLiveChecks(&app{json: true}, &services.Config{}, st, true)
	if err != nil || len(out) != 1 || out[0].State != "ok" || !strings.Contains(out[0].Detail, "image/png") {
		t.Fatalf("live result %+v, %v", out, err)
	}
}
