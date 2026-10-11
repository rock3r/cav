package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A fake YuE2-Turbo (src/yue2/service.py, read at commit 7c88813 on 2026-10-11): no
// "extensions" in /health/ready, and a Bearer key on /internal/load. A fake cav wrapper
// lists "instrumental" and has no /internal/load.
func TestYuE2CheckTellsTurboFromTheWrapper(t *testing.T) {
	const key = "turbo-key-0123456789"
	turbo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/ready":
			w.Write([]byte(`{"status":"ready"}`))
		case "/internal/load":
			if r.Header.Get("Authorization") != "Bearer "+key {
				w.WriteHeader(401)
				w.Write([]byte(`{"detail":"Invalid bearer token"}`))
				return
			}
			w.Write([]byte(`{"schema_version":1,"ready":true,"capacity":16}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer turbo.Close()
	wrapper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health/ready" {
			w.Write([]byte(`{"status":"ready","server":"cav-yue2-server","extensions":["instrumental"],"device":"mps"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer wrapper.Close()
	probe := func(url, env string) Status {
		t.Setenv("YUE2_API_KEY", env)
		c := &Config{Endpoints: map[string]Endpoint{"yue2": {BaseURL: url}}}
		return Probe(context.Background(), c, "yue2")
	}
	if st := probe(turbo.URL, ""); st.State != "fail" || !strings.Contains(st.Detail, "needs its API key") || !strings.Contains(st.Fix, "YUE2_API_KEY") {
		t.Errorf("Turbo with no key: %+v", st)
	}
	if st := probe(turbo.URL, "wrong-key-0123456789"); st.State != "fail" || !strings.Contains(st.Detail, "refused the key (HTTP 401)") ||
		strings.Contains(st.Detail+st.Fix, "wrong-key") {
		t.Errorf("Turbo with the wrong key: %+v", st)
	}
	if st := probe(turbo.URL, key); st.State != "ok" || !strings.Contains(st.Detail, "YuE2-Turbo ready") || !strings.Contains(st.Detail, "two jobs") {
		t.Errorf("Turbo: %+v", st)
	}
	if st := probe(wrapper.URL, ""); st.State != "ok" || !strings.Contains(st.Detail, "wrapper ready") || !strings.Contains(st.Detail, "on mps") {
		t.Errorf("wrapper: %+v", st)
	}
}
