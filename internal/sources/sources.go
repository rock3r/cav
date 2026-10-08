// Package sources searches free and licensed media libraries (sound effects, music and
// reference images) and downloads a chosen item with its licence and credit recorded.
package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
)

// Result is one search hit.
type Result struct {
	Service     string        `json:"service"`
	ID          string        `json:"id"`
	Kind        string        `json:"kind"` // audio, image
	Title       string        `json:"title"`
	Author      string        `json:"author,omitempty"`
	AuthorURL   string        `json:"authorUrl,omitempty"`
	Page        string        `json:"page,omitempty"`
	Preview     string        `json:"preview,omitempty"`  // a file anyone can download
	Original    string        `json:"original,omitempty"` // the full file; may need a login
	Terms       library.Terms `json:"-"`
	Licence     string        `json:"licence"`
	Attribution string        `json:"attribution,omitempty"`
	Seconds     float64       `json:"seconds,omitempty"`
	Width       int           `json:"width,omitempty"`
	Height      int           `json:"height,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	// track is a URL the source asks apps to call when the file is used (Unsplash).
	track string
}

// Ref is "service:id", the handle that `get` takes.
func (r Result) Ref() string { return r.Service + ":" + r.ID }

// Query is a search.
type Query struct {
	Text       string
	Kind       string // audio, image
	Limit      int
	Licence    string  // any, commercial, nc-ok
	MaxSeconds float64 // audio only; 0 = no limit
}

type adapter struct {
	kinds  []string
	search func(ctx context.Context, c *services.Config, key string, q Query) ([]Result, error)
	lookup func(ctx context.Context, c *services.Config, key, kind, id string) (Result, error)
}

var adapters = map[string]adapter{}

func Supports(service, kind string) bool {
	a, ok := adapters[service]
	if !ok {
		return false
	}
	for _, k := range a.kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Search runs the query on one service.
func Search(ctx context.Context, c *services.Config, service, key string, q Query) ([]Result, error) {
	a, ok := adapters[service]
	if !ok || !Supports(service, q.Kind) {
		return nil, fmt.Errorf("%s does not search %s", service, q.Kind)
	}
	if q.Limit <= 0 {
		q.Limit = 12
	}
	rs, err := a.search(ctx, c, key, q)
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, r := range rs {
		r.Licence = r.Terms.ID
		if !r.Terms.Allowed(q.Licence) {
			continue
		}
		if q.MaxSeconds > 0 && r.Seconds > q.MaxSeconds {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Lookup fetches one item by id.
func Lookup(ctx context.Context, c *services.Config, service, key, kind, id string) (Result, error) {
	a, ok := adapters[service]
	if !ok {
		return Result{}, fmt.Errorf("unknown source %q", service)
	}
	r, err := a.lookup(ctx, c, key, kind, id)
	r.Licence = r.Terms.ID
	return r, err
}

// Download saves the item into dir and returns its manifest entry. With original false, or
// when the original needs a login cav does not have, it saves the preview and says so.
func Download(ctx context.Context, c *services.Config, key string, r Result, dir string, original bool) (library.Entry, error) {
	src, isPreview := r.Preview, true
	headers := map[string]string{}
	if original && r.Original != "" {
		src, isPreview = r.Original, false
		if r.Service == "freesound" {
			if c.Keys["freesound"] != "oauth" {
				return library.Entry{}, fmt.Errorf("Freesound originals need a login: run `cav config login freesound`, or drop --original for the preview")
			}
			headers = services.FreesoundAuth(c, key)
		}
	}
	if src == "" {
		src, isPreview = r.Original, false
	}
	if src == "" {
		return library.Entry{}, fmt.Errorf("%s has no downloadable file", r.Ref())
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return library.Entry{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", src, nil)
	if err != nil {
		return library.Entry{}, err
	}
	req.Header.Set("User-Agent", services.UserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := services.HTTPClient.Do(req)
	if err != nil {
		return library.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		return library.Entry{}, &services.HTTPError{Status: resp.StatusCode, Body: string(b)}
	}
	name := fileName(r, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"), src)
	out := filepath.Join(dir, name)
	tmp := out + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return library.Entry{}, err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return library.Entry{}, err
	}
	if err := os.Rename(tmp, out); err != nil {
		return library.Entry{}, err
	}
	if r.track != "" {
		// Unsplash requires this call when a photo is downloaded; it only counts the use.
		services.Do(ctx, "GET", r.track, map[string]string{"Authorization": "Client-ID " + key}, nil, nil)
	}
	e := library.Entry{
		Path: out, Kind: r.Kind, Title: r.Title, Source: r.Service, SourceID: r.ID, URL: r.Page, DownloadURL: src,
		Author: r.Author, AuthorURL: r.AuthorURL, Attribution: r.Attribution, Preview: isPreview,
		RetrievedAt: time.Now().UTC(), SHA256: hex.EncodeToString(h.Sum(nil)),
	}
	r.Terms.Apply(&e)
	if e.Attribution == "" && (e.AttributionRequired || r.Service == "pexels" || r.Service == "unsplash") {
		e.Attribution = credit(r)
	}
	return e, nil
}

func credit(r Result) string {
	switch r.Service {
	case "pexels":
		return "Photo by " + r.Author + " on Pexels"
	case "unsplash":
		return "Photo by " + r.Author + " on Unsplash"
	}
	e := library.Entry{Title: r.Title, Author: r.Author, Source: r.Service, Licence: r.Terms.ID}
	return library.DefaultAttribution(e)
}

var unsafe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(unsafe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	return s
}

func fileName(r Result, ctype, disposition, src string) string {
	ext := ""
	if u, err := url.Parse(src); err == nil {
		ext = strings.ToLower(path.Ext(u.Path))
	}
	if m := regexp.MustCompile(`filename="?([^";]+)`).FindStringSubmatch(disposition); m != nil {
		ext = strings.ToLower(filepath.Ext(m[1]))
	}
	if ext == "" || len(ext) > 5 {
		switch {
		case strings.Contains(ctype, "jpeg"):
			ext = ".jpg"
		case strings.Contains(ctype, "png"):
			ext = ".png"
		case strings.Contains(ctype, "webp"):
			ext = ".webp"
		case strings.Contains(ctype, "mpeg"):
			ext = ".mp3"
		case strings.Contains(ctype, "wav"):
			ext = ".wav"
		case strings.Contains(ctype, "ogg"):
			ext = ".ogg"
		default:
			ext = ".bin"
		}
	}
	id := r.ID
	if len(id) > 12 {
		id = id[:12]
	}
	name := r.Service + "-" + slug(id)
	if s := slug(r.Title); s != "" {
		name += "-" + s
	}
	return name + ext
}

func q(v string) string { return url.QueryEscape(v) }

func asHTTP(err error, target **services.HTTPError) bool { return errors.As(err, target) }
