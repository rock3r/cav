package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
		args:    "image \"<prompt>\" [-o file] [--ref img]... [--aspect 16:9] [--alpha] [--vector] [--service S] [--model M] | vector <image> [-o file.svg] [--service S] | cutout <image> [-o file.png] [--service S]",
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
    --service ideogram keeps gradients (a logo or a mascot). It costs about $0.10, so
    it stays behind recraft and the free tracers.
cav gen cutout photo.jpg -o fox.png
    makes the background of an image transparent: recraft with a key, otherwise rembg
    (BiRefNet, local through uv; the first run downloads the model, about 1 GB).
    With rembg in the image.alpha order, cav gen image --alpha makes the image with the
    image service, then cuts it out with rembg.
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
	case "cutout":
		return genCutout(a, args)
	}
	return usageErr("unknown gen command %q (image, vector, cutout)", sub)
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
	// Check the output before Pick reads a key (which can ask 1Password) and before a
	// paid call. Without -o the file type is known only after the call, so cav checks
	// the folder.
	if *out != "" {
		err = checkOutput(*out, "image.png")
	} else {
		err = checkFolder("assets")
	}
	if err != nil {
		return err
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	// Local models download several GB on their first run, so the bound is generous;
	// each API call has its own shorter timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	pickConfig := c
	if *model != "" {
		// --model goes to whichever service wins, so mflux must be ready with that model.
		pickConfig = c.WithModel("mflux", *model)
	}
	ch, err := services.Pick(ctx, pickConfig, job, *service)
	if err != nil {
		return fail(exitError, err.Error(), "see cav config")
	}
	var im *imagegen.Image
	switch ch.Service {
	case "sketch":
		if *service == "sketch" {
			return fail(exitError, "sketch only renders storyboard SVGs, not a prompt",
				"write board/<id>.svg for each shot and run cav board frames, or pick another image service")
		}
		fallthrough
	case "greybox":
		data, err := imagegen.Greybox(imagegen.Card{Title: "placeholder", Lines: []string{prompt}, Footer: "greybox: add an image key (cav config) to generate"})
		if err != nil {
			return err
		}
		im = &imagegen.Image{Data: data, MIME: "image/png", Provider: "greybox"}
	case "vtracer", "potrace", "ideogram":
		return fail(exitError, ch.Service+" traces images but cannot make one from words", "make a PNG first, then: cav gen vector <png>")
	case "rembg":
		// rembg only cuts out: the image service makes the image, then rembg removes its
		// background.
		// If the cutout fails, keep the paid image next to the output and record it in
		// the manifest, so cav gen cutout on it keeps its generated provenance.
		keep := func(gen *imagegen.Image, skipped []string) (string, error) {
			p := *out
			if p == "" {
				p = filepath.Join("assets", safeSlug(prompt)+".png")
			}
			if err := os.MkdirAll(rawDir(p), 0o755); err != nil {
				return "", err
			}
			p, err := writeFree(strings.TrimSuffix(p, filepath.Ext(p))+"-uncut", gen)
			if err != nil {
				return "", err
			}
			// Record it without printing a result: the command still fails.
			_, err = recordAsset(p, gen, prompt, refs, "")
			return p, err
		}
		im, err = generateThenCutout(ctx, pickConfig, ch, imagegen.Request{Prompt: prompt, Refs: refs, Aspect: *aspect, Model: *model}, keep)
		if err != nil {
			return err
		}
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
	return saveOrKeep(a, path, im, prompt, refs, ch.Skipped, "")
}

func genVector(a *app, args []string) error {
	fs := flag.NewFlagSet("gen vector", flag.ContinueOnError)
	out := fs.String("o", "", "output SVG (default: next to the input)")
	service := fs.String("service", "", "recraft, vtracer, potrace or ideogram")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav gen vector <image> [-o file.svg]")
	}
	// Check the input and the output before Pick reads a key (which can ask 1Password)
	// and before a paid call.
	if _, err := os.Stat(pos[0]); err != nil {
		return fail(exitError, err.Error(), "")
	}
	path := *out
	if path == "" {
		path = strings.TrimSuffix(pos[0], filepath.Ext(pos[0])) + ".svg"
	}
	if err := checkOutput(path, "image.svg"); err != nil {
		return err
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ch, err := services.Pick(ctx, c, "image.vector", *service)
	if err != nil {
		return fail(exitError, err.Error(), "install vtracer (cargo install vtracer) or potrace (brew install potrace), or add a Recraft or Ideogram key")
	}
	if ch.Service != "recraft" && ch.Service != "vtracer" && ch.Service != "potrace" && ch.Service != "ideogram" {
		return fail(exitError, ch.Service+" does not trace images", "")
	}
	im, err := imagegen.Vectorize(ctx, ch, pos[0])
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	return saveOrKeep(a, path, im, "vectorized "+pos[0], []string{pos[0]}, ch.Skipped, pos[0])
}

// generateThenCutout makes an image with the first usable image service, then runs the
// cutout service ch on it. When the cutout fails, keep saves the generated image and
// returns its path, so a retry does not pay for the image again.
func generateThenCutout(ctx context.Context, c *services.Config, ch *services.Choice, r imagegen.Request,
	keep func(*imagegen.Image, []string) (string, error)) (*imagegen.Image, error) {
	gen, err := services.Pick(ctx, c, "image", "")
	if err != nil {
		return nil, fail(exitError, err.Error(), "see cav config")
	}
	if gen.Service == "greybox" || gen.Service == "sketch" {
		return nil, fail(exitError, ch.Service+" cuts the background out of a generated image, but no image service can make one (only greybox is usable)",
			"add an image key (cav config), or cut out an image you have: cav gen cutout <image>")
	}
	ch.Skipped = append(ch.Skipped, gen.Skipped...)
	// Make the folder for the generated image before the image service runs, so a
	// failure here cannot waste a paid call.
	tmp, err := os.MkdirTemp("", "cav-gen")
	if err != nil {
		return nil, err
	}
	im, err := imagegen.Generate(ctx, c, gen, r)
	if err != nil {
		os.RemoveAll(tmp)
		return nil, fail(exitError, err.Error(), "")
	}
	in := filepath.Join(tmp, "image"+im.Ext())
	if err := os.WriteFile(in, im.Data, 0o644); err != nil {
		os.RemoveAll(tmp)
		if kept, kerr := keep(im, gen.Skipped); kerr == nil {
			return nil, fail(exitError, err.Error()+" (the generated image is kept at "+kept+")", "cut it out: cav gen cutout "+shellArg(kept)+" -o <file.png>")
		}
		return nil, err
	}
	cut, err := imagegen.RemoveBackground(ctx, ch, in)
	os.RemoveAll(tmp)
	if err != nil {
		kept, kerr := keep(im, gen.Skipped)
		if kerr != nil {
			return nil, fail(exitError, err.Error()+" (saving the generated image also failed: "+kerr.Error()+")", "")
		}
		return nil, fail(exitError, err.Error()+" (the generated image is kept at "+kept+")", "cut it out again: cav gen cutout "+shellArg(kept)+" -o <file.png>")
	}
	cut.Provider = im.Provider + " + " + cut.Provider
	cut.Model = im.Model + " + " + cut.Model
	return cut, nil
}

func genCutout(a *app, args []string) error {
	fs := flag.NewFlagSet("gen cutout", flag.ContinueOnError)
	out := fs.String("o", "", "output PNG (default: <input>-cutout.png next to the input)")
	service := fs.String("service", "", "recraft or rembg")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav gen cutout <image> [-o file.png]")
	}
	// Check the input before Pick reads a key, which can ask 1Password.
	if _, err := os.Stat(pos[0]); err != nil {
		return fail(exitError, err.Error(), "")
	}
	path := *out
	if path == "" {
		path = strings.TrimSuffix(pos[0], filepath.Ext(pos[0])) + "-cutout.png"
	}
	// Check the output before Pick reads a key (which can ask 1Password) and before the
	// cutout runs, so a bad -o cannot waste a paid call.
	if err := checkOutput(path, "cutout.png"); err != nil {
		return err
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	// Only the services that cut out an existing image, in the image.alpha order.
	cc := *c
	cc.Order = map[string][]string{}
	for _, s := range c.OrderFor("image.alpha") {
		if slices.Contains(imagegen.CutoutServices, s) {
			cc.Order["image.alpha"] = append(cc.Order["image.alpha"], s)
		}
	}
	if len(cc.Order["image.alpha"]) == 0 && *service == "" {
		return fail(exitError, "the image.alpha order has no service that cuts out an image (recraft, rembg)",
			"name one with --service rembg, or add it: cav config order image.alpha "+strings.Join(slices.Concat(c.OrderFor("image.alpha"), []string{"rembg"}), ","))
	}
	// Refuse a known service that cannot cut out before Pick reads its key, so a key in
	// 1Password is never asked for.
	if d, ok := services.Catalog[*service]; ok && slices.Contains(d.Jobs, "image.alpha") && !slices.Contains(imagegen.CutoutServices, *service) {
		return fail(exitError, *service+" makes transparent images only while it generates them", "use --service rembg or --service recraft")
	}
	// The first run downloads the rembg model (about 1 GB).
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	ch, err := services.Pick(ctx, &cc, "image.alpha", *service)
	if err != nil {
		return fail(exitError, err.Error(), "install uv (https://docs.astral.sh/uv/) for rembg, or add a Recraft key")
	}
	if !slices.Contains(imagegen.CutoutServices, ch.Service) {
		return fail(exitError, ch.Service+" makes transparent images only while it generates them", "use --service rembg or --service recraft")
	}
	im, err := imagegen.RemoveBackground(ctx, ch, pos[0])
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	return saveOrKeep(a, path, im, "cut out "+pos[0], []string{pos[0]}, ch.Skipped, pos[0])
}

// checkOutput proves that cav can write path, before a paid call. It refuses a folder,
// makes the parent folder, and checks that an existing file opens for writing (without
// changing it) or that a new file can be made in the folder. example is the file name in
// the hint for a folder.
func checkOutput(path, example string) error {
	missing, err := checkFile(path, example)
	if err != nil || !missing {
		return err
	}
	if err := checkFolder(rawDir(path)); err != nil {
		return err
	}
	// Making the folders can change what path names: missing/../old.png names old.png
	// only once missing exists. So check the file again.
	_, err = checkFile(path, example)
	return err
}

// checkFile refuses a folder and checks that an existing file opens for writing, without
// changing it. It says whether the file is missing.
func checkFile(path, example string) (missing bool, err error) {
	if fi, err := os.Stat(path); strings.HasSuffix(path, "/") || strings.HasSuffix(path, string(filepath.Separator)) || err == nil && fi.IsDir() {
		return false, fail(exitError, path+" is a folder", "give a file name: -o "+filepath.Join(path, example))
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		return false, f.Close()
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fail(exitError, "cannot write "+path+": "+err.Error(), "make it writable, or give another file with -o")
	}
	if _, lerr := os.Lstat(path); lerr == nil {
		// A link to a file that does not exist: cav would write through it, somewhere
		// this check cannot see.
		return false, fail(exitError, path+" is a link to a file that does not exist", "remove the link, or give another file with -o")
	}
	return true, nil
}

// rawDir returns the folder part of path as written, without cleaning it like
// filepath.Dir does. The system then follows the same folders, links and ".." for the
// check as for the save: missing/../fox.png makes the folder missing, and
// link/../fox.png stays next to the link's target.
// A Windows volume keeps its root: C:\x.png gives C:\, and C:x.png gives C:.
func rawDir(path string) string {
	vol := filepath.VolumeName(path)
	rest := path[len(vol):]
	i := strings.LastIndexAny(rest, "/"+string(filepath.Separator))
	switch {
	case i < 0 && vol == "":
		return "."
	case i < 0:
		return vol
	case i == 0:
		return vol + rest[:1]
	}
	return vol + rest[:i]
}

// checkFolder makes dir and proves that cav can make a file in it, by making and
// removing an empty temporary file.
func checkFolder(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(exitError, err.Error(), "")
	}
	f, err := os.CreateTemp(dir, ".cav-write-check-*")
	if err != nil {
		return fail(exitError, "cannot write in "+dir+": "+err.Error(), "make the folder writable, or give a file in another folder with -o")
	}
	f.Close()
	return os.Remove(f.Name())
}

