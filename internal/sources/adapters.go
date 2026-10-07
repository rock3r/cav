package sources

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
)

func init() {
	adapters["openverse"] = adapter{kinds: []string{"audio", "image"}, search: openverseSearch, lookup: openverseLookup}
	adapters["freesound"] = adapter{kinds: []string{"audio"}, search: freesoundSearch, lookup: freesoundLookup}
	adapters["pexels"] = adapter{kinds: []string{"image"}, search: pexelsSearch, lookup: pexelsLookup}
	adapters["unsplash"] = adapter{kinds: []string{"image"}, search: unsplashSearch, lookup: unsplashLookup}
	adapters["wikimedia"] = adapter{kinds: []string{"image"}, search: wikimediaSearch, lookup: wikimediaLookup}
}

// ---------- Openverse: https://api.openverse.org/v1/ ----------

type ovItem struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Landing     string  `json:"foreign_landing_url"`
	URL         string  `json:"url"`
	Thumbnail   string  `json:"thumbnail"`
	Creator     string  `json:"creator"`
	CreatorURL  string  `json:"creator_url"`
	License     string  `json:"license"`
	Version     string  `json:"license_version"`
	Attribution string  `json:"attribution"`
	Duration    float64 `json:"duration"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Source      string  `json:"source"`
	Tags        []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

func (it ovItem) result(kind string) Result {
	r := Result{Service: "openverse", ID: it.ID, Kind: kind, Title: it.Title, Author: it.Creator, AuthorURL: it.CreatorURL,
		Page: it.Landing, Preview: it.URL, Original: it.URL, Terms: library.FromCC(it.License, it.Version),
		Attribution: it.Attribution, Seconds: it.Duration / 1000, Width: it.Width, Height: it.Height}
	for i, t := range it.Tags {
		if i >= 8 {
			break
		}
		r.Tags = append(r.Tags, t.Name)
	}
	if it.Source != "" {
		r.Title += " (via " + it.Source + ")"
	}
	return r
}

func ovPath(kind string) string {
	if kind == "audio" {
		return "audio"
	}
	return "images"
}

func openverseSearch(ctx context.Context, _ *services.Config, _ string, qq Query) ([]Result, error) {
	u := fmt.Sprintf("https://api.openverse.org/v1/%s/?q=%s&page_size=%d&mature=false", ovPath(qq.Kind), q(qq.Text), min(qq.Limit*2, 50))
	if qq.Licence == "commercial" {
		u += "&license_type=commercial"
	}
	var r struct {
		Results []ovItem `json:"results"`
	}
	if err := services.Do(ctx, "GET", u, nil, nil, &r); err != nil {
		return nil, err
	}
	var out []Result
	for _, it := range r.Results {
		out = append(out, it.result(qq.Kind))
	}
	return out, nil
}

func openverseLookup(ctx context.Context, _ *services.Config, _, kind, id string) (Result, error) {
	var it ovItem
	if err := services.Do(ctx, "GET", fmt.Sprintf("https://api.openverse.org/v1/%s/%s/", ovPath(kind), q(id)), nil, nil, &it); err != nil {
		return Result{}, err
	}
	return it.result(kind), nil
}

// ---------- Freesound: https://freesound.org/docs/api/ ----------

const fsFields = "id,name,username,license,duration,previews,url,type,tags"

type fsItem struct {
	ID       int               `json:"id"`
	Name     string            `json:"name"`
	Username string            `json:"username"`
	License  string            `json:"license"`
	Duration float64           `json:"duration"`
	Previews map[string]string `json:"previews"`
	URL      string            `json:"url"`
	Type     string            `json:"type"`
	Tags     []string          `json:"tags"`
}

func (it fsItem) result() Result {
	r := Result{Service: "freesound", ID: strconv.Itoa(it.ID), Kind: "audio", Title: it.Name, Author: it.Username,
		AuthorURL: "https://freesound.org/people/" + it.Username + "/", Page: it.URL, Preview: it.Previews["preview-hq-mp3"],
		Original: fmt.Sprintf("https://freesound.org/apiv2/sounds/%d/download/", it.ID), Terms: library.FromURL(it.License),
		Seconds: it.Duration}
	if len(it.Tags) > 8 {
		it.Tags = it.Tags[:8]
	}
	r.Tags = it.Tags
	if r.Terms.AttributionRequired {
		r.Attribution = fmt.Sprintf("“%s” by %s (freesound.org), %s", it.Name, it.Username, library.LicenceName(r.Terms.ID))
	}
	return r
}

func freesoundSearch(ctx context.Context, c *services.Config, key string, qq Query) ([]Result, error) {
	u := fmt.Sprintf("https://freesound.org/apiv2/search/text/?query=%s&page_size=%d&fields=%s", q(qq.Text), min(qq.Limit*2, 150), fsFields)
	var filters []string
	if qq.MaxSeconds > 0 {
		filters = append(filters, fmt.Sprintf("duration:[0 TO %g]", qq.MaxSeconds))
	}
	if qq.Licence == "commercial" {
		filters = append(filters, `(license:"Creative Commons 0" OR license:"Attribution")`)
	}
	if len(filters) > 0 {
		u += "&filter=" + q(strings.Join(filters, " "))
	}
	var r struct {
		Results []fsItem `json:"results"`
	}
	if err := services.Do(ctx, "GET", u, services.FreesoundAuth(c, key), nil, &r); err != nil {
		return nil, err
	}
	var out []Result
	for _, it := range r.Results {
		out = append(out, it.result())
	}
	return out, nil
}

func freesoundLookup(ctx context.Context, c *services.Config, key, _, id string) (Result, error) {
	var it fsItem
	if err := services.Do(ctx, "GET", fmt.Sprintf("https://freesound.org/apiv2/sounds/%s/?fields=%s", q(id), fsFields), services.FreesoundAuth(c, key), nil, &it); err != nil {
		return Result{}, err
	}
	return it.result(), nil
}

// ---------- Pexels: https://www.pexels.com/api/documentation/ ----------

type pxPhoto struct {
	ID              int    `json:"id"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	URL             string `json:"url"`
	Photographer    string `json:"photographer"`
	PhotographerURL string `json:"photographer_url"`
	Alt             string `json:"alt"`
	Src             struct {
		Original string `json:"original"`
		Large2x  string `json:"large2x"`
	} `json:"src"`
}

