package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/music"
	"github.com/rock3r/cav/internal/services"
	"github.com/rock3r/cav/internal/sources"
)

func init() {
	register(command{
		name:    "sfx",
		args:    "search <words> [--service S] [--licence any|commercial|nc-ok] [--max-seconds N] [--limit N] | get <service:id> [-o sfx/] [--original] | gen \"<prompt>\" [--seconds 2] [--loop] | add <file> --licence ID [--author A] [--source S] [--url U] | index <folder> [--licence ID] | libraries",
		summary: "Find sound effects and music under free licences, download them, and record their credits.",
		run:     func(a *app, args []string) error { return cmdSources(a, "sfx", "audio", "sfx", args) },
	})
	longHelp["sfx"] = `
cav sfx search whoosh --max-seconds 3
    searches the first usable sound source (see cav config: freesound needs a key,
    openverse needs none) and your indexed local libraries. Each hit has a ref such as
    openverse:eab8a6e2-… or freesound:351256, its length, licence and author.
cav sfx get <ref> [-o sfx/] [--original]
    downloads it and records it in .cav/manifest.json with its licence and credit line.
    Freesound gives a high-quality MP3 preview to anyone; --original needs
    cav config login freesound.
cav sfx index ~/Sounds/Sonniss-GDC-2026 [--licence Sonniss-GDC]
    adds a folder you downloaded (for example a Sonniss GDC bundle: royalty-free, no credit
    needed) to the local search. Files stay where they are until you get one.
cav sfx gen "deep whoosh into a soft impact" --seconds 1.5 [--loop]
    generates a sound with ElevenLabs (needs its key), into sfx/, recorded as generated.
cav sfx add <file> --licence CC-BY-4.0 --author "..." [--source ...] [--url ...]
    records a file you found by hand.
The licence filter defaults to the project setting (cav credits licence nc-ok|commercial):
commercial hides NC sounds; nc-ok shows them. Without a setting, everything shows and NC is
marked. cav credits prints the credit lines; cav check warns about licences that do not fit.`
	register(command{
		name:    "ref",
		args:    "search <words> [--service S] [--licence any|commercial|nc-ok] [--limit N] | get <service:id> [-o moodboard/] [--original] | arena <channel or URL> [--limit 30] | add <file> --licence ID [--author A] [--source S] [--url U]",
		summary: "Find reference images for mood boards, download them, and record their credits.",
		run:     func(a *app, args []string) error { return cmdSources(a, "ref", "image", "moodboard", args) },
	})
	longHelp["ref"] = `
cav ref search "neon city night" searches the first usable image source: pexels and
unsplash with a key, openverse and wikimedia without one (--service picks one).
cav ref get <ref> [-o moodboard/] downloads it (a large preview; --original for the full
file) and records licence and credit in .cav/manifest.json. cav board mood lays the folder
out as one picture. Reference images are for looking at: check the licence before putting
one in the final piece.
cav ref arena https://www.are.na/<user>/<channel> [--limit 30] copies a channel's images
into moodboard/ (a public channel needs no token; ARENA_TOKEN or cav config set-key arena
for a private one). They are recorded as reference-only: Are.na blocks carry no licence.`
	register(command{
		name:    "credits",
		args:    "[show] [--all] [-o CREDITS.txt] | check | licence nc-ok|commercial | list",
		summary: "Print the credit lines for the assets a project uses, and check their licences.",
		run:     cmdCredits,
	})
	longHelp["credits"] = `
Every asset that cav downloads or generates is recorded in .cav/manifest.json (at the
project root: the nearest folder with a .cav folder, or the current one).
  cav credits                   the credit lines that licences require (--all: every asset)
  cav credits -o CREDITS.txt    write them to a file
  cav credits check             licence problems: NC in a commercial project, share-alike,
                                no-derivatives, missing credits, missing files (exit 1)
  cav credits licence nc-ok     this project is non-commercial: NC assets are fine
  cav credits licence commercial
  cav credits list              every recorded asset
cav check reports the same licence problems with the scene findings.`
}