// saveOrKeep saves the image like saveAsset. When cav cannot write path (after a paid
// call), it keeps the image under a free name in the working folder, records it there,
// and says where it is, so the image is not lost.
func saveOrKeep(a *app, path string, im *imagegen.Image, prompt string, refs []string, skipped []string, from string) error {
	err := saveAsset(a, path, im, prompt, refs, skipped, from)
	var we *writeError
	if !errors.As(err, &we) {
		return err
	}
	kept, kerr := writeFree(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), im)
	if kerr == nil {
		// Record it without printing a result: the command still fails, and --json must
		// print one result only.
		_, kerr = recordAsset(kept, im, prompt, refs, from)
		if kerr != nil {
			// The image is written; only the manifest failed.
			return fail(exitError, err.Error()+" (the image is kept at "+kept+", but recording it in the manifest failed: "+kerr.Error()+")", "")
		}
	}
	if kerr != nil {
		return fail(exitError, err.Error()+" (keeping the image in the working folder also failed: "+kerr.Error()+")", "")
	}
	return fail(exitError, err.Error()+" (the image is kept at "+kept+")", "the manifest records it there; make "+path+" writable for the next run")
}

// writeError is a failure to write the image file itself, not the manifest.
type writeError struct{ err error }

func (e *writeError) Error() string { return e.err.Error() }
func (e *writeError) Unwrap() error { return e.err }

