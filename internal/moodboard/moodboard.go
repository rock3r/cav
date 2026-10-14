// Package moodboard holds moodboard.json: the references of a mood board (images, local clips
// and video embeds) with a "take from it" line for each. The agent reads the lines and the
// contact sheet; the person sees the same items on the review page.
package moodboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/library"
)

const Schema = "cav-moodboard/1"

// What an item is.
const (
	KindImage = "image"
	KindClip  = "clip"
	KindEmbed = "embed"
)

// Who put an item on the board.
const (
	ByUser  = "user"
	ByAgent = "agent"
	ByBrief = "brief"
)

// Item states. An empty status is an active item.
const (
	// StatusMissing marks an item whose file is gone. The entry stays so its take survives
	// the file coming back.
	StatusMissing = "missing"
	// StatusDropped marks an item the person dropped. The file stays, and a sync never
	// brings the item back.
	StatusDropped = "dropped"
)

// ThumbDir is the folder inside the mood board folder for files cav makes: contact sheets of
// clips and thumbnails of embeds. board.MoodImages does not look inside it.
const ThumbDir = ".thumbs"

// ClipExts are the video files a mood board plays.
var ClipExts = map[string]bool{".mp4": true, ".mov": true, ".webm": true, ".m4v": true}

// Item is one reference. File and Thumb are relative to the folder of moodboard.json.
type Item struct {
	ID      string  `json:"id"`
	Kind    string  `json:"kind,omitempty"`
	File    string  `json:"file,omitempty"`
	Thumb   string  `json:"thumb,omitempty"`
	Seconds float64 `json:"seconds,omitempty"`
	URL     string  `json:"url,omitempty"`
	Title   string  `json:"title,omitempty"`
	Source  string  `json:"source,omitempty"`
	Licence string  `json:"licence,omitempty"`
	Credit  string  `json:"credit,omitempty"`
	Take    string  `json:"take,omitempty"`
	AddedBy string  `json:"addedBy,omitempty"`
	Status  string  `json:"status,omitempty"`
}

// Active says whether the item is on the board.
func (it Item) Active() bool { return it.Status == "" }

// KindOfFile says what kind of item a file is, or "" when it is neither an image nor a clip.
func KindOfFile(name string) string {
	switch {
	case board.IsImageFile(name):
		return KindImage
	case ClipExts[strings.ToLower(filepath.Ext(name))]:
		return KindClip
	}
	return ""
}

// EffectiveKind is the item's kind, worked out from its file or link when the file does not
// say. Hand-written items often leave it out.
func (it Item) EffectiveKind() string {
	switch it.Kind {
	case KindImage, KindClip, KindEmbed:
		return it.Kind
	}
	if it.File != "" {
		if k := KindOfFile(it.File); k != "" {
			return k
		}
		return KindImage
	}
	if _, _, _, ok := ParseVideoURL(it.URL); ok {
		return KindEmbed
	}
	return KindImage
}

type Board struct {
	Schema string `json:"schema"`
	// Dir is the folder of references, relative to moodboard.json.
	Dir   string `json:"dir,omitempty"`
	Items []Item `json:"items"`

	path string
}

// Load reads moodboard.json. It is lenient: the review page reads files an agent edits.
func Load(path string) (*Board, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mb := &Board{}
	if err := json.Unmarshal(b, mb); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	mb.path = path
	return mb, nil
}

// New starts an empty board for a file that does not exist yet.
func New(path, dir string) *Board { return &Board{Schema: Schema, Dir: dir, path: path} }

