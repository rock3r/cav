package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/docs"
	"github.com/rock3r/cav/internal/search"
)

func init() {
	register(command{
		name:    "api",
		args:    "<words> [-n 8] [--ns api|cavalry|ui|ctx|def|render]",
		summary: "Search the scripting API (names, signatures, cav notes). Works offline.",
		run:     cmdAPI,
	})
	register(command{
		name:    "docs",
		args:    "<words> [-n 5] [--full] | update",
		summary: "Search the Cavalry docs offline (build the index once with `cav docs update`).",
		run:     cmdDocs,
	})
	longHelp["docs"] = `
cav docs <words>      search the local docs index (node pages show their layer type id)
cav docs update       download the docs to ~/.cav/cache/docs and rebuild the index
                      (about 520 pages; pages are cached for a week)
The docs belong to Scene Group / Canva and are not shipped with cav. The index is
built on your own computer, for your own offline use.`
	register(command{
		name:    "types",
		args:    "[words]",
		summary: "List or search Cavalry layer type ids (for api.create / cav.create).",
		run:     cmdTypes,
	})
}

type apiEntry struct {
	NS        string   `json:"ns"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Sig       string   `json:"sig"`
	Members   []string `json:"members,omitempty"`
	Overloads []string `json:"overloads,omitempty"`
	Live      *bool    `json:"live,omitempty"`
	Untyped   bool     `json:"untyped,omitempty"`
	Note      string   `json:"note,omitempty"`
	URL       string   `json:"url,omitempty"`
}

func loadAPI() []apiEntry {
	var ref struct {
		Entries []apiEntry `json:"entries"`
	}
	_ = json.Unmarshal(assets.APIRef, &ref)
	return ref.Entries
}

func cmdAPI(a *app, args []string) error {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	n := fs.Int("n", 8, "number of results")
	ns := fs.String("ns", "", "only this namespace")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageErr("usage: cav api <words>, for example: cav api bounding box")
	}
	entries := loadAPI()
	var docs []search.Doc
	var idx []int
	for i, e := range entries {
		if *ns != "" && e.NS != *ns {
			continue
		}
		boost := 1.0
		switch e.NS {
		case "api":
			boost = 1.5
		case "cavalry":
			boost = 1.2
		}
		if e.Note != "" {
			boost *= 1.2
		}
		docs = append(docs, search.Doc{Name: e.Name, Head: e.Sig + " " + strings.Join(e.Members, " "), Body: e.Note, Boost: boost})
		idx = append(idx, i)
	}
	q := strings.Join(pos, " ")
	q = strings.TrimPrefix(strings.TrimPrefix(q, "api."), "cavalry.")
	hits := search.New(docs).Search(q, *n)
	var res []apiEntry
	for _, h := range hits {
		res = append(res, entries[idx[h.Index]])
	}
	docsHint := "For concepts and node attributes, try `cav docs " + q + "`."
	a.emit(map[string]any{"query": q, "results": res, "hint": docsHint}, func() {
		if len(res) == 0 {
			fmt.Println("no match. " + docsHint)
			return
		}
		for _, e := range res {
			sig := e.Sig
			if e.Kind == "class" {
				sig = "class " + e.Name
			}
			fmt.Printf("%s.%s\n    %s\n", e.NS, e.Name, strings.TrimPrefix(sig, "class "))
			for _, o := range e.Overloads {
				fmt.Printf("    %s\n", o)
			}
			for _, m := range e.Members {
				fmt.Printf("      .%s\n", m)
			}
			if e.Live != nil && !*e.Live {
				fmt.Println("    (not found in Cavalry 2.7.2)")
			}
			if e.Note != "" {
				fmt.Printf("    note: %s\n", e.Note)
			}
		}
		fmt.Println("\n" + docsHint)
	})
	return nil
}

func typeByName() map[string]string {
	var lt struct {
		Types []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"types"`
	}
	_ = json.Unmarshal(assets.LayerTypes, &lt)
	m := map[string]string{}
	for _, t := range lt.Types {
		m[strings.ToLower(t.Name)] = t.Type
	}
	return m
}