// writeFree writes the image to base+ext, or base-2+ext, base-3+ext and so on, whichever
// file does not exist yet, and returns that path. It makes the file with an exclusive
// create, so a kept image never replaces another file, even one that another cav run
// makes at the same moment. If the write fails, it removes the file it made.
func writeFree(base string, im *imagegen.Image) (string, error) {
	return writeFreeData(base, im.Ext(), im.Data)
}

// writeFreeData writes data to base+ext, or to base-2+ext, base-3+ext and so on: the
// first name it can create, so it never replaces another file.
func writeFreeData(base, ext string, data []byte) (string, error) {
	p := base + ext
	for n := 2; ; n++ {
		beforeReserve(p)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			p = fmt.Sprintf("%s-%d%s", base, n, ext)
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(p)
			return "", err
		}
		return p, nil
	}
}

// beforeReserve runs just before writeFree tries a name. A test uses it to take the
// name first, as another cav run could.
var beforeReserve = func(string) {}

// derivedTerms gives a cutout or a trace the licence of the image it was made from,
// because it is a changed version of that image. An image that the manifest does not list
// gets an unknown licence, so cav credits check asks about it.
func derivedTerms(m *library.Manifest, root, from string, e library.Entry) library.Entry {
	rel := library.Rel(root, from)
	for _, src := range m.Assets {
		if src.Path != rel {
			continue
		}
		if src.Generated != nil {
			e.Licence, e.LicenceNote, e.CommercialOK = src.Licence, src.LicenceNote, src.CommercialOK
			return e
		}
		// A licensed image: the cutout or trace carries its terms and credit. It is not
		// recorded as generated, because cav credits check skips generated assets.
		e.Generated = nil
		e.Title, e.Source, e.SourceID, e.URL = src.Title, src.Source, src.SourceID, src.URL
		e.Licence, e.LicenceURL, e.LicenceNote = src.Licence, src.LicenceURL, src.LicenceNote
		e.Author, e.AuthorURL, e.Attribution, e.AttributionRequired = src.Author, src.AuthorURL, src.Attribution, src.AttributionRequired
		e.CommercialOK, e.ShareAlike, e.NoDerivatives = src.CommercialOK, src.ShareAlike, src.NoDerivatives
		if src.Licence == "reference-only" {
			// credits check skips mood-board references, but a cutout or trace is meant for
			// the piece.
			e.Licence, e.CommercialOK = "unknown", false
		}
		return e
	}
	e.Licence, e.CommercialOK = "unknown", false
	return e
}

