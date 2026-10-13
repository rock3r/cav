//go:build cavrecord

package services

// A cav built with -tags cavrecord writes every service request and answer to
// $CAV_RECORD_DIR, one JSON file per call, so live answers can become test fixtures.
// Headers that carry keys are left out, and key-like query values are replaced.
// Binary bodies are recorded by size only. Release builds never include this file.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

func init() {
	dir := os.Getenv("CAV_RECORD_DIR")
	if dir == "" {
		return
	}
	os.MkdirAll(dir, 0o700)
	base := HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	HTTPClient.Transport = &recorder{dir: dir, next: base}
}

type recorder struct {
	dir  string
	next http.RoundTripper
	n    atomic.Int64
}

var secretParams = []string{"key", "token", "api_key", "apikey", "client_secret", "code", "access_token", "refresh_token", "md5"}

func redactURL(u string) string {
	i := strings.IndexByte(u, '?')
	if i < 0 {
		return u
	}
	parts := strings.Split(u[i+1:], "&")
	for j, p := range parts {
		k, _, _ := strings.Cut(p, "=")
		for _, s := range secretParams {
			if strings.EqualFold(k, s) {
				parts[j] = k + "=REDACTED"
			}
		}
	}
	return u[:i+1] + strings.Join(parts, "&")
}

func bodyForRecord(b []byte, ctype string) any {
	if len(b) == 0 {
		return nil
	}
	if strings.Contains(ctype, "x-www-form-urlencoded") {
		return redactURL("?" + string(b))[1:]
	}
	if strings.Contains(ctype, "json") || (utf8.Valid(b) && !strings.HasPrefix(ctype, "image/") && !strings.HasPrefix(ctype, "audio/")) {
		var v any
		if json.Unmarshal(b, &v) == nil {
			return redactJSON(v)
		}
		if len(b) > 20000 {
			b = b[:20000]
		}
		return hideKeys(string(b))
	}
	return fmt.Sprintf("<%d bytes of %s>", len(b), ctype)
}

// redactJSON replaces long base64-like strings (images, audio) and key-like fields.
func redactJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			lk := strings.ToLower(k)
			if lk == "api_key" || lk == "client_secret" || lk == "access_token" || lk == "refresh_token" || lk == "label" {
				x[k] = "REDACTED"
				continue
			}
			x[k] = redactJSON(e)
		}
	case []any:
		for i, e := range x {
			x[i] = redactJSON(e)
		}
	case string:
		if len(x) > 2000 {
			return fmt.Sprintf("<%d characters>", len(x))
		}
		// Error messages can echo a key, whole or masked.
		return hideKeys(x)
	}
	return v
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var reqBody []byte
	if req.Body != nil {
		reqBody, _ = io.ReadAll(req.Body)
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}
	start := time.Now()
	resp, err := r.next.RoundTrip(req)
	rec := map[string]any{
		"method":       req.Method,
		"url":          redactURL(req.URL.String()),
		"request_type": req.Header.Get("Content-Type"),
		"request":      bodyForRecord(reqBody, req.Header.Get("Content-Type")),
		"ms":           time.Since(start).Milliseconds(),
	}
	if err != nil {
		rec["error"] = err.Error()
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		rec["status"] = resp.StatusCode
		rec["response_type"] = resp.Header.Get("Content-Type")
		rec["response"] = bodyForRecord(body, resp.Header.Get("Content-Type"))
	}
	n := r.n.Add(1)
	name := fmt.Sprintf("%d-%03d-%s.json", os.Getpid(), n, strings.ReplaceAll(req.URL.Host, ".", "_"))
	if b, e := json.MarshalIndent(rec, "", "  "); e == nil {
		os.WriteFile(filepath.Join(r.dir, name), b, 0o600)
	}
	return resp, err
}
