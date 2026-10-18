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

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/moodboard"
	"github.com/rock3r/cav/internal/music"
	"github.com/rock3r/cav/internal/pyrun"
	"github.com/rock3r/cav/internal/services"
	"github.com/rock3r/cav/internal/sources"
)

func init() {
	register(command{
		name:    "sfx",
		args:    "search <words> [--service S] [--licence any|commercial|nc-ok] [--max-seconds N] [--limit N] [--by-sound] | get <service:id> [-o sfx/] [--original] [--seconds N] [--seed N] | gen \"<prompt>\" [--seconds 2] [--loop] | add <file> --licence ID [--author A] [--source S] [--url U] | index <folder> [--licence ID] | libraries",
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
    The local search matches the file names; --by-sound instead ranks the local files by
    what they sound like, so a bundle whose names say nothing (rec_0042.wav) is still
    searchable. It runs earshot's CLAP model locally through uv: the first run downloads
    about 1.4 GB of models into the cav cache, then it works offline and reuses them
    (new or changed files are the only ones read). Each hit gets a calibrated verdict:
    strong means the right kind of sound about 98% of the time (earshot's held-out
    measurement). Without uv, or if earshot fails, the search falls back to file names
    and says so.
cav sfx search whoosh --service bank
    searches the earshot sound bank: 201 short sounds for motion graphics (clicks,
    whooshes, hits, risers, glitches, UI ticks), rendered locally by code — no key, no
    upload, no 1.4 GB models. The first use downloads the pinned earshot source (about
    4 MB, through uv); after that the bank joins every plain search. The forge sounds
    are code-synthesized and ship under earshot's MIT licence; the fit/* sounds are
    recipes fitted to CC0 recordings (their source is recorded with the take).
cav sfx get bank:whoosh_fast/crisp [-o sfx/] [--seconds 1.5] [--seed 7]
    renders the sound into a WAV: --seconds an exact length, --seed a different take of
    it. Every render passes earshot's own model-free audit; its warnings come through as
    notes. Render a take per shot and they stay consistent.
cav sfx gen "deep whoosh into a soft impact" --seconds 1.5 [--loop]
    generates a sound with ElevenLabs (needs its key), into sfx/, recorded as generated.
cav sfx add <file> --licence CC-BY-4.0 --author "..." [--source ...] [--url ...]
    records a file you found by hand.
The licence filter defaults to the project setting (cav credits licence nc-ok|commercial):
commercial hides NC sounds; nc-ok shows them. Without a setting, everything shows and NC is
marked. cav credits prints the credit lines; cav check warns about licences that do not fit.`
	register(command{
		name:    "ref",
		args:    "search <words> [--service S] [--licence any|commercial|nc-ok] [--limit N] | get <service:id> [-o moodboard/] [--original] | arena <channel or URL> [--limit 30] | add <file> --licence ID [--author A] [--source S] [--url U] [--take T] [--by user|brief|agent] | add <youtube or vimeo link> [--take T] [--by user|brief|agent] [--title T]",
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
for a private one). They are recorded as reference-only: Are.na blocks carry no licence.
cav ref add https://www.youtube.com/watch?v=... (or a vimeo.com link) puts a web video on the
mood board as an embed. cav never downloads the video: it asks the site's oEmbed address for
the title and author, saves the thumbnail into moodboard/.thumbs/ (only from the site's own
image hosts), and records it as reference-only. The review page plays it from the site.
Other web links are refused: only YouTube and Vimeo can be embedded; save a picture with
cav ref get. A clip you already have is added like an image: cav ref add clip.mp4 --licence ID.
--take "..." writes the item's take-from-it line at once, and --by says who asked for it:
user (the person gave it), brief (it came from brief.json refs) or agent (the default). If the
site cannot be reached the video is still added, without a thumbnail; give --title by hand.`
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
		return sourcesAdd(a, kind, root, defaultDir, args)
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
	bySound := fs.Bool("by-sound", false, "audio only: rank local library files by what they sound like (earshot's CLAP model through uv; the first run downloads about 1.4 GB of models)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if *bySound && kind != "audio" {
		return usageErr("--by-sound is for cav sfx search")
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
	// Ranking new sounds costs about a second of CPU each the first time (the embeddings
	// are then cached), so a by-sound search gets the same budget as comparing takes.
	timeout := 2 * time.Minute
	if *bySound {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var results []sources.Result
	var notes []string
	if kind == "audio" && (*service == "" || *service == "local") {
		if *bySound {
			local, note := searchLocalBySound(ctx, q)
			if note != "" {
				notes = append(notes, note)
			}
			results = append(results, local...)
		} else if local, err := searchLocal(q); err != nil {
			notes = append(notes, "local libraries: "+err.Error())
		} else {
			results = append(results, local...)
		}
	}
	used := ""
	if kind == "audio" && *service == "bank" {
		// An explicit --service bank pays the one-time setup and its error is fatal.
		bs, err := searchBank(ctx, q)
		if err != nil {
			return fail(exitError, err.Error(), "the bank renders locally through uv; without it, cav sfx search finds free sounds")
		}
		results = append(results, bs...)
		used = "the sound bank"
	} else if kind == "audio" && *service == "" && bankReady() {
		// After the first explicit use the bank joins every search, like mflux.
		if bs, err := searchBank(ctx, q); err != nil {
			notes = append(notes, "sound bank: "+err.Error())
		} else {
			results = append(results, bs...)
		}
	}
	if *service != "local" && *service != "bank" {
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
	a.emit(map[string]any{"service": used, "licenceFilter": *licence, "bySound": *bySound, "results": results, "notes": notes}, func() {
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
		seen := map[string]bool{}
		if used == "the sound bank" {
			seen["bank"] = true // already named; do not list it twice
		}
		for _, r := range results {
			where := map[string]string{"local": "local libraries", "bank": "the sound bank"}[r.Service]
			if where != "" && !seen[r.Service] {
				from = append(from, where)
				seen[r.Service] = true
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
			if r.Sound != nil {
				fmt.Printf("%-48s by sound: %s, z %+.1f; most like %s\n", "", r.Sound.Verdict, r.Sound.Z, clip(r.Sound.SoundsLike, 50))
			}
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
	seconds := fs.Float64("seconds", 0, "bank only: render this length in seconds")
	seed := fs.Int("seed", -1, "bank only: the take's seed (default: the bank's delivered take)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav %s get <service:id>", cmd)
	}
	service, id, ok := strings.Cut(pos[0], ":")
	if !ok {
		return usageErr("a ref looks like openverse:<id>, freesound:<id> or bank:<id> (from cav %s search)", cmd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	var e library.Entry
	var bankNotes []string
	switch service {
	case "local":
		e, err = getLocal(id, *out)
	case "bank":
		if kind != "audio" {
			return usageErr("the bank holds sounds: cav sfx get %s", pos[0])
		}
		if *original {
			return usageErr("--original is for downloads; a bank sound is rendered (try --seconds N)")
		}
		e, bankNotes, err = bankGet(ctx, id, *out, *seconds, *seed)
	default:
		if *seconds > 0 || *seed >= 0 {
			return usageErr("--seconds and --seed are for bank sounds")
		}
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
	for _, n := range bankNotes {
		fmt.Fprintln(os.Stderr, "note: "+n)
	}
	return recordAndReport(a, root, e)
}

func recordAndReport(a *app, root string, e library.Entry) error {
	if err := library.Update(root, func(m *library.Manifest) error {
		e = m.Put(root, e)
		return nil
	}); err != nil {
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

func sourcesAdd(a *app, kind, root, defaultDir string, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	lic := fs.String("licence", "", "licence id (CC0-1.0, CC-BY-4.0, CC-BY-NC-4.0, Pexels, ...), a Creative Commons URL, or a name")
	author := fs.String("author", "", "who made it")
	source := fs.String("source", "manual", "where it came from")
	url := fs.String("url", "", "the page it came from")
	credit := fs.String("credit", "", "the exact credit line to use")
	take := fs.String("take", "", "ref only: what to take from it (the mood board line)")
	by := fs.String("by", "", "ref only: who asked for it: user, brief or agent (default agent)")
	title := fs.String("title", "", "ref only: the title of a web video, if the site cannot be reached")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if kind == "image" {
		if *by != "" && *by != moodboard.ByUser && *by != moodboard.ByBrief && *by != moodboard.ByAgent {
			return usageErr("--by is user, brief or agent")
		}
		if len(pos) == 1 && strings.Contains(pos[0], "://") {
			if _, _, _, ok := moodboard.ParseVideoURL(pos[0]); !ok {
				return fail(exitError, "cav ref add takes a YouTube or Vimeo link, or a file: "+pos[0],
					"only YouTube and Vimeo videos can be embedded; save a picture from a page with cav ref get, or download it yourself and cav ref add <file>")
			}
			return refAddVideo(a, root, defaultDir, pos[0], *take, *by, *title)
		}
	} else if *take != "" || *by != "" || *title != "" {
		return usageErr("--take, --by and --title are for cav ref add")
	}
	if len(pos) != 1 || *lic == "" {
		return usageErr("usage: cav sfx|ref add <file> --licence ID [--author A] [--source S] [--url U] [--credit TEXT]; cav ref add <youtube or vimeo link>")
	}
	if _, err := os.Stat(pos[0]); err != nil {
		return fail(exitError, "cannot read "+pos[0], "")
	}
	e := library.Entry{Path: pos[0], Kind: kind, Title: strings.TrimSuffix(filepath.Base(pos[0]), filepath.Ext(pos[0])),
		Source: *source, URL: *url, Author: *author, Attribution: *credit, RetrievedAt: time.Now().UTC()}
	if kind == "image" && moodboard.KindOfFile(pos[0]) == moodboard.KindClip {
		e.Kind = "video"
	}
	termsFor(*lic).Apply(&e)
	if e.Attribution == "" && e.AttributionRequired {
		e.Attribution = library.DefaultAttribution(e)
	}
	if err := recordAndReport(a, root, e); err != nil {
		return err
	}
	if kind == "image" && (*take != "" || *by != "") {
		return moodMark(defaultDir, "", pos[0], *take, *by, "")
	}
	return nil
}

// moodMark sets the take and the author of an item that was just added, after a sync has
// made the item. The item is found by its file or, for a video, by its id.
func moodMark(dir, id, file, take, by, title string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	jsonFile := moodFile(dir)
	b, _, notes, err := moodSync(ctx, dir, jsonFile)
	if err != nil {
		return err
	}
	for _, n := range notes {
		fmt.Fprintln(os.Stderr, "note: "+n)
	}
	var it *moodboard.Item
	if id != "" {
		it = b.Find(id)
	} else if abs, err := filepath.Abs(file); err == nil {
		for i := range b.Items {
			if b.Items[i].File != "" && filepath.Clean(b.Path(b.Items[i].File)) == abs {
				it = &b.Items[i]
			}
		}
	}
	if it == nil {
		fmt.Fprintf(os.Stderr, "note: %s is not in %s, so --take and --by were not used\n", firstNonEmpty(id, file), dir)
		return nil
	}
	if take != "" {
		it.Take = strings.TrimSpace(take)
	}
	if by != "" {
		it.AddedBy = by
	}
	if title != "" && it.Title == "" {
		it.Title = title
	}
	return b.Save()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// refAddVideo puts a YouTube or Vimeo video on the mood board. It downloads no video: only
// the page's oEmbed answer and the thumbnail from the site's own image host.
func refAddVideo(a *app, root, dir, link, take, by, title string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	emb, oerr := sources.OEmbed(ctx, link)
	if emb.ID == "" {
		return fail(exitError, oerr.Error(), "")
	}
	id := moodboard.EmbedID(emb.Provider, emb.ID)
	var notes []string
	if oerr != nil {
		notes = append(notes, "could not reach "+emb.Provider+" ("+oerr.Error()+"): added without a thumbnail; give --title by hand")
	}
	if emb.Title == "" {
		emb.Title = title
	}
	thumb := ""
	if emb.ThumbURL != "" {
		out := filepath.Join(dir, moodboard.ThumbDir, id+".jpg")
		if err := sources.SaveThumb(ctx, emb.ThumbURL, out); err != nil {
			notes = append(notes, "no thumbnail: "+err.Error())
		} else {
			thumb = out
		}
	}
	credit := ""
	if emb.Author != "" {
		credit = "by " + emb.Author
	}
	if thumb != "" {
		e := library.Entry{Path: thumb, Kind: "embed", Title: emb.Title, Source: emb.Provider, URL: emb.PageURL,
			Author: emb.Author, AuthorURL: emb.AuthorURL, Attribution: credit, RetrievedAt: time.Now().UTC()}
		sources.ReferenceOnly.Apply(&e)
		if err := library.Update(root, func(m *library.Manifest) error { m.Put(root, e); return nil }); err != nil {
			return err
		}
	}
	jsonFile := moodFile(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	m, _ := library.Load(root)
	b, _, err := moodboard.Sync(jsonFile, dir, m, root)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	it := b.FindEmbed(emb.Provider, emb.ID)
	if it == nil {
		// No thumbnail, so no manifest entry: the item is written by hand.
		id = b.UniqueID(id)
		b.Items = append(b.Items, moodboard.Item{ID: id, Kind: moodboard.KindEmbed, URL: emb.PageURL, Title: emb.Title,
			Source: emb.Provider, Licence: sources.ReferenceOnly.ID, Credit: credit, AddedBy: moodboard.ByAgent})
		it = &b.Items[len(b.Items)-1]
	}
	id = it.ID
	// Adding it again refreshes what the site said: a changed link (a Vimeo hash), a title
	// (also one given by hand on a retry), and a thumbnail that arrived this time.
	it.URL = emb.PageURL
	if emb.Title != "" {
		it.Title = emb.Title
	}
	if thumb != "" {
		if abs, err := filepath.Abs(thumb); err == nil {
			if rel, err := filepath.Rel(filepath.Dir(b.FilePath()), abs); err == nil {
				it.Thumb = filepath.ToSlash(rel)
			}
		}
	}
	it.Status = "" // asking for it again brings a dropped video back
	if take != "" {
		it.Take = strings.TrimSpace(take)
	}
	if by != "" {
		it.AddedBy = by
	}
	if err := b.Save(); err != nil {
		return err
	}
	a.emit(map[string]any{"id": id, "kind": moodboard.KindEmbed, "provider": emb.Provider, "url": emb.PageURL, "title": emb.Title,
		"thumb": thumb, "licence": sources.ReferenceOnly.ID, "json": jsonFile, "notes": nonNil(notes)}, func() {
		for _, n := range notes {
			fmt.Fprintln(os.Stderr, "note: "+n)
		}
		fmt.Printf("added %s: %s\n  embedded from %s, not downloaded; reference only\n", id, firstNonEmpty(emb.Title, emb.PageURL), emb.Provider)
		if thumb != "" {
			fmt.Printf("  thumbnail: %s\n", thumb)
		}
		fmt.Printf("  %s\n", jsonFile)
		if take == "" {
			fmt.Printf("write what to take from it: cav board mood take %s \"...\"\n", id)
		}
	})
	return nil
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
	if _, err := library.Load(root); err != nil {
		return err
	}
	var saved, failed []string
	var got []library.Entry
	for _, r := range rs {
		e, err := sources.Download(ctx, c, key, r, *out, true)
		if err != nil {
			failed = append(failed, r.Ref()+": "+err.Error())
			continue
		}
		got = append(got, e)
	}
	if err := library.Update(root, func(m *library.Manifest) error {
		for _, e := range got {
			saved = append(saved, m.Put(root, e).Path)
		}
		return nil
	}); err != nil {
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
					if e.Generated.CoverOf != "" {
						src += ", a cover of " + e.Generated.CoverOf
					}
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

// earshotCommit pins earshot 0.3.0 (https://github.com/Leonxlnx/earshot). Code under the
// MIT licence; its models download once from their authors under their own licences (CLAP
// Apache-2.0, the AudioSet tagger BSD-3-Clause) into the cav cache, and every later run
// works offline. Nothing is uploaded: the audio never leaves the machine.
const earshotCommit = "925ebe56a1b755e5d354ab642bb8f344440cfa9c"

// earshotSpec installs the pinned earshot with its dependencies (torch included) through
// uv, for the CLAP model that cav sfx search --by-sound runs.
const earshotSpec = "earshot @ git+https://github.com/Leonxlnx/earshot@" + earshotCommit

// maxBySoundFiles bounds how many library files one by-sound search reads. A new file
// costs about a second of CPU; its embedding is then cached, so later searches pay only
// for new or changed files.
const maxBySoundFiles = 1000

type soundRank struct {
	Path       string  `json:"path"`
	Z          float64 `json:"z"`
	Cosine     float64 `json:"cosine"`
	Verdict    string  `json:"verdict"`
	SoundsLike string  `json:"soundsMostLike"`
	NearSilent bool    `json:"nearSilent"`
}

// searchLocalBySound ranks the local library files by how well they match the query's
// sound (earshot's CLAP model, locally through uv), best first. The note is empty unless
// the search was capped or the ranking did not run and the file-name search answered
// instead.
func searchLocalBySound(ctx context.Context, q sources.Query) ([]sources.Result, string) {
	libs, err := loadLibraries()
	if err != nil || len(libs) == 0 {
		return nil, ""
	}
	type soundFile struct {
		path  string
		terms library.Terms
	}
	var files []soundFile
	total := 0
	for _, l := range libs {
		terms := termsFor(l.Licence)
		if !terms.Allowed(q.Licence) {
			continue
		}
		walkAudio(l.Path, func(p string) bool {
			total++
			if len(files) < maxBySoundFiles {
				files = append(files, soundFile{p, terms})
			}
			return true
		})
	}
	if len(files) == 0 {
		return nil, ""
	}
	note := ""
	if total > len(files) {
		note = fmt.Sprintf("by sound: ranked the first %d of %d local files (the cap)", len(files), total)
	}
	paths := make([]string, len(files))
	termsByPath := make(map[string]library.Terms, len(files))
	for i, f := range files {
		paths[i] = f.path
		termsByPath[f.path] = f.terms
	}
	ranked, err := rankBySound(ctx, q.Text, paths)
	if err != nil {
		fallback, ferr := searchLocal(q)
		if ferr != nil {
			return nil, "by sound: " + err.Error() + "; the file-name search failed too: " + ferr.Error()
		}
		return fallback, "by sound unavailable (" + err.Error() + "); matched file names instead"
	}
	var out []sources.Result
	for _, r := range ranked {
		terms, ok := termsByPath[r.Path]
		if !ok {
			continue
		}
		out = append(out, sources.Result{Service: "local", ID: r.Path, Kind: "audio", Title: filepath.Base(r.Path),
			Page: r.Path, Original: r.Path, Terms: terms, Licence: terms.ID,
			Sound: &sources.SoundMatch{Z: r.Z, Cosine: r.Cosine, Verdict: r.Verdict, SoundsLike: r.SoundsLike}})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, note
}

// rankBySound runs the embedded helper through uv: earshot embeds the query and every
// file with CLAP and returns them sorted by the calibrated match, best first.
func rankBySound(ctx context.Context, query string, paths []string) ([]soundRank, error) {
	script, err := assets.Python.ReadFile("python/sfx_rank.py")
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr, "ranking local sounds by what they sound like (the first run downloads about 1.4 GB of models)…")
	args := append([]string{filepath.Join(config.CacheDir(), "earshot"), query}, paths...)
	out, err := pyrun.Run(ctx, filepath.Join(config.CacheDir(), "python"), "sfx_rank.py", script, []string{earshotSpec}, args, nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Results []soundRank `json:"results"`
	}
	if err := jsonUnmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("unexpected ranker output: %w", err)
	}
	return r.Results, nil
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
