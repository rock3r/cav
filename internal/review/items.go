package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rock3r/cav/internal/board"
	"github.com/rock3r/cav/internal/brief"
)

// Kind is what a review is about: a rendered video, or one of the stills the workflow
// asks the person to approve before slow or paid work starts.
type Kind string

const (
	KindVideo      Kind = "video"
	KindBrief      Kind = "brief"
	KindMoodboard  Kind = "moodboard"
	KindStoryboard Kind = "storyboard"
)

// Until the mood board has its own package, its schema lives here.
const MoodboardSchema = "cav-moodboard/1"

// Item is one thing the reviewer judges on a stills page: a shot, a reference or a field of
// the brief. SHA changes when the item changes, and a verdict is only valid for the SHA it
// was given on.
type Item struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text,omitempty"`
	Link  string `json:"link,omitempty"`
	// Image is the file the page shows; it never leaves the server (see SafeImage).
	Image    string `json:"-"`
	HasImage bool   `json:"hasImage"`
	// Meta marks a setting of the whole file, such as the storyboard's tempo and style. It
	// is reviewed like the rest but not counted as an item.
	Meta bool   `json:"meta,omitempty"`
	SHA  string `json:"sha"`
}

// Loader reads the items of one kind of target.
type Loader func(path string) ([]Item, error)

var loaders = map[Kind]Loader{
	KindStoryboard: storyboardItems,
	KindBrief:      briefItems,
	KindMoodboard:  moodboardItems,
}

