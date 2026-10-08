package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
)

// ReferenceOnly marks an image collected to look at, not cleared for use in a piece: Are.na
// blocks carry no licence, and most are other people's work.
var ReferenceOnly = library.Terms{ID: "reference-only"}

type arenaBlock struct {
	ID             int    `json:"id"`
	Class          string `json:"class"`
	Title          string `json:"title"`
	GeneratedTitle string `json:"generated_title"`
	Source         *struct {
		URL string `json:"url"`
	} `json:"source"`
	User *struct {
		FullName string `json:"full_name"`
		Slug     string `json:"slug"`
	} `json:"user"`
	Image *struct {
		ContentType string `json:"content_type"`
		Display     struct {
			URL string `json:"url"`
		} `json:"display"`
		Original struct {
			URL string `json:"url"`
		} `json:"original"`
	} `json:"image"`
}

// ArenaSlug takes a channel slug or an are.na URL (https://www.are.na/<user>/<channel>).
func ArenaSlug(s string) string {
	s = strings.TrimSpace(s)
	if u, err := url.Parse(s); err == nil && strings.HasSuffix(u.Host, "are.na") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		return parts[len(parts)-1]
	}
	return strings.Trim(s, "/")
}

// ArenaChannel lists the images of a public Are.na channel (image blocks, and the preview
// images of links and attachments), newest first, up to limit.
func ArenaChannel(ctx context.Context, c *services.Config, key, channel string, limit int) ([]Result, error) {
	slug := ArenaSlug(channel)
	if slug == "" {
		return nil, fmt.Errorf("give a channel slug or URL")
	}
	h := map[string]string{}
	if key != "" {
		h["Authorization"] = "Bearer " + key
	}
	var out []Result
	for page := 1; len(out) < limit && page <= 20; page++ {
		var r struct {
			Contents []arenaBlock `json:"contents"`
		}
		u := fmt.Sprintf("https://api.are.na/v2/channels/%s/contents?per=50&page=%d", url.PathEscape(slug), page)
		if err := services.Do(ctx, "GET", u, h, nil, &r); err != nil {
			var he *services.HTTPError
			if asHTTP(err, &he) && (he.Status == 401 || he.Status == 403) {
				return nil, fmt.Errorf("the channel %q is private: add an Are.na token (cav config set-key arena ...)", slug)
			}
			return nil, err
		}
		if len(r.Contents) == 0 {
			break
		}
		for _, b := range r.Contents {
			if b.Image == nil || (b.Image.Original.URL == "" && b.Image.Display.URL == "") {
				continue
			}
			title := b.Title
			if title == "" {
				title = b.GeneratedTitle
			}
			res := Result{Service: "arena", ID: strconv.Itoa(b.ID), Kind: "image", Title: title,
				Page: "https://www.are.na/block/" + strconv.Itoa(b.ID), Preview: b.Image.Display.URL, Original: b.Image.Original.URL,
				Terms: ReferenceOnly, Licence: ReferenceOnly.ID}
			if b.User != nil {
				res.Author = b.User.FullName
			}
			if b.Source != nil && b.Source.URL != "" {
				res.Page = b.Source.URL
			}
			out = append(out, res)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
