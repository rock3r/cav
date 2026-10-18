package main

// The earshot sound bank as a sound source: 201 short sounds for motion graphics (clicks,
// whooshes, hits, risers, UI ticks, glitches, ...), rendered locally by earshot's forge.
// Unlike the remote sources it needs no key and works offline, and unlike `cav sfx gen`
// each sound can be rendered to an exact length. The forge sounds are code-synthesized
// (no recording inside); the fit/* sounds are recipes fitted to CC0 recordings, whose
// source each entry names. Search matches the bank's own descriptions; the by-sound CLAP
// ranking is for the indexed local libraries.
//
// Setup is the mflux pattern: explicit `cav sfx search --service bank ...` or
// `cav sfx get bank:<id>` downloads the pinned earshot source (about 4 MB) into the cav
// cache; after that the bank joins every plain `cav sfx search`. Rendering needs numpy and
// scipy only (through uv), so the bank never pulls the CLAP models.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/pyrun"
	"github.com/rock3r/cav/internal/services"
	"github.com/rock3r/cav/internal/sources"
)

// bankTerms covers the sounds the forge synthesizes in code: they ship with earshot under
// its MIT licence and carry no credit. fit/* entries override this with their CC0 source.
var bankTerms = library.Terms{ID: "MIT", CommercialOK: true}

// bankDir is where the pinned earshot source lives (versioned by commit, so a pin bump
// fetches cleanly next to the old one).
func bankDir() string {
	return filepath.Join(config.CacheDir(), "earshot-bank", earshotCommit[:12])
}

// bankReady reports whether the bank can join a plain search without setup.
func bankReady() bool {
	_, err := os.Stat(filepath.Join(bankDir(), "earshot", "forge", "bank"))
	return err == nil
}

// ensureBank downloads and extracts the pinned earshot source when missing, and returns
// its directory.
func ensureBank(ctx context.Context) (string, error) {
	dir := bankDir()
	if bankReady() {
		return dir, nil
	}
	url := "https://codeload.github.com/Leonxlnx/earshot/tar.gz/" + earshotCommit
	fmt.Fprintln(os.Stderr, "fetching the earshot sound bank (about 4 MB, once)…")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := services.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download the earshot source: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download the earshot source: %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	// Stage beside the target: os.Rename cannot cross filesystems.
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".cav-earshot")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := untar(resp.Body, tmp); err != nil {
		return "", fmt.Errorf("extract the earshot source: %w", err)
	}
	top, err := os.ReadDir(tmp)
	if err != nil || len(top) != 1 {
		return "", fmt.Errorf("extract the earshot source: unexpected layout")
	}
	// The target only ever appears through this rename, so it is never partial. If the
	// rename fails because a concurrent install finished first, that install is fine.
	if err := os.Rename(filepath.Join(tmp, top[0].Name()), dir); err != nil {
		if bankReady() {
			return dir, nil
		}
		return "", err
	}
	return dir, nil
}

// untar extracts a .tar.gz of regular files and directories into dir, refusing anything
// that would escape it.
func untar(r io.Reader, dir string) error {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p := filepath.Join(dir, filepath.FromSlash(h.Name))
		if !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			f.Close()
			if err != nil {
				return err
			}
		}
	}
}

type bankSound struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Member   string   `json:"member"`
	Type     string   `json:"type"`
	DurS     float64  `json:"dur_s"`
	Kinds    []string `json:"kinds"`
	Notes    string   `json:"notes"`
	Source   *struct {
		Title   string `json:"title"`
		Author  string `json:"author"`
		License string `json:"license"`
		URL     string `json:"url"`
	} `json:"source"`
}