func projectFilter(root string) string {
	p, err := library.LoadProject(root)
	if err != nil || p.Licence == "" {
		return "any"
	}
	return p.Licence
}

func cmdSources(a *app, cmd, kind, defaultDir string, args []string) error {
	if len(args) == 0 {
		return usageErr("usage: cav %s search <words> | get <service:id> | add <file>", cmd)
	}
	sub, args := args[0], args[1:]
	root := library.Root()
	switch sub {
	case "search":
		return sourcesSearch(a, cmd, kind, root, args)
	case "get":
		return sourcesGet(a, cmd, kind, defaultDir, root, args)
	case "add":
		return sourcesAdd(a, kind, root, args)
	case "index":
		if kind != "audio" {
			break
		}
		return sfxIndex(a, args)
	case "arena":
		if kind != "image" {
			break
		}
		return refArena(a, root, defaultDir, args)
	case "gen":
		if kind != "audio" {
			break
		}
		return sfxGen(a, root, defaultDir, args)
	case "libraries":
		if kind != "audio" {
			break
		}
		libs, err := loadLibraries()
		if err != nil {
			return err
		}
		a.emit(map[string]any{"libraries": libs}, func() {
			if len(libs) == 0 {
				fmt.Println("no local libraries; add one with cav sfx index <folder>")
			}
			for _, l := range libs {
				fmt.Printf("%s  (%s)\n", l.Path, l.Licence)
			}
		})
		return nil
	}
	return usageErr("unknown %s command %q", cmd, sub)
}

func sourcesSearch(a *app, cmd, kind, root string, args []string) error {
	fs := flag.NewFlagSet(cmd+" search", flag.ContinueOnError)
	service := fs.String("service", "", "search this source only")
	licence := fs.String("licence", "", "any, commercial or nc-ok (default: the project setting)")
	maxSec := fs.Float64("max-seconds", 0, "longest sound to show")
	limit := fs.Int("limit", 12, "how many results")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageErr("give some words to search for")
	}
	if *licence == "" {
		*licence = projectFilter(root)
	}
	if *licence != "any" && *licence != "commercial" && *licence != "nc-ok" {
		return usageErr("--licence must be any, commercial or nc-ok")
	}
	q := sources.Query{Text: strings.Join(pos, " "), Kind: kind, Limit: *limit, Licence: *licence, MaxSeconds: *maxSec}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var results []sources.Result
	var notes []string
	if kind == "audio" && (*service == "" || *service == "local") {
		local, err := searchLocal(q)
		if err != nil {
			notes = append(notes, "local libraries: "+err.Error())
		}
		results = append(results, local...)
	}
	used := ""
	if *service != "local" {
		c, err := services.Load()
		if err != nil {
			return err
		}
		job := "sfx"
		if kind == "image" {
			job = "ref"
		}
		ch, err := services.Pick(ctx, c, job, *service)
		if err != nil {
			if len(results) == 0 {
				return fail(exitError, err.Error(), "see cav config")
			}
			notes = append(notes, err.Error())
		} else {
			used = ch.Service
			for _, s := range ch.Skipped {
				notes = append(notes, "skipped "+s)
			}
			rs, err := sources.Search(ctx, c, ch.Service, ch.Key, q)
			if err != nil {
				return fail(exitError, ch.Service+": "+err.Error(), "")
			}
			results = append(results, rs...)
		}
	}
	if len(results) > *limit {
		results = results[:*limit]
	}
	a.emit(map[string]any{"service": used, "licenceFilter": *licence, "results": results, "notes": notes}, func() {
		for _, n := range notes {
			fmt.Fprintln(os.Stderr, "note: "+n)
		}
		if len(results) == 0 {
			fmt.Printf("no results for %q (licence filter: %s)\n", q.Text, *licence)
			return
		}
		var from []string
		if used != "" {
			from = append(from, used)
		}
		for _, r := range results {
			if r.Service == "local" {
				from = append(from, "local libraries")
				break
			}
		}
		fmt.Printf("%d results from %s (licence filter: %s)\n", len(results), strings.Join(from, " and "), *licence)
		for _, r := range results {
			extra := ""
			if r.Seconds > 0 {
				extra = fmt.Sprintf("%5.1fs", r.Seconds)
			} else if r.Width > 0 {
				extra = fmt.Sprintf("%dx%d", r.Width, r.Height)
			}
			mark := ""
			if !r.Terms.CommercialOK {
				mark = " [non-commercial]"
			}
			fmt.Printf("%-48s %-9s %-14s %s%s\n", r.Ref(), extra, r.Licence, clip(r.Title, 50), mark)
			if r.Author != "" {
				fmt.Printf("%-48s by %s\n", "", clip(r.Author, 60))
			}
		}
		fmt.Printf("\nDownload one with: cav %s get <ref>\n", cmd)
	})
	return nil
}

