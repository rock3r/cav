// Package docs builds and loads a local, offline index of the Cavalry documentation.
//
// The docs are not redistributed with cav. `cav docs update` downloads the pages from
// cavalry.studio to the user's own computer and splits them into sections.
package docs

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const SitemapURL = "https://cavalry.studio/docs/sitemap.xml"

type Chunk struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Heading string `json:"heading"`
	Text    string `json:"text"`
	Type    string `json:"type,omitempty"` // Cavalry layer type id for node pages
}

type Index struct {
	BuiltAt string  `json:"builtAt"`
	Source  string  `json:"source"`
	Pages   int     `json:"pages"`
	Chunks  []Chunk `json:"chunks"`
}

func Path(dir string) string { return filepath.Join(dir, "index.json") }

func Load(dir string) (*Index, error) {
	b, err := os.ReadFile(Path(dir))
	if err != nil {
		return nil, err
	}
	var ix Index
	if err := json.Unmarshal(b, &ix); err != nil {
		return nil, err
	}
	return &ix, nil
}

type Progress func(done, total int)

// Build downloads every page in the sitemap and writes the index to dir.
// typeByName maps a node's display name (e.g. "Duplicator") to its layer type id.
func Build(ctx context.Context, dir string, typeByName map[string]string, progress Progress) (*Index, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	sm, err := getCached(ctx, client, SitemapURL, filepath.Join(dir, "pages"))
	if err != nil {
		return nil, fmt.Errorf("sitemap: %w", err)
	}
	var urls []string
	for _, m := range regexp.MustCompile(`<loc>([^<]+)</loc>`).FindAllStringSubmatch(sm, -1) {
		urls = append(urls, strings.TrimSpace(m[1]))
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("sitemap has no pages")
	}
	cacheDir := filepath.Join(dir, "pages")
	_ = os.MkdirAll(cacheDir, 0o755)
	type page struct {
		url, body string
	}
	pages := make([]page, len(urls))
	var mu sync.Mutex
	done := 0
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var firstErr error
	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			body, err := getCached(ctx, client, u, cacheDir)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			pages[i] = page{u, body}
			done++
			if progress != nil {
				progress(done, len(urls))
			}
		}(i, u)
	}
	wg.Wait()
	ix := &Index{BuiltAt: time.Now().UTC().Format(time.RFC3339), Source: SitemapURL}
	ok := 0
	for _, p := range pages {
		if p.body == "" {
			continue
		}
		ok++
		ix.Chunks = append(ix.Chunks, Split(p.url, p.body, typeByName)...)
	}
	ix.Pages = ok
	if ok == 0 {
		return nil, fmt.Errorf("could not download any page: %v", firstErr)
	}
	b, _ := json.Marshal(ix)
	if err := os.WriteFile(Path(dir), b, 0o644); err != nil {
		return nil, err
	}
	return ix, nil
}

func get(ctx context.Context, c *http.Client, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "cav-docs/0.1 (offline docs index for one user)")
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// getCached keeps each page for a week, so a second `cav docs update` is fast and light.
func getCached(ctx context.Context, c *http.Client, url, dir string) (string, error) {
	name := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.TrimPrefix(url, "https://"), "_") + ".html"
	p := filepath.Join(dir, name)
	if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < 7*24*time.Hour {
		b, err := os.ReadFile(p)
		return string(b), err
	}
	body, err := get(ctx, c, url)
	if err != nil {
		return "", err
	}
	_ = os.WriteFile(p, []byte(body), 0o644)
	return body, nil
}

var (
	reArticle = regexp.MustCompile(`(?s)<article[^>]*>(.*)</article>`)
	reNav     = regexp.MustCompile(`(?s)<nav[^>]*>.*?</nav>`)
	reTOC     = regexp.MustCompile(`(?s)<div[^>]*theme-doc-toc-mobile[^>]*>.*?</div>`)
	reTitle   = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	reHead    = regexp.MustCompile(`(?s)<h([23])[^>]*>(.*?)</h[23]>`)
	rePre     = regexp.MustCompile(`(?s)<pre[^>]*>(.*?)</pre>`)
	reBreak   = regexp.MustCompile(`(?i)<br\s*/?>|</(p|li|tr|div|h[1-6]|table|ul|ol)>`)
	reCell    = regexp.MustCompile(`(?i)</t[dh]>`)
	reLi      = regexp.MustCompile(`(?i)<li[^>]*>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace   = regexp.MustCompile(`[ \t]+`)
	reBlank   = regexp.MustCompile(`\n{3,}`)
	reHash    = regexp.MustCompile(`\s*(Direct link to.*|\x{200B})$`)
)

// Split turns one HTML page into sections, one per h2/h3 heading.
func Split(url, page string, typeByName map[string]string) []Chunk {
	m := reArticle.FindStringSubmatch(page)
	if m == nil {
		return nil
	}
	body := reNav.ReplaceAllString(m[1], "")
	body = reTOC.ReplaceAllString(body, "")
	title := ""
	if t := reTitle.FindStringSubmatch(body); t != nil {
		title = clean(t[1])
		body = strings.Replace(body, t[0], "", 1)
	}
	// Keep code blocks: mark them before stripping tags.
	body = rePre.ReplaceAllStringFunc(body, func(s string) string {
		inner := rePre.FindStringSubmatch(s)[1]
		inner = regexp.MustCompile(`(?i)</span>\s*<span[^>]*class="token-line"`).ReplaceAllString(inner, "\n<span")
		return "\n```\n" + strings.TrimSpace(html.UnescapeString(reTag.ReplaceAllString(reBreak.ReplaceAllString(inner, "\n"), ""))) + "\n```\n"
	})
	typ := typeByName[strings.ToLower(title)]
	var chunks []Chunk
	locs := reHead.FindAllStringSubmatchIndex(body, -1)
	emit := func(heading, raw string) {
		text := toText(raw)
		if len(strings.TrimSpace(text)) < 20 {
			return
		}
		for _, part := range splitLong(text, 1500) {
			chunks = append(chunks, Chunk{URL: url, Title: title, Heading: heading, Text: part, Type: typ})
		}
	}
	prev, heading := 0, title
	for _, l := range locs {
		emit(heading, body[prev:l[0]])
		heading = title + " > " + clean(body[l[4]:l[5]])
		prev = l[1]
	}
	emit(heading, body[prev:])
	return chunks
}

func clean(s string) string {
	s = html.UnescapeString(reTag.ReplaceAllString(s, ""))
	s = reHash.ReplaceAllString(strings.TrimSpace(s), "")
	return strings.TrimSpace(strings.ReplaceAll(s, "\u200b", ""))
}

func toText(s string) string {
	s = reLi.ReplaceAllString(s, "\n- ")
	s = reCell.ReplaceAllString(s, " | ")
	s = reBreak.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u200b", "")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(reSpace.ReplaceAllString(l, " "), " ")
	}
	s = strings.Join(lines, "\n")
	s = reBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func splitLong(s string, max int) []string {
	if len(s) <= max {
		return []string{s}
	}
	var out []string
	var cur strings.Builder
	for _, para := range strings.Split(s, "\n\n") {
		if cur.Len() > 0 && cur.Len()+len(para) > max {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
		cur.WriteString(para + "\n\n")
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

// Titles lists the page titles, sorted, for diagnostics.
func (ix *Index) Titles() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range ix.Chunks {
		if !seen[c.Title] {
			seen[c.Title] = true
			out = append(out, c.Title)
		}
	}
	sort.Strings(out)
	return out
}
