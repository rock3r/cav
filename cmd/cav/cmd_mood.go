package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/moodboard"
)

// moodFile is where moodboard.json goes by default: beside the mood board folder, next to
// brief.json and storyboard.json.
func moodFile(dir string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(dir)), "moodboard.json")
}

// moodSync brings moodboard.json in line with the folder and the manifest, and makes the
// contact sheet of every clip that has none. It saves the file. A clip it cannot read stays
// on the board without a sheet, and the reason comes back as a note.
func moodSync(ctx context.Context, dir, file string) (*moodboard.Board, moodboard.Changes, []string, error) {
	root := library.Root()
	m, _ := library.Load(root)
	b, ch, err := moodboard.Sync(file, dir, m, root)
	if err != nil {
		return nil, ch, nil, err
	}
	var notes []string
	for i := range b.Items {
		it := &b.Items[i]
		if !it.Active() || it.EffectiveKind() != moodboard.KindClip || it.File == "" {
			continue
		}
		it.Thumb = b.ClipThumb(it.ID)
		out, err := b.SheetPath(*it)
		if err == nil && !b.InFolder(it.File) {
			err = fmt.Errorf("%s is outside the mood board folder", it.File)
		}
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: no contact sheet (%v)", it.ID, err))
			continue
		}
		// The sheet is current when it exists and was made from these very bytes. This is
		// checked first, so an unchanged clip never calls ffprobe or ffmpeg.
		digest, derr := moodboard.FileDigest(b.Path(it.File))
		_, statErr := os.Stat(out)
		if derr == nil && it.SheetOf == digest && statErr == nil {
			continue
		}
		secs, err := moodboard.ContactSheet(ctx, b.Path(it.File), out, 6)
		if err != nil {
			// A clip that did not change (or whose bytes cannot be read now) keeps its
			// sheet: the failure may be passing. A clip that changed makes the old sheet and
			// length wrong, so they go. SheetPath has checked that out is a plain file in .thumbs.
			if derr != nil || (it.SheetOf != "" && it.SheetOf == digest) {
				notes = append(notes, fmt.Sprintf("%s: could not refresh the contact sheet, kept the old one (%v)", it.ID, err))
				continue
			}
			it.Seconds, it.SheetOf = 0, ""
			os.Remove(out)
			notes = append(notes, fmt.Sprintf("%s: no contact sheet (%v)", it.ID, err))
			continue
		}
		it.Seconds = float64(int(secs*10+0.5)) / 10
		it.SheetOf = digest
	}
	if err := b.Save(); err != nil {
		return nil, ch, notes, err
	}
	return b, ch, notes, nil
}

func moodFlags(fs *flag.FlagSet) (dir, file *string) {
	dir = fs.String("dir", "moodboard", "the folder of references")
	file = fs.String("file", "", "moodboard.json (default: beside the folder)")
	return
}

