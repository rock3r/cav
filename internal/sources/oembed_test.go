package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

// toServer sends every request of the shared client to one test server, keeping the host the
// code asked for in the X-Asked-Host header. No test touches the network.
type toServer struct{ u *url.URL }

func (s toServer) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("X-Asked-Host", r.URL.Host)
	r2.URL.Scheme, r2.URL.Host = s.u.Scheme, s.u.Host
	return http.DefaultTransport.RoundTrip(r2)
}

func stubVideoServices(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: toServer{u}}
	t.Cleanup(func() { services.HTTPClient = old })
	return srv
}

const png1x1 = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89"

func TestOEmbedAsksWithTheParsedIDOnly(t *testing.T) {
	var asked []string
	stubVideoServices(t, func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.Header.Get("X-Asked-Host")+r.URL.Path+"?"+r.URL.Query().Get("url"))
		switch r.Header.Get("X-Asked-Host") {
		case "www.youtube.com":
			w.Write([]byte(`{"title":"Reel","author_name":"Studio X","author_url":"https://www.youtube.com/@x","thumbnail_url":"https://i.ytimg.com/vi/aqz-KE-bpKQ/hqdefault.jpg"}`))
		case "vimeo.com":
			w.Write([]byte(`{"title":"Film","author_name":"Maker","author_url":"https://vimeo.com/maker","thumbnail_url":"https://evil.test/t.jpg"}`))
		default:
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	e, err := OEmbed(ctx, "https://youtu.be/aqz-KE-bpKQ?si=junk&list=other")
	if err != nil || e.Title != "Reel" || e.Author != "Studio X" || e.ThumbURL == "" || e.ID != "aqz-KE-bpKQ" {
		t.Fatalf("%+v %v", e, err)
	}
	v, err := OEmbed(ctx, "https://vimeo.com/123456789")
	if err != nil || v.Title != "Film" {
		t.Fatalf("%+v %v", v, err)
	}
	if v.ThumbURL != "" {
		t.Errorf("a thumbnail on another host must be dropped: %s", v.ThumbURL)
	}
	want := []string{"www.youtube.com/oembed?https://www.youtube.com/watch?v=aqz-KE-bpKQ", "vimeo.com/api/oembed.json?https://vimeo.com/123456789"}
	if strings.Join(asked, "|") != strings.Join(want, "|") {
		t.Errorf("asked %v", asked)
	}
	if _, err := OEmbed(ctx, "https://evil.test/watch?v=aqz-KE-bpKQ"); err == nil || len(asked) != 2 {
		t.Errorf("another host must not be asked: %v %v", err, asked)
	}
}

func TestAllowedThumb(t *testing.T) {
	for in, want := range map[string]bool{
		"https://i.ytimg.com/vi/x/hqdefault.jpg": true,
		"https://i.vimeocdn.com/video/1_295x166": true,
		"https://i-f.vimeocdn.com/video/1.jpg":   true,
		"http://i.ytimg.com/vi/x/a.jpg":          false,
		"https://i.ytimg.com.evil.test/a.jpg":    false,
		"https://evilvimeocdn.com/a.jpg":         false,
		"https://x@i.ytimg.com/a.jpg":            false,
		"https://i.ytimg.com:8443/a.jpg":         false,
		"file:///etc/passwd":                     false,
		"":                                       false,
	} {
		if AllowedThumb(in) != want {
			t.Errorf("%q should be %v", in, want)
		}
	}
}

func TestSaveThumb(t *testing.T) {
	stubVideoServices(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/img.png":
			w.Write([]byte(png1x1))
		case "/html":
			w.Write([]byte("<html>not an image</html>"))
		case "/redirect":
			http.Redirect(w, r, "https://evil.test/img.png", http.StatusFound)
		}
	})
	dir := t.TempDir()
	out := filepath.Join(dir, ".thumbs", "t.jpg")
	ctx := context.Background()
	if err := SaveThumb(ctx, "https://i.ytimg.com/img.png", out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != png1x1 {
		t.Error("thumbnail not saved")
	}
	for _, bad := range []string{"https://i.ytimg.com/html", "https://i.ytimg.com/redirect", "https://evil.test/img.png", "http://i.ytimg.com/img.png"} {
		if err := SaveThumb(ctx, bad, filepath.Join(dir, "x.jpg")); err == nil {
			t.Errorf("%s should fail", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "x.jpg")); err == nil {
		t.Error("a refused thumbnail left a file")
	}
}
