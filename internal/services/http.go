package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClient is shared by every service call; tests replace its transport.
var HTTPClient = &http.Client{Timeout: 5 * time.Minute}

// UserAgent identifies cav to the services (Openverse and Wikimedia ask for one).
var UserAgent = "cav (https://github.com/rock3r/cav)"

// HTTPError is a non-2xx answer. Body is cut short and never holds the key.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	msg := fmt.Sprintf("HTTP %d", e.Status)
	switch e.Status {
	case 401:
		msg += " (the key was refused)"
	case 403:
		msg += " (the key lacks access, or the service blocks this request)"
	case 429:
		msg += " (rate limited or out of credit)"
	}
	if b := bodyMessage(e.Body); b != "" {
		if len(b) > 300 {
			b = b[:300] + "…"
		}
		msg += ": " + b
	}
	return msg
}

// KeyRefused reports whether a service refused the key (some answer 400, not 401).
func (e *HTTPError) KeyRefused() bool {
	return e.Status == 401 || e.Status == 403 || (e.Status == 400 && strings.Contains(e.Body, "API_KEY_INVALID"))
}

// bodyMessage pulls the human message out of the usual JSON error shapes.
func bodyMessage(body string) string {
	var v map[string]any
	if json.Unmarshal([]byte(body), &v) != nil {
		return strings.Join(strings.Fields(body), " ")
	}
	pick := func(m map[string]any) string {
		for _, k := range []string{"message", "detail", "error_description", "error"} {
			switch x := m[k].(type) {
			case string:
				return x
			case map[string]any:
				if s, ok := x["message"].(string); ok {
					return s
				}
			}
		}
		return ""
	}
	if s := pick(v); s != "" {
		return s
	}
	if e, ok := v["error"].(map[string]any); ok {
		if s := pick(e); s != "" {
			return s
		}
	}
	return strings.Join(strings.Fields(body), " ")
}

// Do sends a request and decodes a JSON answer into out (when out is not nil). body may be
// nil, []byte, or any value that is sent as JSON.
func Do(ctx context.Context, method, url string, headers map[string]string, body any, out any) error {
	raw, _, err := DoRaw(ctx, method, url, headers, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unexpected answer from %s: %w", hostOf(url), err)
	}
	return nil
}

// DoRaw sends a request and returns the raw answer and its content type.
func DoRaw(ctx context.Context, method, url string, headers map[string]string, body any) ([]byte, string, error) {
	var r io.Reader
	ctype := ""
	switch b := body.(type) {
	case nil:
	case []byte:
		r = bytes.NewReader(b)
	case io.Reader:
		r = b
	default:
		j, err := json.Marshal(b)
		if err != nil {
			return nil, "", err
		}
		r = bytes.NewReader(j)
		ctype = "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", hostOf(url), err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", &HTTPError{Status: resp.StatusCode, Body: string(raw)}
	}
	return raw, resp.Header.Get("Content-Type"), nil
}

func hostOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexAny(u, "/?"); i >= 0 {
		return u[:i]
	}
	return u
}
