package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/imagegen"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/services"
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func init() {
	register(command{
		name:    "gen",
		args:    "image \"<prompt>\" [-o file] [--ref img]... [--aspect 16:9] [--alpha] [--vector] [--service S] [--model M] | vector <image> [-o file.svg] [--service S]",
		summary: "Generate a still or an asset (transparent PNG, SVG) with the configured image service.",
		run:     cmdGen,
	})
	longHelp["gen"] = `
cav gen image "a paper-cut mountain range at dusk" -o assets/mountains.png
    uses the first usable service for the job (cav config): image, or image.alpha with
    --alpha (a transparent background, for layering in Cavalry), or image.vector with
    --vector (an SVG that Cavalry imports as shapes).
    --ref (repeatable) sends reference images for a consistent look.
cav gen vector logo.png -o logo.svg
    traces a PNG into an SVG: recraft with a key, otherwise vtracer or potrace if installed.
Every result is recorded in .cav/manifest.json with the provider, model and prompt.
Without an image key, cav gen image draws a greybox card with the prompt instead.`
}

func cmdGen(a *app, args []string) error {
	if len(args) == 0 {
		printHelp("gen")
		return nil
	}
	sub, args := args[0], args[1:]
	switch sub {
	case "image":
		return genImage(a, args)
	case "vector":
		return genVector(a, args)
	}
	return usageErr("unknown gen command %q (image, vector)", sub)
}

func genImage(a *app, args []string) error {
	fs := flag.NewFlagSet("gen image", flag.ContinueOnError)
	out := fs.String("o", "", "output file (default: assets/<words>.<ext>)")
	var refs stringList
	fs.Var(&refs, "ref", "reference image (repeatable)")
	aspect := fs.String("aspect", "16:9", "aspect ratio")
	alpha := fs.Bool("alpha", false, "transparent background")
	vector := fs.Bool("vector", false, "SVG")
	service := fs.String("service", "", "service to use")
	model := fs.String("model", "", "model to use")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageErr("give a prompt")
	}
	prompt := strings.Join(pos, " ")
	job := "image"
	switch {
	case *vector:
		job = "image.vector"
	case *alpha:
		job = "image.alpha"
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	// Local models download several GB on their first run, so the bound is generous;
	// each API call has its own shorter timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	ch, err := services.Pick(ctx, c, job, *service)
	if err != nil {
		return fail(exitError, err.Error(), "see cav config")
	}
	var im *imagegen.Image
	switch ch.Service {
	case "greybox":
		data, err := imagegen.Greybox(imagegen.Card{Title: "placeholder", Lines: []string{prompt}, Footer: "greybox: add an image key (cav config) to generate"})
		if err != nil {
			return err
		}
		im = &imagegen.Image{Data: data, MIME: "image/png", Provider: "greybox"}
	case "vtracer", "potrace":
		return fail(exitError, ch.Service+" traces images but cannot make one from words", "make a PNG first, then: cav gen vector <png>")
	default:
		im, err = imagegen.Generate(ctx, c, ch, imagegen.Request{Prompt: prompt, Refs: refs, Aspect: *aspect, Alpha: *alpha, Vector: *vector, Model: *model})
		if err != nil {
			return fail(exitError, err.Error(), "")
		}
	}
	path := *out
	if path == "" {
		path = filepath.Join("assets", safeSlug(prompt)+im.Ext())
	}
	return saveGenerated(a, path, im, prompt, refs, ch.Skipped)
}

func genVector(a *app, args []string) error {
	fs := flag.NewFlagSet("gen vector", flag.ContinueOnError)
	out := fs.String("o", "", "output SVG (default: next to the input)")
	service := fs.String("service", "", "recraft, vtracer or potrace")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav gen vector <image> [-o file.svg]")
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ch, err := services.Pick(ctx, c, "image.vector", *service)
	if err != nil {
		return fail(exitError, err.Error(), "install vtracer (cargo install vtracer) or potrace (brew install potrace), or add a Recraft key")
	}
	if ch.Service != "recraft" && ch.Service != "vtracer" && ch.Service != "potrace" {
		return fail(exitError, ch.Service+" does not trace images", "")
	}
	im, err := imagegen.Vectorize(ctx, ch, pos[0])
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	path := *out
	if path == "" {
		path = strings.TrimSuffix(pos[0], filepath.Ext(pos[0])) + ".svg"
	}
	return saveGenerated(a, path, im, "vectorized "+pos[0], []string{pos[0]}, ch.Skipped)
}

func saveGenerated(a *app, path string, im *imagegen.Image, prompt string, refs []string, skipped []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, im.Data, 0o644); err != nil {
		return err
	}
	root := library.Root()
	m, err := library.Load(root)
	if err != nil {
		return err
	}
	sum, _ := fileSHA(path)
	kind := "image"
	if strings.Contains(im.MIME, "svg") {
		kind = "svg"
	}
	e := m.Put(root, library.Entry{Path: path, Kind: kind, Source: im.Provider, Licence: "generated", CommercialOK: true,
		RetrievedAt: time.Now().UTC(), SHA256: sum, Generated: &library.Generated{Provider: im.Provider, Model: im.Model, Prompt: prompt, Refs: refs}})
	if err := m.Save(root); err != nil {
		return err
	}
	a.emit(map[string]any{"file": path, "provider": im.Provider, "model": im.Model, "asset": e, "skipped": skipped}, func() {
		for _, s := range skipped {
			fmt.Fprintln(os.Stderr, "note: skipped "+s)
		}
		model := ""
		if im.Model != "" {
			model = " (" + im.Model + ")"
		}
		fmt.Printf("%s: made with %s%s\n", path, im.Provider, model)
	})
	return nil
}

func safeSlug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "image"
	}
	return out
}