// Save writes the file in one step, so a reader never sees half of it.
func (b *Board) Save() error {
	if b.path == "" {
		return errors.New("the mood board has no file")
	}
	b.Schema = Schema
	if b.Items == nil {
		b.Items = []Item{}
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(b.path), ".moodboard-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(data, '\n'))
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), b.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// FilePath is where the board is saved.
func (b *Board) FilePath() string { return b.path }

// Path resolves a path written in the file.
func (b *Board) Path(rel string) string {
	if rel == "" || filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(filepath.Dir(b.path), filepath.FromSlash(rel))
}

// Find returns the item with this id.
func (b *Board) Find(id string) *Item {
	for i := range b.Items {
		if b.Items[i].ID == id {
			return &b.Items[i]
		}
	}
	return nil
}

// Active lists the items that are on the board.
func (b *Board) Active() []Item {
	var out []Item
	for _, it := range b.Items {
		if it.Active() {
			out = append(out, it)
		}
	}
	return out
}

// IDs lists every id, for a message.
func (b *Board) IDs() []string {
	var out []string
	for _, it := range b.Items {
		out = append(out, it.ID)
	}
	return out
}

// thumbDir is the folder of generated files, as an absolute path.
func (b *Board) thumbDir() string {
	return filepath.Join(filepath.Dir(b.path), filepath.FromSlash(b.Dir), ThumbDir)
}

func (b *Board) inThumbs(rel string) bool {
	p := filepath.Clean(b.Path(filepath.FromSlash(rel)))
	return strings.HasPrefix(p, b.thumbDir()+string(filepath.Separator))
}

// ClipThumb is where the contact sheet of a clip goes. cav decides it from the id, whatever
// the file says, so a hand-edited path cannot make cav write elsewhere.
func (b *Board) ClipThumb(id string) string {
	return filepath.ToSlash(filepath.Join(b.Dir, ThumbDir, Slug(id)+".png"))
}

// SheetPath returns the place to write the contact sheet of a clip, or an error when it is
// not safe: the path must stay inside the thumbnail folder, and neither it nor that folder
// may be a link.
func (b *Board) SheetPath(it Item) (string, error) {
	out := filepath.Clean(b.Path(filepath.FromSlash(b.ClipThumb(it.ID))))
	dir := b.thumbDir()
	if !strings.HasPrefix(out, dir+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: the contact sheet would be outside %s", it.ID, dir)
	}
	for _, p := range []string{dir, out} {
		if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%s is a link, so cav does not write there", p)
		}
	}
	return out, nil
}

// InFolder says whether a file named in the file is inside the mood board folder.
func (b *Board) InFolder(rel string) bool {
	root := filepath.Join(filepath.Dir(b.path), filepath.FromSlash(b.Dir)) + string(filepath.Separator)
	return strings.HasPrefix(filepath.Clean(b.Path(filepath.FromSlash(rel))), root)
}

// Changes says what a sync did.
type Changes struct {
	Added   []string `json:"added"`
	Missing []string `json:"missing"`
	Revived []string `json:"revived"`
}

// Slug makes an id from a file name: lower case words joined by dashes.
func Slug(name string) string {
	name = strings.TrimSuffix(name, filepath.Ext(name))
	var sb strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
			dash = false
		} else if !dash && sb.Len() > 0 {
			sb.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(sb.String(), "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	if s == "" {
		s = "item"
	}
	return s
}