func (p pxPhoto) result() Result {
	title := p.Alt
	if title == "" {
		title = "Pexels photo " + strconv.Itoa(p.ID)
	}
	return Result{Service: "pexels", ID: strconv.Itoa(p.ID), Kind: "image", Title: title, Author: p.Photographer,
		AuthorURL: p.PhotographerURL, Page: p.URL, Preview: p.Src.Large2x, Original: p.Src.Original,
		Terms: library.PexelsTerms, Width: p.Width, Height: p.Height}
}

func pexelsSearch(ctx context.Context, _ *services.Config, key string, qq Query) ([]Result, error) {
	var r struct {
		Photos []pxPhoto `json:"photos"`
	}
	u := fmt.Sprintf("https://api.pexels.com/v1/search?query=%s&per_page=%d", q(qq.Text), min(qq.Limit, 80))
	if err := services.Do(ctx, "GET", u, map[string]string{"Authorization": key}, nil, &r); err != nil {
		return nil, err
	}
	var out []Result
	for _, p := range r.Photos {
		out = append(out, p.result())
	}
	return out, nil
}

func pexelsLookup(ctx context.Context, _ *services.Config, key, _, id string) (Result, error) {
	var p pxPhoto
	if err := services.Do(ctx, "GET", "https://api.pexels.com/v1/photos/"+q(id), map[string]string{"Authorization": key}, nil, &p); err != nil {
		return Result{}, err
	}
	return p.result(), nil
}

// ---------- Unsplash: https://unsplash.com/documentation ----------