func cmdDocs(a *app, args []string) error {
	fs := flag.NewFlagSet("docs", flag.ContinueOnError)
	n := fs.Int("n", 5, "number of results")
	full := fs.Bool("full", false, "print whole sections instead of snippets")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 1 && pos[0] == "update" {
		return docsUpdate(a)
	}
	if len(pos) == 0 {
		return usageErr("usage: cav docs <words> | cav docs update")
	}
	ix, err := docs.Load(config.DocsDir())
	if err != nil {
		return fail(exitError, "the docs index is not built yet", "run `cav docs update` once (needs internet, about a minute)")
	}
	var sd []search.Doc
	for _, c := range ix.Chunks {
		sd = append(sd, search.Doc{
			Name:  c.Title + " " + c.Type,
			Title: c.Title,
			Head:  c.Heading,
			Body:  c.Text,
		})
	}
	q := strings.Join(pos, " ")
	hits := search.New(sd).Search(q, *n)
	type out struct {
		docs.Chunk
		Score float64 `json:"score"`
	}
	var res []out
	for _, h := range hits {
		c := ix.Chunks[h.Index]
		if !*full && len(c.Text) > 700 {
			c.Text = c.Text[:700] + " …"
		}
		res = append(res, out{c, float64(int(h.Score*100)) / 100})
	}
	a.emit(map[string]any{"query": q, "results": res}, func() {
		if len(res) == 0 {
			fmt.Println("no match")
			return
		}
		for _, r := range res {
			typ := ""
			if r.Type != "" {
				typ = "  [layer type: " + r.Type + "]"
			}
			fmt.Printf("== %s%s\n   %s\n%s\n\n", r.Heading, typ, r.URL, indent(r.Text, "   "))
		}
	})
	return nil
}

func indent(s, pre string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = pre + lines[i]
	}
	return strings.Join(lines, "\n")
}

func docsUpdate(a *app) error {
	start := time.Now()
	last := time.Time{}
	ix, err := docs.Build(context.Background(), config.DocsDir(), typeByName(), func(done, total int) {
		if !a.json && time.Since(last) > 2*time.Second {
			last = time.Now()
			fmt.Fprintf(os.Stderr, "cav: docs %d/%d pages\n", done, total)
		}
	})
	if err != nil {
		return fail(exitError, "docs update failed: "+err.Error(), "check the internet connection and try again")
	}
	a.emit(map[string]any{"pages": ix.Pages, "sections": len(ix.Chunks), "path": docs.Path(config.DocsDir())}, func() {
		fmt.Printf("docs index: %d pages, %d sections in %s (%.0f s)\n", ix.Pages, len(ix.Chunks), docs.Path(config.DocsDir()), time.Since(start).Seconds())
	})
	return nil
}

func docsCheck(build bool) check {
	ix, err := docs.Load(config.DocsDir())
	if err == nil {
		return check{Name: "docs", OK: true, Optional: true, Detail: fmt.Sprintf("%d sections, built %s", len(ix.Chunks), ix.BuiltAt)}
	}
	if !build {
		return check{Name: "docs", Optional: true, Detail: "no local docs index", Fix: "cav docs update"}
	}
	ix, err = docs.Build(context.Background(), config.DocsDir(), typeByName(), nil)
	if err != nil {
		return check{Name: "docs", Optional: true, Detail: "could not build the docs index: " + err.Error(), Fix: "cav docs update"}
	}
	return check{Name: "docs", OK: true, Optional: true, Detail: fmt.Sprintf("built: %d pages, %d sections", ix.Pages, len(ix.Chunks))}
}

func cmdTypes(a *app, args []string) error {
	var lt struct {
		Types []struct {
			Type         string `json:"type"`
			Name         string `json:"name"`
			Experimental bool   `json:"experimental,omitempty"`
		} `json:"types"`
	}
	_ = json.Unmarshal(assets.LayerTypes, &lt)
	q := strings.ToLower(strings.Join(args, " "))
	type row struct {
		Type         string `json:"type"`
		Name         string `json:"name"`
		Experimental bool   `json:"experimental,omitempty"`
	}
	var res []row
	for _, t := range lt.Types {
		if q == "" || strings.Contains(strings.ToLower(t.Name), q) || strings.Contains(strings.ToLower(t.Type), q) {
			res = append(res, row(t))
		}
	}
	a.emit(map[string]any{"types": res}, func() {
		for _, r := range res {
			exp := ""
			if r.Experimental {
				exp = "  (experimental)"
			}
			fmt.Printf("%-32s %s%s\n", r.Type, r.Name, exp)
		}
	})
	return nil
}