func (b *Board) uniqueID(base string) string {
	id := base
	for n := 2; b.Find(id) != nil; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

// Sync brings the file in line with the folder: it adds the images and clips that are new,
// marks the ones that are gone, and brings back the ones that returned. Ids, takes and
// "dropped" survive. Videos recorded in the manifest as embeds, with a thumbnail inside the
// folder, are added too. Source, licence and credit are copied from the manifest every time.
// The caller saves the board.
func Sync(jsonPath, dir string, m *library.Manifest, root string) (*Board, Changes, error) {
	var ch Changes
	absJSON, err := filepath.Abs(jsonPath)
	if err != nil {
		return nil, ch, err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, ch, err
	}
	jsonDir := filepath.Dir(absJSON)
	relDir, err := filepath.Rel(jsonDir, absDir)
	if err != nil || relDir == ".." || strings.HasPrefix(relDir, ".."+string(filepath.Separator)) {
		return nil, ch, fmt.Errorf("the folder %s must be inside the folder of %s", dir, jsonPath)
	}
	relDir = filepath.ToSlash(relDir)
	b, err := Load(absJSON)
	switch {
	case os.IsNotExist(err):
		b = New(absJSON, relDir)
	case err != nil:
		return nil, ch, err
	}
	b.path = absJSON
	b.Dir = relDir
	if relDir == "." {
		b.Dir = ""
	}

	entries := map[string]library.Entry{}
	if m != nil {
		for _, e := range m.Assets {
			entries[e.Path] = e
		}
	}
	apply := func(it *Item, e library.Entry) {
		if e.Source != "" {
			it.Source = e.Source
		}
		if e.Licence != "" {
			it.Licence = e.Licence
		}
		credit := e.Attribution
		if credit == "" && e.AttributionRequired {
			credit = library.DefaultAttribution(e)
		}
		if credit != "" {
			it.Credit = credit
		}
		if e.Title != "" {
			it.Title = e.Title
		}
		if e.URL != "" && it.EffectiveKind() != KindEmbed {
			it.URL = e.URL
		}
	}

	seen := map[string]bool{}
	files, err := os.ReadDir(absDir)
	if err != nil {
		return nil, ch, err
	}
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, ".") || !f.Type().IsRegular() {
			continue
		}
		kind := KindOfFile(name)
		if kind == "" {
			continue
		}
		abs := filepath.Join(absDir, name)
		rel, err := filepath.Rel(jsonDir, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		var it *Item
		for i := range b.Items {
			if f := b.Items[i].File; f != "" && filepath.Clean(b.Path(filepath.FromSlash(f))) == abs {
				it = &b.Items[i]
				it.File = rel // one canonical form, whatever the file said
			}
		}
		if it == nil {
			b.Items = append(b.Items, Item{ID: b.uniqueID(Slug(name)), Kind: kind, File: rel, AddedBy: ByAgent})
			it = &b.Items[len(b.Items)-1]
			ch.Added = append(ch.Added, it.ID)
		} else if it.Status == StatusMissing {
			it.Status = ""
			ch.Revived = append(ch.Revived, it.ID)
		}
		seen[it.ID] = true
		if it.Kind == "" {
			it.Kind = kind
		}
		if kind == KindClip {
			it.Thumb = b.ClipThumb(it.ID)
		}
		if e, ok := entries[library.Rel(root, abs)]; ok {
			apply(it, e)
		}
	}

	// Videos that are embedded, not downloaded: the manifest holds their thumbnails.
	thumbRoot := filepath.Join(absDir, ThumbDir) + string(filepath.Separator)
	var embeds []library.Entry
	for _, e := range entries {
		if e.Kind != "embed" {
			continue
		}
		p := e.Path
		if !filepath.IsAbs(filepath.FromSlash(p)) {
			p = filepath.Join(root, filepath.FromSlash(p))
		}
		if !strings.HasPrefix(filepath.Clean(p), thumbRoot) {
			continue
		}
		embeds = append(embeds, e)
	}
	sort.Slice(embeds, func(i, j int) bool { return embeds[i].Path < embeds[j].Path })
	seenEmbed := map[string]bool{}
	for _, e := range embeds {
		provider, vid, hash, ok := ParseVideoURL(e.URL)
		if !ok {
			continue
		}
		id := EmbedID(provider, vid)
		p := e.Path
		if !filepath.IsAbs(filepath.FromSlash(p)) {
			p = filepath.Join(root, filepath.FromSlash(p))
		}
		thumb, err := filepath.Rel(jsonDir, p)
		if err != nil {
			continue
		}
		it := b.Find(id)
		if it == nil {
			b.Items = append(b.Items, Item{ID: id, Kind: KindEmbed, URL: PageURL(provider, vid, hash), AddedBy: ByAgent})
			it = &b.Items[len(b.Items)-1]
			ch.Added = append(ch.Added, id)
		}
		if it.Kind == "" {
			it.Kind = KindEmbed
		}
		it.Thumb = filepath.ToSlash(thumb)
		apply(it, e)
		seenEmbed[id] = true
	}

	for i := range b.Items {
		it := &b.Items[i]
		// A thumbnail cav did not record is not trusted: the page reads it, so it must be a
		// file inside the thumbnail folder.
		if it.EffectiveKind() == KindEmbed && !seenEmbed[it.ID] && it.Thumb != "" && !b.inThumbs(it.Thumb) {
			it.Thumb = ""
		}
		if it.EffectiveKind() == KindEmbed || it.Status != "" || seen[it.ID] || it.File == "" {
			continue
		}
		it.Status = StatusMissing
		ch.Missing = append(ch.Missing, it.ID)
	}
	return b, ch, nil
}