type usPhoto struct {
	ID          string `json:"id"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Description string `json:"description"`
	Alt         string `json:"alt_description"`
	URLs        struct {
		Full    string `json:"full"`
		Regular string `json:"regular"`
	} `json:"urls"`
	Links struct {
		HTML     string `json:"html"`
		Download string `json:"download_location"`
	} `json:"links"`
	User struct {
		Name  string `json:"name"`
		Links struct {
			HTML string `json:"html"`
		} `json:"links"`
	} `json:"user"`
}

func (p usPhoto) result() Result {
	title := p.Description
	if title == "" {
		title = p.Alt
	}
	return Result{Service: "unsplash", ID: p.ID, Kind: "image", Title: title, Author: p.User.Name, AuthorURL: p.User.Links.HTML,
		Page: p.Links.HTML, Preview: p.URLs.Regular, Original: p.URLs.Full, Terms: library.UnsplashTerms,
		Width: p.Width, Height: p.Height, track: p.Links.Download}
}

func usHeaders(key string) map[string]string {
	return map[string]string{"Authorization": "Client-ID " + key, "Accept-Version": "v1"}
}

func unsplashSearch(ctx context.Context, _ *services.Config, key string, qq Query) ([]Result, error) {
	var r struct {
		Results []usPhoto `json:"results"`
	}
	u := fmt.Sprintf("https://api.unsplash.com/search/photos?query=%s&per_page=%d", q(qq.Text), min(qq.Limit, 30))
	if err := services.Do(ctx, "GET", u, usHeaders(key), nil, &r); err != nil {
		return nil, err
	}
	var out []Result
	for _, p := range r.Results {
		out = append(out, p.result())
	}
	return out, nil
}

func unsplashLookup(ctx context.Context, _ *services.Config, key, _, id string) (Result, error) {
	var p usPhoto
	if err := services.Do(ctx, "GET", "https://api.unsplash.com/photos/"+q(id), usHeaders(key), nil, &p); err != nil {
		return Result{}, err
	}
	return p.result(), nil
}

// ---------- Wikimedia Commons: https://commons.wikimedia.org/w/api.php ----------

type wmPage struct {
	PageID    int    `json:"pageid"`
	Title     string `json:"title"`
	ImageInfo []struct {
		URL      string `json:"url"`
		ThumbURL string `json:"thumburl"`
		DescURL  string `json:"descriptionurl"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Mime     string `json:"mime"`
		Meta     map[string]struct {
			Value string `json:"value"`
		} `json:"extmetadata"`
	} `json:"imageinfo"`
}

var tags = regexp.MustCompile(`<[^>]+>`)

func (p wmPage) result() (Result, bool) {
	if len(p.ImageInfo) == 0 {
		return Result{}, false
	}
	ii := p.ImageInfo[0]
	if !strings.HasPrefix(ii.Mime, "image/") {
		return Result{}, false
	}
	meta := func(k string) string {
		return strings.TrimSpace(html.UnescapeString(tags.ReplaceAllString(ii.Meta[k].Value, "")))
	}
	terms := library.FromName(meta("LicenseShortName"))
	if terms.ID == "unknown" && meta("LicenseUrl") != "" {
		terms = library.FromURL(meta("LicenseUrl"))
	}
	if meta("AttributionRequired") == "true" {
		terms.AttributionRequired = true
	}
	title := meta("ObjectName")
	if title == "" {
		title = strings.TrimPrefix(p.Title, "File:")
	}
	r := Result{Service: "wikimedia", ID: strconv.Itoa(p.PageID), Kind: "image", Title: title, Author: meta("Artist"),
		Page: ii.DescURL, Preview: ii.ThumbURL, Original: ii.URL, Terms: terms, Width: ii.Width, Height: ii.Height}
	if terms.AttributionRequired {
		r.Attribution = fmt.Sprintf("“%s” by %s, Wikimedia Commons, %s", title, r.Author, library.LicenceName(terms.ID))
	}
	return r, true
}

const wmProps = "&prop=imageinfo&iiprop=url|extmetadata|size|mime&iiurlwidth=1920&iiextmetadatafilter=LicenseShortName|LicenseUrl|Artist|AttributionRequired|ObjectName"

func wikimediaQuery(ctx context.Context, u string) ([]Result, error) {
	var r struct {
		Query struct {
			Pages []wmPage `json:"pages"`
		} `json:"query"`
	}
	if err := services.Do(ctx, "GET", u, nil, nil, &r); err != nil {
		return nil, err
	}
	var out []Result
	for _, p := range r.Query.Pages {
		if res, ok := p.result(); ok {
			out = append(out, res)
		}
	}
	return out, nil
}

func wikimediaSearch(ctx context.Context, _ *services.Config, _ string, qq Query) ([]Result, error) {
	u := fmt.Sprintf("https://commons.wikimedia.org/w/api.php?action=query&format=json&formatversion=2&generator=search&gsrsearch=%s&gsrnamespace=6&gsrlimit=%d%s",
		q(qq.Text+" filetype:bitmap"), min(qq.Limit*2, 50), wmProps)
	return wikimediaQuery(ctx, u)
}

func wikimediaLookup(ctx context.Context, _ *services.Config, _, _, id string) (Result, error) {
	rs, err := wikimediaQuery(ctx, "https://commons.wikimedia.org/w/api.php?action=query&format=json&formatversion=2&pageids="+q(id)+wmProps)
	if err != nil {
		return Result{}, err
	}
	if len(rs) == 0 {
		return Result{}, fmt.Errorf("no image with page id %s on Wikimedia Commons", id)
	}
	return rs[0], nil
}