// bankList runs the helper's list mode and returns the bank with its category texts.
func bankList(ctx context.Context) ([]bankSound, map[string]string, string, error) {
	dir, err := ensureBank(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	script, err := assets.Python.ReadFile("python/sfx_bank.py")
	if err != nil {
		return nil, nil, "", err
	}
	out, err := pyrun.Run(ctx, filepath.Join(config.CacheDir(), "python"), "sfx_bank.py", script,
		[]string{"numpy", "scipy"}, []string{dir, "list"}, nil)
	if err != nil {
		return nil, nil, "", err
	}
	var r struct {
		Version    string `json:"version"`
		Categories map[string]struct {
			Target string `json:"target"`
			Use    string `json:"use"`
		} `json:"categories"`
		Sounds []bankSound `json:"sounds"`
	}
	if err := jsonUnmarshal(out, &r); err != nil {
		return nil, nil, "", fmt.Errorf("unexpected bank list output: %w", err)
	}
	texts := make(map[string]string, len(r.Categories))
	for c, info := range r.Categories {
		texts[c] = info.Target + " " + info.Use
	}
	return r.Sounds, texts, r.Version, nil
}

// searchBank matches the query against the bank's ids, kinds and description texts (the
// same all-words rule as the local file-name search).
func searchBank(ctx context.Context, q sources.Query) ([]sources.Result, error) {
	sounds, cats, _, err := bankList(ctx)
	if err != nil {
		return nil, err
	}
	words := strings.Fields(strings.ToLower(q.Text))
	var out []sources.Result
	for _, s := range sounds {
		hay := strings.ToLower(s.ID + " " + strings.Join(s.Kinds, " ") + " " + s.Notes + " " + cats[s.Category])
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if q.MaxSeconds > 0 && s.DurS > q.MaxSeconds {
			continue
		}
		terms := bankTerms
		author := ""
		if s.Source != nil {
			terms = library.Terms{ID: "CC0-1.0", CommercialOK: true}
			author = s.Source.Author
		}
		title := s.ID
		if s.Notes != "" {
			title += " — " + s.Notes
		}
		out = append(out, sources.Result{Service: "bank", ID: s.ID, Kind: "audio", Title: title,
			Author: author, Seconds: s.DurS, Terms: terms, Licence: terms.ID})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

// bankGet renders a bank sound into dir (with an exact length when asked) and records it
// like a generated take: the forge sounds as MIT-covered code output, a fit sound with its
// CC0 source. The audit's warnings come back as notes.
func bankGet(ctx context.Context, id, dir string, seconds float64, seed int) (library.Entry, []string, error) {
	bdir, err := ensureBank(ctx)
	if err != nil {
		return library.Entry{}, nil, err
	}
	script, err := assets.Python.ReadFile("python/sfx_bank.py")
	if err != nil {
		return library.Entry{}, nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return library.Entry{}, nil, err
	}
	name := strings.NewReplacer("/", "-", " ", "-").Replace(id)
	out := filepath.Join(dir, name+".wav")
	// Reserve the take name with an exclusive create, so a concurrent get cannot pick
	// the same path; the renderer then overwrites the placeholder.
	var guard *os.File
	for i := 2; ; i++ {
		guard, err = os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return library.Entry{}, nil, err
		}
		out = filepath.Join(dir, fmt.Sprintf("%s-%d.wav", name, i))
	}
	guard.Close()
	keep := false
	defer func() {
		if !keep {
			os.Remove(out)
		}
	}()
	durArg, seedArg := "-", "-"
	if seconds > 0 {
		durArg = fmt.Sprintf("%g", seconds)
	}
	var seedPtr *int64
	if seed >= 0 {
		seedArg = fmt.Sprint(seed)
		s := int64(seed)
		seedPtr = &s
	}
	stdout, err := pyrun.Run(ctx, filepath.Join(config.CacheDir(), "python"), "sfx_bank.py", script,
		[]string{"numpy", "scipy"}, []string{bdir, "render", id, out, durArg, seedArg}, nil)
	if err != nil {
		return library.Entry{}, nil, err
	}
	var r struct {
		Seconds float64 `json:"seconds"`
		Checks  struct {
			OK       bool     `json:"ok"`
			Warnings []string `json:"warnings"`
			Notes    []string `json:"notes"`
		} `json:"checks"`
		Source *struct {
			Title   string `json:"title"`
			Author  string `json:"author"`
			License string `json:"license"`
			URL     string `json:"url"`
		} `json:"source"`
	}
	if err := jsonUnmarshal(stdout, &r); err != nil {
		return library.Entry{}, nil, fmt.Errorf("unexpected bank render output: %w", err)
	}
	if info, err := os.Stat(out); err != nil || info.Size() == 0 { // the placeholder is empty
		return library.Entry{}, nil, fmt.Errorf("the renderer did not write %s", out)
	}
	sum, _ := fileSHA(out)
	e := library.Entry{Path: out, Kind: "audio", Title: id, Source: "earshot sound bank",
		RetrievedAt: time.Now().UTC(), SHA256: sum,
		Generated: &library.Generated{Provider: "earshot", Model: "sound bank", Prompt: id, Seed: seedPtr}}
	if r.Source != nil {
		e.Title = r.Source.Title
		e.Author = r.Source.Author
		e.URL = r.Source.URL
		e.Licence = "CC0-1.0"
		e.CommercialOK = true
		e.LicenceNote = "a recording by " + r.Source.Author + " fitted to code by earshot (CC0: no credit needed)"
	} else {
		e.Licence = "MIT"
		e.CommercialOK = true
		e.LicenceNote = "synthesized in code by earshot's forge (no recording inside); ships under earshot's MIT licence"
	}
	var notes []string
	for _, w := range r.Checks.Warnings {
		notes = append(notes, "the bank's own audit flags this take: "+w)
	}
	keep = true
	return e, notes, nil
}