func sourcesGet(a *app, cmd, kind, defaultDir, root string, args []string) error {
	fs := flag.NewFlagSet(cmd+" get", flag.ContinueOnError)
	out := fs.String("o", defaultDir, "folder to save into")
	original := fs.Bool("original", false, "download the original file instead of the preview")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav %s get <service:id>", cmd)
	}
	service, id, ok := strings.Cut(pos[0], ":")
	if !ok {
		return usageErr("a ref looks like openverse:<id> or freesound:<id> (from cav %s search)", cmd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	var e library.Entry
	if service == "local" {
		e, err = getLocal(id, *out)
	} else {
		c, lerr := services.Load()
		if lerr != nil {
			return lerr
		}
		key := ""
		if d, ok := services.Catalog[service]; ok && d.Kind == services.KindKey {
			if key, err = services.Resolve(ctx, service, c.KeySource(service)); err != nil {
				return fail(exitError, service+": "+err.Error(), "see cav config")
			}
		}
		r, lerr := sources.Lookup(ctx, c, service, key, kind, id)
		if lerr != nil {
			return fail(exitError, pos[0]+": "+lerr.Error(), "")
		}
		e, err = sources.Download(ctx, c, key, r, *out, *original)
	}
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	return recordAndReport(a, root, e)
}

func recordAndReport(a *app, root string, e library.Entry) error {
	m, err := library.Load(root)
	if err != nil {
		return err
	}
	e = m.Put(root, e)
	if err := m.Save(root); err != nil {
		return err
	}
	p, _ := library.LoadProject(root)
	probs := library.Check(root, &library.Manifest{Assets: []library.Entry{e}}, p)
	a.emit(map[string]any{"asset": e, "manifest": library.ManifestPath(root), "problems": probs}, func() {
		fmt.Printf("saved %s\n  licence: %s", e.Path, library.LicenceName(e.Licence))
		if e.Preview {
			fmt.Print(" (preview file)")
		}
		fmt.Println()
		if e.Attribution != "" {
			fmt.Printf("  credit:  %s\n", e.Attribution)
		}
		for _, p := range probs {
			fmt.Printf("  warning: %s\n", p.Detail)
		}
	})
	return nil
}

func sourcesAdd(a *app, kind, root string, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	lic := fs.String("licence", "", "licence id (CC0-1.0, CC-BY-4.0, CC-BY-NC-4.0, Pexels, ...), a Creative Commons URL, or a name")
	author := fs.String("author", "", "who made it")
	source := fs.String("source", "manual", "where it came from")
	url := fs.String("url", "", "the page it came from")
	credit := fs.String("credit", "", "the exact credit line to use")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *lic == "" {
		return usageErr("usage: cav sfx|ref add <file> --licence ID [--author A] [--source S] [--url U] [--credit TEXT]")
	}
	if _, err := os.Stat(pos[0]); err != nil {
		return fail(exitError, "cannot read "+pos[0], "")
	}
	e := library.Entry{Path: pos[0], Kind: kind, Title: strings.TrimSuffix(filepath.Base(pos[0]), filepath.Ext(pos[0])),
		Source: *source, URL: *url, Author: *author, Attribution: *credit, RetrievedAt: time.Now().UTC()}
	termsFor(*lic).Apply(&e)
	if e.Attribution == "" && e.AttributionRequired {
		e.Attribution = library.DefaultAttribution(e)
	}
	return recordAndReport(a, root, e)
}

func sfxGen(a *app, root, dir string, args []string) error {
	fs := flag.NewFlagSet("sfx gen", flag.ContinueOnError)
	seconds := fs.Float64("seconds", 0, "length, 0.5-30 (default: the model decides)")
	loop := fs.Bool("loop", false, "make it loop smoothly")
	out := fs.String("o", dir, "folder")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageErr("describe the sound: cav sfx gen \"glassy riser into a hit\"")
	}
	prompt := strings.Join(pos, " ")
	c, err := services.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ch, err := services.Pick(ctx, c, "sfx", "elevenlabs")
	if err != nil {
		return fail(exitError, err.Error(), "sound generation needs an ElevenLabs key (cav config); cav sfx search finds free sounds")
	}
	tr, err := music.SoundEffect(ctx, ch.Key, prompt, *seconds, *loop)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	path := filepath.Join(*out, safeSlug(prompt)+tr.Ext)
	if err := os.WriteFile(path, tr.Data, 0o644); err != nil {
		return err
	}
	sum, _ := fileSHA(path)
	return recordAndReport(a, root, library.Entry{Path: path, Kind: "audio", Title: prompt, Source: tr.Provider, Licence: "generated",
		CommercialOK: true, RetrievedAt: time.Now().UTC(), SHA256: sum, Generated: &library.Generated{Provider: tr.Provider, Model: tr.Model, Prompt: prompt}})
}