// saveAsset writes the image and records it in the manifest. from is the image that a
// cutout or a trace was made from, or "" for an image made from words.
func saveAsset(a *app, path string, im *imagegen.Image, prompt string, refs []string, skipped []string, from string) error {
	e, err := storeAsset(path, im, prompt, refs, from)
	if err != nil {
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

// storeAsset writes the image and records it in the manifest, without printing anything.
func storeAsset(path string, im *imagegen.Image, prompt string, refs []string, from string) (library.Entry, error) {
	// A .png name gets PNG bytes, even when the service answered JPEG (Gemini does).
	if strings.EqualFold(filepath.Ext(path), ".png") && !strings.Contains(im.MIME, "png") && !strings.Contains(im.MIME, "svg") {
		conv, err := imagegen.ToPNG(im)
		if err != nil {
			return library.Entry{}, fmt.Errorf("converting the %s image to PNG: %w", im.MIME, err)
		}
		*im = *conv
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return library.Entry{}, &writeError{err}
	}
	if err := os.WriteFile(path, im.Data, 0o644); err != nil {
		return library.Entry{}, &writeError{err}
	}
	return recordAsset(path, im, prompt, refs, from)
}

// recordAsset records the image that is already written at path in the manifest.
func recordAsset(path string, im *imagegen.Image, prompt string, refs []string, from string) (library.Entry, error) {
	root := library.Root()
	sum, _ := fileSHA(path)
	kind := "image"
	if strings.Contains(im.MIME, "svg") {
		kind = "svg"
	}
	e := library.Entry{Path: path, Kind: kind, Source: im.Provider, Licence: "generated", CommercialOK: true,
		RetrievedAt: time.Now().UTC(), SHA256: sum, Generated: &library.Generated{Provider: im.Provider, Model: im.Model, Prompt: prompt, Refs: refs}}
	err := library.Update(root, func(m *library.Manifest) error {
		if from != "" {
			e = derivedTerms(m, root, from, e)
		}
		e = m.Put(root, e)
		return nil
	})
	if err != nil {
		return library.Entry{}, err
	}
	return e, nil
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