// KindOf says what a review target is. Any file that is not .json is a video. A .json file
// names its kind in "schema", or by its file name when it has none.
func KindOf(path string) (Kind, error) {
	if strings.ToLower(filepath.Ext(path)) != ".json" {
		return KindVideo, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var head struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	switch {
	case strings.HasPrefix(head.Schema, "cav-board/"):
		return KindStoryboard, nil
	case strings.HasPrefix(head.Schema, "cav-moodboard/"):
		return KindMoodboard, nil
	case strings.HasPrefix(head.Schema, "cav-brief/"):
		return KindBrief, nil
	case head.Schema == "":
		switch filepath.Base(path) {
		case "storyboard.json":
			return KindStoryboard, nil
		case "moodboard.json":
			return KindMoodboard, nil
		case "brief.json":
			return KindBrief, nil
		}
	}
	return "", errors.New("cav review takes a video, brief.json, moodboard.json or storyboard.json")
}

// Items lists the items of a stills target, each with its hash.
func Items(kind Kind, path string) ([]Item, error) {
	load := loaders[kind]
	if load == nil {
		return nil, fmt.Errorf("%s has no items", kind)
	}
	items, err := load(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, it := range items {
		if it.ID == "" {
			return nil, errors.New("an item has no id")
		}
		if seen[it.ID] {
			return nil, fmt.Errorf("two items are called %s", it.ID)
		}
		seen[it.ID] = true
	}
	return items, nil
}

// Verdicts lists what the reviewer can say about an item of this kind.
func Verdicts(k Kind) []string {
	switch k {
	case KindMoodboard:
		return []string{"keep", "more", "drop"}
	case KindBrief, KindStoryboard:
		return []string{"approve", "redo"}
	}
	return nil
}

// Blocking verdicts stop the stage approval until they change.
func Blocking(v string) bool { return v == "redo" || v == "drop" }

// Actionable verdicts ask the agent to do something, so they stay open until it has.
func Actionable(v string) bool { return v == "redo" || v == "drop" || v == "more" }

func sum(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(h[:])
}

type digestKey struct {
	path string
	size int64
	mod  int64
}

var (
	digestMu    sync.Mutex
	digestCache = map[digestKey]string{}
)

// cachedDigest is Digest for a file that is read again and again: the page polls every few
// seconds, and the file has not changed when its size and time have not.
func cachedDigest(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	k := digestKey{abs, info.Size(), info.ModTime().UnixNano()}
	digestMu.Lock()
	d, ok := digestCache[k]
	digestMu.Unlock()
	if ok {
		return d, nil
	}
	if d, err = Digest(abs); err != nil {
		return "", err
	}
	digestMu.Lock()
	if len(digestCache) > 4096 {
		digestCache = map[digestKey]string{}
	}
	digestCache[k] = d
	digestMu.Unlock()
	return d, nil
}

// TargetSHA is the hash of the target file itself, which names the version of the page.
func TargetSHA(path string) string {
	d, _ := cachedDigest(path)
	return d
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}

// SafeImage returns the real path of an item's image, or "" when the page must not show it.
// A target file comes from the project and could name any path, so the image has to be a
// regular image file that is not a symlink and whose real place is inside the target's folder.
func SafeImage(target, img string) string {
	if img == "" || !imageExts[strings.ToLower(filepath.Ext(img))] {
		return ""
	}
	info, err := os.Lstat(img)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	// Both are made absolute first: a relative path stays relative when its links are
	// resolved, and /tmp is itself a link on some systems.
	absRoot, err := filepath.Abs(filepath.Dir(target))
	if err != nil {
		return ""
	}
	absImg, err := filepath.Abs(img)
	if err != nil {
		return ""
	}
	root, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return ""
	}
	real, err := filepath.EvalSymlinks(absImg)
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(real, root+string(filepath.Separator)) {
		return ""
	}
	return real
}

func storyboardItems(path string) ([]Item, error) {
	sb, err := board.Load(path)
	if err != nil {
		return nil, err
	}
	// The settings are an item of their own: tempo, format, offset, grid and style change what
	// every shot means, so a change to them must remove the approval, whatever the shots say.
	settings := *sb
	settings.Shots = nil
	sj, _ := json.Marshal(settings)
	grid := ""
	if sb.Grid != "" {
		grid = "missing:" + sb.Grid
		if d, err := cachedDigest(sb.Path(sb.Grid)); err == nil {
			grid = d
		}
	}
	id := "board"
	for _, shot := range sb.Shots {
		if shot.ID == id {
			id = "storyboard-settings" // a shot already has the name
		}
	}
	stext := fmt.Sprintf("%d x %d at %g fps, %g bpm, %g s", sb.Width, sb.Height, sb.FPS, sb.BPM, sb.Seconds)
	if sb.Offset != 0 {
		stext += fmt.Sprintf(", music offset %g s", sb.Offset)
	}
	if sb.Grid != "" {
		stext += ", beat grid " + sb.Grid
	}
	if sb.Style.Prompt != "" {
		stext += "\nStyle: " + sb.Style.Prompt
	}
	if len(sb.Style.Palette) > 0 {
		stext += "\nPalette: " + strings.Join(sb.Style.Palette, ", ")
	}
	if len(sb.Style.Refs) > 0 {
		stext += "\nStyle references: " + strings.Join(sb.Style.Refs, ", ")
	}
	items := []Item{{ID: id, Title: "Storyboard settings", Text: stext, Meta: true, SHA: sum("cav-item/1", "board", string(sj), grid)}}
	for _, shot := range sb.Shots {
		frame := sb.Path(shot.Frame)
		digest := ""
		if shot.Frame != "" {
			// The bytes count even for a frame the page may not show (see SafeImage).
			digest = "missing:" + shot.Frame
			if d, err := cachedDigest(frame); err == nil {
				digest = d
			}
		}
		img := SafeImage(path, frame)
		j, _ := json.Marshal(shot)
		text := shot.What
		if shot.Camera != "" {
			text += "\nCamera: " + shot.Camera
		}
		items = append(items, Item{ID: shot.ID, Title: sb.Label(shot), Text: text, Image: img, HasImage: img != "",
			SHA: sum("cav-item/1", "shot", string(j), digest)})
	}
	return items, nil
}

// briefItems makes one item per field of brief.json. The brief package says which fields
// there are, in what order and in what words. The hash stays what it always was: the field's
// own JSON, so a verdict given before the brief had its own package still holds. The words
// the page shows never go into the hash.
func briefItems(path string) ([]Item, error) {
	fields, err := brief.Fields(path)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, f := range fields {
		var v any
		if err := json.Unmarshal(f.Raw, &v); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", path, f.Key, err)
		}
		canon, _ := json.Marshal(v) // map keys come out sorted, so spacing and key order do not matter
		items = append(items, Item{ID: f.Key, Title: f.Title, Text: f.Text, SHA: sum("cav-item/1", "brief", f.Key, string(canon))})
	}
	return items, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// moodboardItems reads the references of moodboard.json: {items:[{id,file,url,thumb,take}]}.
// It is deliberately minimal: the mood board's own schema comes with `cav moodboard`.
func moodboardItems(path string) ([]Item, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mb struct {
		Items []struct {
			ID    string `json:"id"`
			File  string `json:"file"`
			URL   string `json:"url"`
			Thumb string `json:"thumb"`
			Take  string `json:"take"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &mb); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	dir := filepath.Dir(path)
	at := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(dir, p)
	}
	var items []Item
	for _, r := range mb.Items {
		id := r.ID
		if id == "" && r.File != "" {
			id = filepath.Base(r.File)
		}
		if id == "" {
			id = r.URL
		}
		// The file's bytes and every word the reviewer sees go into the hash.
		digest := "url:" + r.URL
		if r.File != "" {
			d, err := cachedDigest(at(r.File))
			if err != nil {
				d = "missing:" + r.File
			}
			digest = d
		}
		sha := sum("cav-item/1", "ref", digest, r.Take, r.URL, r.Thumb)
		img := SafeImage(path, at(r.File))
		if img == "" {
			img = SafeImage(path, at(r.Thumb))
		}
		items = append(items, Item{ID: id, Title: id, Text: r.Take, Link: r.URL, Image: img, HasImage: img != "", SHA: sha})
	}
	return items, nil
}

// ItemState is what the reviewer has said about one item, as of the item's current hash.
type ItemState struct {
	Verdict   string `json:"verdict,omitempty"`
	VerdictID string `json:"verdictId,omitempty"`
	// Outdated is set when the newest verdict was given to an earlier version of the item;
	// OldVerdict is that verdict, which no longer counts.
	Outdated   bool   `json:"outdated,omitempty"`
	OldVerdict string `json:"oldVerdict,omitempty"`
	OpenNotes  int    `json:"openNotes"`
	// Gone is set for an item id that notes name but the target no longer has.
	Gone bool `json:"gone,omitempty"`
}

// isNote tells a note (words or a drawing) from a bare verdict.
func isNote(c Comment) bool { return c.Text != "" || len(c.Shapes) > 0 }

// replacedNote starts the note of a verdict that a newer verdict on the same item replaced.
const replacedNote = "replaced by "

// replaced reports whether a newer verdict took the place of this one. It no longer counts,
// so deleting the newer verdict does not bring it back.
func replaced(c Comment) bool {
	return c.Status == "resolved" && strings.HasPrefix(c.ResolvedNote, replacedNote)
}

// ItemStates gives the state of every item, and of every other item id that a note names.
func ItemStates(d *Doc, items []Item) map[string]ItemState {
	cur := map[string]string{}
	st := map[string]ItemState{}
	for _, it := range items {
		cur[it.ID] = it.SHA
		st[it.ID] = ItemState{}
	}
	for _, c := range d.Comments {
		if c.Item == "" {
			continue
		}
		sha, known := cur[c.Item]
		s := st[c.Item]
		s.Gone = !known
		if c.Status == "open" && isNote(c) {
			s.OpenNotes++
		}
		if c.Verdict != "" && !replaced(c) { // the newest one wins: comments stay in the order they were made
			if known && c.ItemSHA != sha {
				s.Verdict, s.VerdictID, s.Outdated, s.OldVerdict = "", "", true, c.Verdict
			} else {
				s.Verdict, s.VerdictID, s.Outdated, s.OldVerdict = c.Verdict, c.ID, false, ""
			}
		}
		st[c.Item] = s
	}
	return st
}

// Blockers lists what stops the stage approval: an item marked redo or drop, and any open note.
func Blockers(d *Doc, items []Item) []string {
	var out []string
	st := ItemStates(d, items)
	for _, it := range items {
		if s := st[it.ID]; Blocking(s.Verdict) {
			out = append(out, fmt.Sprintf("%s is marked %s", it.ID, s.Verdict))
		}
	}
	for _, c := range d.Comments {
		if c.Status == "open" && isNote(c) {
			out = append(out, c.ID+" is open")
		}
	}
	return out
}

// Reconcile removes the approval when it no longer holds: an item changed, was added or
// removed, or something blocks again. It reports whether it changed d.
func Reconcile(d *Doc, items []Item, now time.Time) bool {
	if d.Approved == nil {
		return false
	}
	reason := ""
	have := map[string]bool{}
	for _, it := range items {
		have[it.ID] = true
		sha, ok := d.Approved.Items[it.ID]
		switch {
		case !ok && reason == "":
			reason = it.ID + " added"
		case ok && sha != it.SHA && reason == "":
			reason = it.ID + " changed"
		}
	}
	if reason == "" {
		var gone []string
		for id := range d.Approved.Items {
			if !have[id] {
				gone = append(gone, id)
			}
		}
		sort.Strings(gone)
		if len(gone) > 0 {
			reason = gone[0] + " removed"
		}
	}
	if reason == "" {
		if b := Blockers(d, items); len(b) > 0 {
			reason = b[0]
		}
	}
	if reason == "" {
		return false
	}
	d.Approved = nil
	d.ApprovalCleared = &Cleared{At: now, Reason: reason}
	return true
}

// ItemFragment is the box around a drawing, as the W3C media fragment a still can use.
func ItemFragment(shapes []Shape) string {
	if f := boxFragment(shapes); f != "" {
		return "#" + f[1:]
	}
	return ""
}