func refArena(a *app, root, dir string, args []string) error {
	fs := flag.NewFlagSet("ref arena", flag.ContinueOnError)
	limit := fs.Int("limit", 30, "most images to copy")
	out := fs.String("o", dir, "folder")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav ref arena <channel-slug or https://www.are.na/user/channel>")
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	key, _ := services.Resolve(ctx, "arena", c.KeySource("arena"))
	rs, err := sources.ArenaChannel(ctx, c, key, pos[0], *limit)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	m, err := library.Load(root)
	if err != nil {
		return err
	}
	var saved, failed []string
	for _, r := range rs {
		e, err := sources.Download(ctx, c, key, r, *out, true)
		if err != nil {
			failed = append(failed, r.Ref()+": "+err.Error())
			continue
		}
		e = m.Put(root, e)
		saved = append(saved, e.Path)
	}
	if err := m.Save(root); err != nil {
		return err
	}
	a.emit(map[string]any{"channel": sources.ArenaSlug(pos[0]), "saved": saved, "failed": failed}, func() {
		fmt.Printf("copied %d images from are.na/%s into %s (recorded as reference-only)\n", len(saved), sources.ArenaSlug(pos[0]), *out)
		for _, f := range failed {
			fmt.Fprintln(os.Stderr, "skipped "+f)
		}
		fmt.Println("Lay them out: cav board mood " + *out)
	})
	return nil
}

// termsFor reads a licence given on the command line.
func termsFor(s string) library.Terms {
	switch strings.ToLower(s) {
	case "pexels":
		return library.PexelsTerms
	case "unsplash":
		return library.UnsplashTerms
	case "sonniss-gdc", "royalty-free":
		return library.Terms{ID: s, CommercialOK: true}
	}
	if strings.Contains(s, "creativecommons.org") {
		return library.FromURL(s)
	}
	if t := library.FromName(strings.NewReplacer("-", " ").Replace(s)); t.ID != "unknown" {
		return t
	}
	return library.Terms{ID: s}
}

