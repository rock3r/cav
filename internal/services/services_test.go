package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestParseSource(t *testing.T) {
	for _, ok := range []string{"env:X", "keychain:cav/openai", "op://Private/OpenAI/api key", "oauth"} {
		if _, err := ParseSource(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "env:", "sk-live-abc", "keychain:nope", "op://Private", "op://a/b"} {
		if _, err := ParseSource(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestResolveNeverLeaksAndClassifiesOp(t *testing.T) {
	t.Setenv("CAV_TEST_KEY", "  secret-value \n")
	v, err := Resolve(context.Background(), "openai", "env:CAV_TEST_KEY")
	if err != nil || v != "secret-value" {
		t.Fatalf("env: got %q, %v", v, err)
	}
	_, err = Resolve(context.Background(), "openai", "env:CAV_TEST_MISSING")
	var ke *KeyError
	if !errors.As(err, &ke) || !ke.Missing {
		t.Fatalf("missing env: %v", err)
	}

	oldRun, oldLook := runCmd, lookPath
	defer func() { runCmd, lookPath = oldRun, oldLook }()
	lookPath = func(string) (string, error) { return "/usr/local/bin/op", nil }
	runCmd = func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		return nil, []byte("[ERROR] 2026/10/08 account is not signed in"), errors.New("exit 1")
	}
	_, err = Resolve(context.Background(), "openai", "op://Private/OpenAI/key")
	if !errors.As(err, &ke) || !strings.Contains(ke.Msg, "locked or not signed in") || ke.Missing {
		t.Fatalf("locked op: %v", err)
	}
	runCmd = func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name != "op" || args[0] != "read" || args[len(args)-1] != "op://Private/OpenAI/key" {
			t.Fatalf("unexpected command %s %v", name, args)
		}
		return []byte("sk-from-op\n"), nil, nil
	}
	v, err = Resolve(context.Background(), "openai", "op://Private/OpenAI/key")
	if err != nil || v != "sk-from-op" {
		t.Fatalf("op: got %q, %v", v, err)
	}
}

func TestPickSkipsUnusableServices(t *testing.T) {
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")
	t.Setenv("OPENAI_API_KEY", "")
	c := &Config{}
	ch, err := Pick(context.Background(), c, "image", "")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Service != "greybox" || len(ch.Skipped) != 3 {
		t.Fatalf("no keys: got %s, skipped %v", ch.Service, ch.Skipped)
	}
	t.Setenv("OPENAI_API_KEY", "sk-test")
	ch, err = Pick(context.Background(), c, "image", "")
	if err != nil || ch.Service != "openai" || ch.Key != "sk-test" {
		t.Fatalf("with openai key: %+v %v", ch, err)
	}
	if _, err := Pick(context.Background(), c, "image", "freesound"); err == nil {
		t.Fatal("freesound cannot make images")
	}
	if err := c.SetOrder("image", []string{"greybox", "openai"}); err != nil {
		t.Fatal(err)
	}
	ch, _ = Pick(context.Background(), c, "image", "")
	if ch.Service != "greybox" {
		t.Fatalf("configured order ignored: %s", ch.Service)
	}
	if err := c.SetOrder("music", []string{"pexels"}); err == nil {
		t.Fatal("pexels cannot make music")
	}
}

// redirect sends every request to the test server, keeping the path and query.
type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Original-Host", req.URL.Host)
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestProbeReportsRefusedKeyWithoutLeakingIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Original-Host") == "api.openai.com" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"message":"Incorrect API key provided: sk-test-0123456789"}}`))
			return
		}
		w.Write([]byte(`{"result_count": 7}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := HTTPClient
	HTTPClient = &http.Client{Transport: redirect{u}}
	defer func() { HTTPClient = old }()

	t.Setenv("OPENAI_API_KEY", "sk-test-0123456789")
	st := Probe(context.Background(), &Config{}, "openai")
	if st.State != "fail" || !strings.Contains(st.Detail, "refused") || st.Fix == "" {
		t.Fatalf("openai: %+v", st)
	}
	if strings.Contains(st.Detail, "sk-test-0123456789") {
		t.Fatalf("key leaked into the report: %s", st.Detail)
	}
	st = Probe(context.Background(), &Config{}, "openverse")
	if st.State != "ok" || !strings.Contains(st.Detail, "7") {
		t.Fatalf("openverse: %+v", st)
	}
	t.Setenv("PEXELS_API_KEY", "")
	if st = Probe(context.Background(), &Config{}, "pexels"); st.State != "skip" {
		t.Fatalf("pexels without key: %+v", st)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetKey("openai", "op://Private/OpenAI/api key"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetKey("openai", "sk-raw-key-pasted"); err == nil {
		t.Fatal("a raw key must not be accepted as a source")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(Path())
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions %v", info.Mode().Perm())
	}
	c2, _ := Load()
	if c2.KeySource("openai") != "op://Private/OpenAI/api key" {
		t.Fatalf("round trip: %v", c2.Keys)
	}
}
