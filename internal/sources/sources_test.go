package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
)

// The openverse_* and wikimedia_* fixtures are real answers recorded with curl on
// 2026-10-08. The Freesound, Pexels and Unsplash answers below are written from their API
// documentation (no key was available), so they test the mapping, not the live shape.

type replay struct {
	t      *testing.T
	target *url.URL
}

func (r replay) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Host", req.URL.Host)
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func fixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fake struct {
	mu    sync.Mutex
	calls []string
}

func setup(t *testing.T) *fake {
	f := &fake{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Header.Get("X-Host")
		f.mu.Lock()
		f.calls = append(f.calls, host+r.URL.Path)
		f.mu.Unlock()
		switch {
		case host == "api.openverse.org" && r.URL.Path == "/v1/audio/":
			w.Write(fixture(t, "openverse_audio_search.json"))
		case host == "api.openverse.org" && strings.HasPrefix(r.URL.Path, "/v1/audio/"):
			w.Write(fixture(t, "openverse_audio_item.json"))
		case host == "api.openverse.org" && r.URL.Path == "/v1/images/":
			w.Write(fixture(t, "openverse_image_search.json"))
		case host == "commons.wikimedia.org":
			w.Write(fixture(t, "wikimedia_search.json"))
		case host == "freesound.org" && r.URL.Path == "/apiv2/search/text/":
			if r.Header.Get("Authorization") != "Token fs-key" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"count":2,"results":[
			  {"id":351256,"name":"Deep Whoosh","username":"Kinoton","license":"http://creativecommons.org/publicdomain/zero/1.0/","duration":3.1,"previews":{"preview-hq-mp3":"https://cdn.freesound.org/previews/351/351256_2247456-hq.mp3"},"url":"https://freesound.org/people/Kinoton/sounds/351256/","type":"wav","tags":["whoosh"]},
			  {"id":42,"name":"Swish","username":"someone","license":"https://creativecommons.org/licenses/by-nc/4.0/","duration":1.2,"previews":{"preview-hq-mp3":"https://cdn.freesound.org/previews/0/42-hq.mp3"},"url":"https://freesound.org/people/someone/sounds/42/","type":"wav"}]}`))
		case host == "api.unsplash.com" && strings.HasPrefix(r.URL.Path, "/photos/abc"):
			if strings.HasSuffix(r.URL.Path, "/download") {
				w.Write([]byte(`{"url":"x"}`))
				return
			}
			w.Write([]byte(`{"id":"abc123","width":4000,"height":3000,"description":"Neon sign","urls":{"full":"https://images.unsplash.com/photo-abc?full","regular":"https://images.unsplash.com/photo-abc?regular"},"links":{"html":"https://unsplash.com/photos/abc123","download_location":"https://api.unsplash.com/photos/abc123/download"},"user":{"name":"Ana Lens","links":{"html":"https://unsplash.com/@ana"}}}`))
		case host == "cdn.freesound.org", host == "images.unsplash.com", host == "upload.wikimedia.org":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("fake media bytes"))
		default:
			t.Logf("unexpected request %s%s", host, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: replay{t, u}}
	t.Cleanup(func() { services.HTTPClient = old })
	return f
}

func TestOpenverseAudioRealAnswer(t *testing.T) {
	setup(t)
	ctx := context.Background()
	rs, err := Search(ctx, &services.Config{}, "openverse", "", Query{Text: "whoosh", Kind: "audio", Limit: 10, Licence: "any"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 6 {
		t.Fatalf("got %d results", len(rs))
	}
	var nc, cc0 int
	for _, r := range rs {
		if r.Licence == "CC-BY-NC-4.0" && !r.Terms.CommercialOK {
			nc++
		}
		if r.Licence == "CC0-1.0" {
			cc0++
		}
		if r.Preview == "" || r.Attribution == "" {
			t.Fatalf("missing preview or attribution: %+v", r)
		}
	}
	if nc != 1 || cc0 != 4 {
		t.Fatalf("licence mapping: nc=%d cc0=%d", nc, cc0)
	}
	rs, _ = Search(ctx, &services.Config{}, "openverse", "", Query{Text: "whoosh", Kind: "audio", Limit: 10, Licence: "commercial", MaxSeconds: 5})
	for _, r := range rs {
		if !r.Terms.CommercialOK || r.Seconds > 5 {
			t.Fatalf("filter let through %s %.1fs", r.Licence, r.Seconds)
		}
	}
	if len(rs) != 3 {
		t.Fatalf("commercial and at most 5 s: got %d, want 3", len(rs))
	}
}

func TestWikimediaRealAnswer(t *testing.T) {
	setup(t)
	rs, err := Search(context.Background(), &services.Config{}, "wikimedia", "", Query{Text: "neon sign", Kind: "image", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var sa, pd int
	for _, r := range rs {
		if r.Terms.ShareAlike && r.Terms.AttributionRequired && strings.HasPrefix(r.Licence, "CC-BY-SA-") {
			sa++
		}
		if r.Licence == "PDM" {
			pd++
		}
		if strings.Contains(r.Author, "<") {
			t.Fatalf("HTML left in author: %q", r.Author)
		}
	}
	if sa != 5 || pd != 1 {
		t.Fatalf("licences: share-alike %d, public domain %d", sa, pd)
	}
}

func TestDownloadRecordsLicenceAndCredit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	dir := t.TempDir()
	c := &services.Config{}
	rs, err := Search(ctx, c, "freesound", "fs-key", Query{Text: "whoosh", Kind: "audio", Limit: 5, Licence: "commercial"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Licence != "CC0-1.0" {
		t.Fatalf("freesound commercial filter: %+v", rs)
	}
	e, err := Download(ctx, c, "fs-key", rs[0], dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Preview || !e.CommercialOK || e.SHA256 == "" || filepath.Ext(e.Path) != ".mp3" {
		t.Fatalf("entry %+v", e)
	}
	if _, err := Download(ctx, c, "fs-key", rs[0], dir, true); err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("original without OAuth should ask for a login, got %v", err)
	}

	r, err := Lookup(ctx, c, "unsplash", "us-key", "image", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	e, err = Download(ctx, c, "us-key", r, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if e.Attribution != "Photo by Ana Lens on Unsplash" || e.Licence != "Unsplash" {
		t.Fatalf("unsplash entry %+v", e)
	}
	tracked := false
	for _, call := range f.calls {
		if call == "api.unsplash.com/photos/abc123/download" {
			tracked = true
		}
	}
	if !tracked {
		t.Fatalf("Unsplash download was not reported: %v", f.calls)
	}
}

func TestLicenceNames(t *testing.T) {
	cases := map[string]string{
		"CC BY-SA 4.0": "CC-BY-SA-4.0", "CC0": "CC0-1.0", "Public domain": "PDM",
		"Attribution NonCommercial": "CC-BY-NC-4.0", "Creative Commons 0": "CC0-1.0", "Attribution 3.0": "CC-BY-3.0",
	}
	for in, want := range cases {
		if got := library.FromName(in).ID; got != want {
			t.Errorf("%q: got %s, want %s", in, got, want)
		}
	}
	if got := library.FromURL("http://creativecommons.org/licenses/by-nc-nd/3.0/"); got.ID != "CC-BY-NC-ND-3.0" || !got.NoDerivatives || got.CommercialOK {
		t.Errorf("URL: %+v", got)
	}
}

func TestArenaChannelRealAnswer(t *testing.T) {
	// arena_contents.json is page 1 of are.na/abstract-motion-design, recorded 2026-10-08.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Host") != "api.are.na" || r.URL.Path != "/v2/channels/abstract-motion-design/contents" {
			t.Errorf("unexpected %s%s", r.Header.Get("X-Host"), r.URL.Path)
		}
		if r.URL.Query().Get("page") == "1" {
			w.Write(fixture(t, "arena_contents.json"))
			return
		}
		w.Write([]byte(`{"contents":[]}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: replay{t, u}}
	defer func() { services.HTTPClient = old }()

	rs, err := ArenaChannel(context.Background(), &services.Config{}, "", "https://www.are.na/zed-zara/abstract-motion-design", 50)
	if err != nil {
		t.Fatal(err)
	}
	// Six blocks: two attachments without a preview image, one with, and three images.
	if len(rs) != 4 {
		t.Fatalf("got %d images", len(rs))
	}
	for _, r := range rs {
		if r.Licence != "reference-only" || r.Original == "" || r.Author == "" {
			t.Fatalf("result %+v", r)
		}
	}
	if ArenaSlug("abstract-motion-design/") != "abstract-motion-design" {
		t.Fatal("slug")
	}
}
