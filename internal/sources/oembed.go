package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rock3r/cav/internal/moodboard"
	"github.com/rock3r/cav/internal/services"
)

// oEmbed endpoints. They are variables so a test can point them at a local server.
var (
	YouTubeOEmbed = "https://www.youtube.com/oembed"
	VimeoOEmbed   = "https://vimeo.com/api/oembed.json"
)

// maxThumb is the most bytes cav reads for a thumbnail.
const maxThumb = 8 << 20

// Embed is what a video service tells about a video through oEmbed. cav never downloads the
// video: it keeps the title, the author and a thumbnail to look at.
type Embed struct {
	Provider  string
	ID        string
	Hash      string
	PageURL   string
	Title     string
	Author    string
	AuthorURL string
	ThumbURL  string
}

// OEmbed asks YouTube or Vimeo about a video. The question is built from the id taken out of
// the link, never from the link as written.
func OEmbed(ctx context.Context, pageURL string) (Embed, error) {
	provider, id, hash, ok := moodboard.ParseVideoURL(pageURL)
	if !ok {
		return Embed{}, errors.New("only YouTube and Vimeo links can be embedded")
	}
	e := Embed{Provider: provider, ID: id, Hash: hash, PageURL: moodboard.PageURL(provider, id, hash)}
	endpoint := YouTubeOEmbed
	if provider == moodboard.Vimeo {
		endpoint = VimeoOEmbed
	}
	var r struct {
		Title        string `json:"title"`
		AuthorName   string `json:"author_name"`
		AuthorURL    string `json:"author_url"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	q := url.Values{"format": {"json"}, "url": {e.PageURL}}
	if err := services.Do(ctx, "GET", endpoint+"?"+q.Encode(), nil, nil, &r); err != nil {
		return e, err
	}
	e.Title, e.Author, e.AuthorURL = r.Title, r.AuthorName, r.AuthorURL
	if AllowedThumb(r.ThumbnailURL) {
		e.ThumbURL = r.ThumbnailURL
	}
	return e, nil
}

// AllowedThumb says whether cav may fetch a thumbnail from this address: https, on the image
// hosts of YouTube and Vimeo. The address comes from a service's answer, so it is checked.
func AllowedThumb(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "i.ytimg.com" || h == "i.vimeocdn.com" || strings.HasSuffix(h, ".vimeocdn.com")
}

// SaveThumb downloads a thumbnail into out. It refuses other hosts (also after a redirect),
// answers that are not images, and anything over a size cap.
func SaveThumb(ctx context.Context, thumbURL, out string) error {
	if !AllowedThumb(thumbURL) {
		return fmt.Errorf("not a thumbnail address cav fetches: %s", thumbURL)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", thumbURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", services.UserAgent)
	c := *services.HTTPClient
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || !AllowedThumb(r.URL.String()) {
			return errors.New("the thumbnail moved to a host cav does not fetch")
		}
		return nil
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("thumbnail: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxThumb+1))
	if err != nil {
		return err
	}
	if len(data) > maxThumb {
		return errors.New("thumbnail is too large")
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return errors.New("the thumbnail is not an image")
	}
	if err := moodboard.CheckThumbWrite(out); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	// The folder was made after the first check, so check again. The file is written next to
	// its place and renamed over it: a rename replaces a link itself and never follows it.
	if err := moodboard.CheckThumbWrite(out); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(out), ".thumb-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), out); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