func cmdCredits(a *app, args []string) error {
	sub := "show"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	root := library.Root()
	m, err := library.Load(root)
	if err != nil {
		return err
	}
	switch sub {
	case "show":
		fs := flag.NewFlagSet("credits", flag.ContinueOnError)
		all := fs.Bool("all", false, "a line for every asset, not only those whose licence requires one")
		out := fs.String("o", "", "write the lines to this file")
		if _, err := parseFlags(fs, args); err != nil {
			return err
		}
		lines := library.Credits(m, *all)
		if *out != "" {
			text := strings.Join(lines, "\n")
			if text != "" {
				text += "\n"
			}
			if err := os.WriteFile(*out, []byte(text), 0o644); err != nil {
				return err
			}
		}
		a.emit(map[string]any{"credits": lines, "file": *out}, func() {
			if len(lines) == 0 {
				fmt.Println("no credits needed (no recorded asset requires one)")
			}
			for _, l := range lines {
				fmt.Println(l)
			}
			if *out != "" {
				fmt.Fprintf(os.Stderr, "wrote %s\n", *out)
			}
		})
		return nil
	case "check":
		p, err := library.LoadProject(root)
		if err != nil {
			return err
		}
		probs := library.Check(root, m, p)
		a.emit(map[string]any{"licence": p.Licence, "assets": len(m.Assets), "problems": probs}, func() {
			for _, pr := range probs {
				fmt.Printf("%-16s %s: %s\n", pr.Kind, pr.Path, pr.Detail)
			}
			fmt.Printf("%d assets, %d problems (project licence: %s)\n", len(m.Assets), len(probs), orDefault(p.Licence, "not set, checked as commercial"))
		})
		if len(probs) > 0 {
			return &cliError{code: exitError, msg: "", data: map[string]any{"problems": probs}}
		}
		return nil
	case "licence", "license":
		if len(args) != 1 {
			return usageErr("usage: cav credits licence nc-ok|commercial")
		}
		if err := library.SaveProject(root, &library.Project{Licence: args[0]}); err != nil {
			return usageErr("%v", err)
		}
		a.emit(map[string]any{"licence": args[0], "file": library.ProjectPath(root)}, func() {
			fmt.Printf("project licence: %s (%s)\n", args[0], library.ProjectPath(root))
		})
		return nil
	case "list":
		a.emit(map[string]any{"assets": m.Assets}, func() {
			for _, e := range m.Assets {
				src := e.Source
				if e.Generated != nil {
					src = "generated by " + e.Generated.Provider
				}
				fmt.Printf("%-50s %-14s %s\n", e.Path, e.Licence, src)
			}
		})
		return nil
	}
	return usageErr("unknown credits command %q (show, check, licence, list)", sub)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// licenceFindings turns licence problems into cav check findings. No manifest, no findings.
func licenceFindings() []finding {
	root := library.Root()
	if _, err := os.Stat(library.ManifestPath(root)); err != nil {
		return nil
	}
	m, err := library.Load(root)
	if err != nil {
		return []finding{{Kind: "licence", Detail: err.Error(), Fix: "fix or delete " + library.ManifestPath(root)}}
	}
	p, _ := library.LoadProject(root)
	var out []finding
	for _, pr := range library.Check(root, m, p) {
		fix := "replace the asset, or record the right licence with cav sfx|ref add"
		switch pr.Kind {
		case "nc-in-commercial":
			fix = "replace it with a CC0 or CC-BY asset, or set `cav credits licence nc-ok` if the project is non-commercial"
		case "missing-credit":
			fix = "record the credit: cav sfx|ref add <file> --licence ... --credit \"...\""
		case "missing-file":
			fix = "restore the file or remove it from .cav/manifest.json"
		}
		out = append(out, finding{Kind: "licence", Name: pr.Path, Detail: pr.Detail, Fix: fix, Severity: "warning"})
	}
	return out
}

// ---------- local sound libraries ----------

type soundLibrary struct {
	Path    string `json:"path"`
	Licence string `json:"licence"`
}

func librariesPath() string {
	return filepath.Join(filepath.Dir(services.Path()), "sound-libraries.json")
}

func loadLibraries() ([]soundLibrary, error) {
	var libs []soundLibrary
	b, err := os.ReadFile(librariesPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return libs, jsonUnmarshal(b, &libs)
}

func sfxIndex(a *app, args []string) error {
	fs := flag.NewFlagSet("sfx index", flag.ContinueOnError)
	lic := fs.String("licence", "Sonniss-GDC", "the licence of every file in the folder")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav sfx index <folder> [--licence ID]")
	}
	abs, err := filepath.Abs(pos[0])
	if err != nil {
		return err
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return fail(exitError, abs+" is not a folder", "")
	}
	libs, err := loadLibraries()
	if err != nil {
		return err
	}
	kept := libs[:0]
	for _, l := range libs {
		if l.Path != abs {
			kept = append(kept, l)
		}
	}
	kept = append(kept, soundLibrary{Path: abs, Licence: *lic})
	if err := writeJSONFile(librariesPath(), kept); err != nil {
		return err
	}
	n := 0
	walkAudio(abs, func(string) bool { n++; return true })
	a.emit(map[string]any{"library": abs, "licence": *lic, "files": n}, func() {
		fmt.Printf("indexed %s: %d sound files (%s); cav sfx search now looks there first\n", abs, n, *lic)
	})
	return nil
}

var audioExt = map[string]bool{".wav": true, ".aif": true, ".aiff": true, ".flac": true, ".mp3": true, ".ogg": true, ".m4a": true}

func walkAudio(root string, fn func(path string) bool) {
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if audioExt[strings.ToLower(filepath.Ext(p))] && !fn(p) {
			return io.EOF
		}
		return nil
	})
}