func boardMood(a *app, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "take":
			return moodTake(a, args[1:])
		case "drop":
			return moodDrop(a, args[1:])
		}
	}
	fs := flag.NewFlagSet("board mood", flag.ContinueOnError)
	out := fs.String("o", filepath.Join(outDir(), "moodboard.png"), "output PNG")
	cols := fs.Int("cols", 0, "columns (default: 3 or 4)")
	file := fs.String("file", "", "moodboard.json (default: beside the folder)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	dir := boardPath(pos, "moodboard")
	if *file == "" {
		*file = moodFile(dir)
	}
	ctx := context.Background()
	b, ch, notes, err := moodSync(ctx, dir, *file)
	if err != nil {
		return fail(exitError, err.Error(), "put reference images in "+dir+", for example with cav ref get")
	}
	for _, f := range ch.Skipped {
		notes = append(notes, f+" is not shown: convert it to PNG or JPEG")
	}
	root := library.Root()
	var tiles []board.MoodTile
	phDir := ""
	defer func() {
		if phDir != "" {
			os.RemoveAll(phDir)
		}
	}()
	counts := map[string]int{}
	var noTake []string
	for _, it := range b.Active() {
		kind := it.EffectiveKind()
		counts[kind]++
		if strings.TrimSpace(it.Take) == "" {
			noTake = append(noTake, it.ID)
		}
		p := ""
		switch {
		case kind == moodboard.KindImage && it.File != "":
			p = b.Path(it.File)
		case it.Thumb != "":
			p = b.Path(it.Thumb)
		}
		if st, err := os.Stat(p); p == "" || err != nil || st.IsDir() {
			// No picture (a video added offline, or a clip ffmpeg could not read): a plain
			// card keeps the item on the board, and its caption says what it is.
			if p, err = placeholderCard(&phDir); err != nil {
				return err
			}
			notes = append(notes, it.ID+": no picture, drawn as a plain card")
		}
		cap := it.ID
		if it.File != "" {
			cap = filepath.Base(it.File)
		}
		if it.Source != "" || it.Licence != "" {
			cap = strings.TrimSpace(it.Source + "  " + library.LicenceName(it.Licence))
			if e := manifestEntry(root, b.Path(it.File)); e != nil && e.Author != "" {
				cap += "  " + e.Author
			}
		}
		switch kind {
		case moodboard.KindClip:
			cap = "clip  " + cap
		case moodboard.KindEmbed:
			cap = "web video  " + firstNonEmpty(it.Title, it.ID)
		}
		tiles = append(tiles, board.MoodTile{Path: p, Caption: cap})
	}
	if len(tiles) == 0 {
		return fail(exitError, "nothing to lay out in "+dir, "add some with cav ref search / cav ref get / cav ref add <youtube or vimeo link>")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	w, h, err := board.Mood(ctx, tiles, *out, *cols)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	total := len(b.Active())
	a.emit(map[string]any{"file": *out, "json": *file, "images": len(tiles), "items": total, "counts": counts,
		"added": nonNil(ch.Added), "missing": nonNil(ch.Missing), "revived": nonNil(ch.Revived), "noTake": nonNil(noTake),
		"notes": nonNil(notes), "width": w, "height": h}, func() {
		fmt.Printf("%s (%s, %dx%d)\n", *out, moodCounts(total, counts), w, h)
		fmt.Printf("%s\n", *file)
		if len(ch.Added) > 0 {
			fmt.Println("added: " + strings.Join(ch.Added, ", "))
		}
		if len(ch.Revived) > 0 {
			fmt.Println("back again: " + strings.Join(ch.Revived, ", "))
		}
		if len(ch.Missing) > 0 {
			fmt.Println("file gone (kept, with its take): " + strings.Join(ch.Missing, ", "))
		}
		for _, n := range notes {
			fmt.Fprintln(os.Stderr, "note: "+n)
		}
		if len(noTake) > 0 {
			fmt.Println("no take line yet: " + strings.Join(noTake, ", "))
			fmt.Println(`write what to take from each: cav board mood take <id> "..."`)
		}
	})
	return nil
}

func manifestEntry(root, abs string) *library.Entry {
	m, err := library.Load(root)
	if err != nil || m == nil {
		return nil
	}
	rel := library.Rel(root, abs)
	for i := range m.Assets {
		if m.Assets[i].Path == rel {
			return &m.Assets[i]
		}
	}
	return nil
}

// moodItem syncs the board and finds the item an id names. An unknown id lists the ids.
func moodItem(ctx context.Context, dir, file, id string) (*moodboard.Board, *moodboard.Item, error) {
	b, _, _, err := moodSync(ctx, dir, file)
	if err != nil {
		return nil, nil, fail(exitError, err.Error(), "")
	}
	it := b.Find(id)
	if it == nil {
		return nil, nil, usageErr("there is no item %q in %s; the items are: %s", id, file, strings.Join(b.IDs(), ", "))
	}
	return b, it, nil
}

func moodTake(a *app, args []string) error {
	fs := flag.NewFlagSet("board mood take", flag.ContinueOnError)
	dir, file := moodFlags(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return usageErr(`usage: cav board mood take <id> "what to take from it"`)
	}
	if *file == "" {
		*file = moodFile(*dir)
	}
	b, it, err := moodItem(context.Background(), *dir, *file, pos[0])
	if err != nil {
		return err
	}
	it.Take = strings.TrimSpace(strings.Join(pos[1:], " "))
	if err := b.Save(); err != nil {
		return err
	}
	a.emit(map[string]any{"id": it.ID, "take": it.Take, "json": *file}, func() {
		fmt.Printf("%s: %s\n", it.ID, it.Take)
	})
	return nil
}

func moodDrop(a *app, args []string) error {
	fs := flag.NewFlagSet("board mood drop", flag.ContinueOnError)
	dir, file := moodFlags(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav board mood drop <id>")
	}
	if *file == "" {
		*file = moodFile(*dir)
	}
	b, it, err := moodItem(context.Background(), *dir, *file, pos[0])
	if err != nil {
		return err
	}
	it.Status = moodboard.StatusDropped
	if err := b.Save(); err != nil {
		return err
	}
	a.emit(map[string]any{"id": it.ID, "status": it.Status, "json": *file}, func() {
		fmt.Printf("%s dropped; its file stays in %s and cav will not bring it back\n", it.ID, *dir)
	})
	return nil
}

// moodCounts says "4 items: 2 images, 1 clip, 1 web video", with the right plural.
func moodCounts(total int, counts map[string]int) string {
	noun := func(n int, one, many string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s", one)
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	return noun(total, "item", "items") + ": " + noun(counts[moodboard.KindImage], "image", "images") + ", " +
		noun(counts[moodboard.KindClip], "clip", "clips") + ", " + noun(counts[moodboard.KindEmbed], "web video", "web videos")
}

// placeholderCard writes a plain dark card once into a temporary folder and returns its path.
func placeholderCard(dir *string) (string, error) {
	if *dir == "" {
		d, err := os.MkdirTemp("", "cav-mood-card")
		if err != nil {
			return "", err
		}
		*dir = d
	}
	p := filepath.Join(*dir, "card.png")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	img := image.NewRGBA(image.Rect(0, 0, 480, 270))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{44, 47, 56, 255}}, image.Point{}, draw.Src)
	f, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return p, png.Encode(f, img)
}
