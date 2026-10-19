package ghfetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveTar returns an http.Client that answers every request with status and, for 200, a
// tar.gz of files. calls counts the requests, so a test can see a cached fetch is skipped.
func serveTar(t *testing.T, status int, files map[string]string) (*http.Client, *int) {
	t.Helper()
	var body bytes.Buffer
	zw := gzip.NewWriter(&body)
	tw := tar.NewWriter(zw)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	calls := new(int)
	return &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		*calls++
		return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Body: io.NopCloser(bytes.NewReader(body.Bytes()))}, nil
	})}, calls
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchExtractsTheTarballOnce(t *testing.T) {
	client, calls := serveTar(t, http.StatusOK, map[string]string{
		"repo-abc/scripts/a.py":      "print(1)",
		"repo-abc/requirements.txt":  "mlx",
		"repo-abc/docs/guide.md.txt": "hi",
	})
	dest := filepath.Join(t.TempDir(), "cache", "tool")
	ready := func() bool {
		_, err := os.Stat(filepath.Join(dest, "scripts", "a.py"))
		return err == nil
	}
	for i := 0; i < 2; i++ {
		got, err := Fetch(context.Background(), client, "https://example.test/x.tar.gz", dest, ready)
		if err != nil || got != dest {
			t.Fatalf("Fetch: %q %v", got, err)
		}
	}
	if *calls != 1 {
		t.Fatalf("a ready destination was fetched again: %d calls", *calls)
	}
	data, err := os.ReadFile(filepath.Join(dest, "scripts", "a.py"))
	if err != nil || string(data) != "print(1)" {
		t.Fatalf("extracted file: %q %v", data, err)
	}
}

func TestFetchRefusesAnEscapingPath(t *testing.T) {
	// repo-abc/../evil.sh would be safe (filepath.Join cleans it into the destination);
	// ../evil.sh is the one that must be refused.
	client, _ := serveTar(t, http.StatusOK, map[string]string{"../evil.sh": "x"})
	dest := filepath.Join(t.TempDir(), "cache", "tool")
	_, err := Fetch(context.Background(), client, "https://example.test/x.tar.gz", dest, func() bool { return false })
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("got %v", err)
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("the destination exists after a refused tarball")
	}
}

func TestFetchReportsABadStatus(t *testing.T) {
	client, _ := serveTar(t, http.StatusInternalServerError, nil)
	dest := filepath.Join(t.TempDir(), "cache", "tool")
	_, err := Fetch(context.Background(), client, "https://example.test/x.tar.gz", dest, func() bool { return false })
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("got %v", err)
	}
}