func searchLocal(q sources.Query) ([]sources.Result, error) {
	libs, err := loadLibraries()
	if err != nil || len(libs) == 0 {
		return nil, err
	}
	words := strings.Fields(strings.ToLower(q.Text))
	var out []sources.Result
	for _, l := range libs {
		terms := termsFor(l.Licence)
		if !terms.Allowed(q.Licence) {
			continue
		}
		walkAudio(l.Path, func(p string) bool {
			rel, _ := filepath.Rel(l.Path, p)
			hay := strings.ToLower(rel)
			for _, w := range words {
				if !strings.Contains(hay, w) {
					return true
				}
			}
			out = append(out, sources.Result{Service: "local", ID: p, Kind: "audio", Title: filepath.Base(p),
				Page: p, Original: p, Terms: terms, Licence: terms.ID})
			return len(out) < q.Limit
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func getLocal(path, dir string) (library.Entry, error) {
	libs, err := loadLibraries()
	if err != nil {
		return library.Entry{}, err
	}
	var lib *soundLibrary
	for i := range libs {
		if strings.HasPrefix(path, libs[i].Path+string(filepath.Separator)) {
			lib = &libs[i]
		}
	}
	if lib == nil {
		return library.Entry{}, fmt.Errorf("%s is not in an indexed library (cav sfx libraries)", path)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return library.Entry{}, err
	}
	dst := filepath.Join(dir, filepath.Base(path))
	if err := copyFile(path, dst); err != nil {
		return library.Entry{}, err
	}
	sum, _ := fileSHA(dst)
	e := library.Entry{Path: dst, Kind: "audio", Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Source: "local library " + filepath.Base(lib.Path), DownloadURL: path, RetrievedAt: time.Now().UTC(), SHA256: sum}
	termsFor(lib.Licence).Apply(&e)
	return e, nil
}
