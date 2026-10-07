// Package library records every asset a project imports or generates: where it came from,
// its licence, and the credit line it needs. The record lives in .cav/manifest.json at the
// project root; the licence the project accepts lives in .cav/project.json.
package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Project is .cav/project.json.
type Project struct {
	// Licence is "nc-ok" (non-commercial use: NC assets pass) or "commercial".
	Licence string `json:"licence,omitempty"`
}

// Generated describes how an AI asset was made, so it can be made again.
type Generated struct {
	Provider string   `json:"provider"`
	Model    string   `json:"model,omitempty"`
	Prompt   string   `json:"prompt,omitempty"`
	Seed     *int64   `json:"seed,omitempty"`
	Refs     []string `json:"refs,omitempty"`
}

// Entry is one asset.
type Entry struct {
	Path                string     `json:"path"`
	Kind                string     `json:"kind"` // image, audio, video, svg
	Title               string     `json:"title,omitempty"`
	Source              string     `json:"source"`
	SourceID            string     `json:"sourceId,omitempty"`
	URL                 string     `json:"url,omitempty"`
	DownloadURL         string     `json:"downloadUrl,omitempty"`
	Licence             string     `json:"licence"`
	LicenceURL          string     `json:"licenceUrl,omitempty"`
	Author              string     `json:"author,omitempty"`
	AuthorURL           string     `json:"authorUrl,omitempty"`
	Attribution         string     `json:"attribution,omitempty"`
	AttributionRequired bool       `json:"attributionRequired"`
	CommercialOK        bool       `json:"commercialOk"`
	ShareAlike          bool       `json:"shareAlike,omitempty"`
	NoDerivatives       bool       `json:"noDerivatives,omitempty"`
	Preview             bool       `json:"preview,omitempty"` // a lower-quality preview, not the original file
	Generated           *Generated `json:"generated,omitempty"`
	RetrievedAt         time.Time  `json:"retrievedAt"`
	SHA256              string     `json:"sha256,omitempty"`
}

type Manifest struct {
	Assets []Entry `json:"assets"`
}

// Root finds the project root: the nearest folder (from the working directory up) that has a
// .cav folder, or else the working directory.
func Root() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for dir := wd; ; {
		if info, err := os.Stat(filepath.Join(dir, ".cav")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return wd
		}
		dir = parent
	}
}

func dir(root string) string          { return filepath.Join(root, ".cav") }
func ManifestPath(root string) string { return filepath.Join(dir(root), "manifest.json") }
func ProjectPath(root string) string  { return filepath.Join(dir(root), "project.json") }

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func LoadProject(root string) (*Project, error) {
	p := &Project{}
	return p, readJSON(ProjectPath(root), p)
}

func SaveProject(root string, p *Project) error {
	if p.Licence != "nc-ok" && p.Licence != "commercial" {
		return fmt.Errorf("licence must be nc-ok or commercial, got %q", p.Licence)
	}
	return writeJSON(ProjectPath(root), p)
}

func Load(root string) (*Manifest, error) {
	m := &Manifest{}
	if err := readJSON(ManifestPath(root), m); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manifest) Save(root string) error {
	sort.SliceStable(m.Assets, func(i, j int) bool { return m.Assets[i].Path < m.Assets[j].Path })
	return writeJSON(ManifestPath(root), m)
}

// Put adds the entry, replacing any entry with the same path. Paths are stored relative to
// the project root with forward slashes.
func (m *Manifest) Put(root string, e Entry) Entry {
	e.Path = Rel(root, e.Path)
	for i := range m.Assets {
		if m.Assets[i].Path == e.Path {
			m.Assets[i] = e
			return e
		}
	}
	m.Assets = append(m.Assets, e)
	return e
}

// Rel makes a path relative to the project root when it is inside it.
func Rel(root, p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	if r, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(abs)
}

// Problem is one licence issue.
type Problem struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"` // nc-in-commercial, share-alike, no-derivatives, missing-credit, missing-file, unknown-licence
	Detail string `json:"detail"`
}

// Check compares the assets with the licence the project accepts. A missing project setting
// is treated as commercial, the stricter case, and says so.
func Check(root string, m *Manifest, p *Project) []Problem {
	var out []Problem
	licence := p.Licence
	for _, e := range m.Assets {
		path := e.Path
		if !filepath.IsAbs(filepath.FromSlash(path)) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		if _, err := os.Stat(path); err != nil {
			out = append(out, Problem{e.Path, "missing-file", "the manifest lists it, but the file is gone"})
		}
		if e.Generated != nil {
			continue
		}
		if e.Licence == "" || e.Licence == "unknown" {
			out = append(out, Problem{e.Path, "unknown-licence", "no licence recorded: check the source before using it"})
			continue
		}
		if !e.CommercialOK && licence != "nc-ok" {
			msg := e.Licence + " allows non-commercial use only"
			if licence == "" {
				msg += " (the project sets no licence, so cav assumes commercial; `cav credits licence nc-ok` if it is non-commercial)"
			}
			out = append(out, Problem{e.Path, "nc-in-commercial", msg})
		}
		if e.NoDerivatives {
			out = append(out, Problem{e.Path, "no-derivatives", e.Licence + " forbids changed versions: use it unedited (no trims, effects or recolours), or replace it"})
		}
		if e.ShareAlike {
			out = append(out, Problem{e.Path, "share-alike", e.Licence + " requires the finished piece to use the same licence"})
		}
		if e.AttributionRequired && strings.TrimSpace(e.Attribution) == "" {
			out = append(out, Problem{e.Path, "missing-credit", e.Licence + " requires a credit, and none is recorded"})
		}
	}
	return out
}

// Credits returns one credit line per asset that needs or deserves one, in path order.
func Credits(m *Manifest, all bool) []string {
	var lines []string
	seen := map[string]bool{}
	for _, e := range m.Assets {
		if e.Generated != nil && !all {
			continue
		}
		line := e.Attribution
		if line == "" && (e.AttributionRequired || all) {
			line = DefaultAttribution(e)
		}
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		lines = append(lines, line)
	}
	return lines
}

// DefaultAttribution builds a TASL credit: title, author, source, licence.
func DefaultAttribution(e Entry) string {
	if e.Generated != nil {
		s := "Generated with " + e.Generated.Provider
		if e.Generated.Model != "" {
			s += " (" + e.Generated.Model + ")"
		}
		return s
	}
	title := e.Title
	if title == "" {
		title = filepath.Base(e.Path)
	}
	s := "“" + title + "”"
	if e.Author != "" {
		s += " by " + e.Author
	}
	if e.Source != "" {
		s += " (" + e.Source + ")"
	}
	if e.Licence != "" {
		s += ", " + LicenceName(e.Licence)
	}
	return s
}
