package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Stills names the target of a review of stills.
type Stills struct {
	Kind Kind
	Path string // brief.json, moodboard.json or storyboard.json
}

var errItemChanged = errors.New("an item changed since the page loaded; look again")

// blockedError says the stage cannot be approved, and why.
type blockedError struct{ blockers []string }

func (e *blockedError) Error() string {
	return "the stage cannot be approved yet: " + strings.Join(e.blockers, "; ")
}

// items loads the items. When the target cannot be read (an agent may be in the middle of
// editing it), it returns the last items that loaded, with the error, so the page stays usable.
func (s *Server) items() ([]Item, error) {
	items, err := Items(s.Stills.Kind, s.Stills.Path)
	s.itemsMu.Lock()
	defer s.itemsMu.Unlock()
	if err != nil {
		s.lastItemsE = err.Error()
		return s.lastItems, err
	}
	s.lastItems, s.lastItemsE = items, ""
	return items, nil
}

// update is Store.Update, and for stills it also drops an approval that the change ended:
// a new redo, a reopened note, an item that changed.
func (s *Server) update(fn func(d *Doc) error) (*Doc, error) {
	if s.Stills == nil {
		return s.Store.Update(fn)
	}
	items, ierr := s.items()
	return s.Store.Update(func(d *Doc) error {
		if err := fn(d); err != nil {
			return err
		}
		if ierr == nil {
			Reconcile(d, items, time.Now().UTC())
		}
		return nil
	})
}

// reconcile removes an approval that no longer holds, before the state is read. There is no
// background loop: the page polls, and every write goes through update.
func (s *Server) reconcile(items []Item) {
	d, err := s.Store.Load()
	if err != nil {
		return
	}
	probe := *d
	if !Reconcile(&probe, items, time.Now().UTC()) {
		return
	}
	if _, err := s.Store.Update(func(d *Doc) error { Reconcile(d, items, time.Now().UTC()); return nil }); err == nil {
		s.changed()
	}
}

// itemView is an item with what the reviewer said about it.
type itemView struct {
	Item
	ItemState
}

func (s *Server) stillsState(w http.ResponseWriter) {
	items, ierr := s.items()
	if ierr == nil {
		s.reconcile(items)
	}
	d, err := s.Store.Load()
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	states := ItemStates(d, items)
	views := make([]itemView, 0, len(items))
	for _, it := range items {
		views = append(views, itemView{it, states[it.ID]})
	}
	blockers := Blockers(d, items)
	if blockers == nil {
		blockers = []string{}
	}
	target := ""
	if ierr != nil {
		target = ierr.Error()
	}
	writeJSON(w, map[string]any{
		"kind": string(s.Stills.Kind), "author": s.Author, "doc": d, "name": filepath.Base(s.Stills.Path),
		"version": shortSHA(TargetSHA(s.Stills.Path)), "items": views, "verdicts": Verdicts(s.Stills.Kind),
		"approval":        map[string]any{"approved": d.Approved != nil, "canApprove": ierr == nil && len(items) > 0 && len(blockers) == 0, "blockers": blockers},
		"approvalCleared": d.ApprovalCleared, "targetError": target,
	})
}

// itemImage serves an item's image, and nothing else. The target file comes from the project
// and could name any path, so SafeImage decides what may leave the folder.
func (s *Server) itemImage(w http.ResponseWriter, r *http.Request) {
	if s.Stills == nil {
		http.NotFound(w, r)
		return
	}
	items, _ := s.items()
	for _, it := range items {
		if it.ID != r.PathValue("id") {
			continue
		}
		p := SafeImage(s.Stills.Path, it.Image)
		if p == "" {
			break
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, p)
		return
	}
	http.NotFound(w, r)
}

func isFullSHA(s string) bool {
	return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == ""
}

// itemFileName makes an item id safe to use in a snapshot file name.
func itemFileName(id string) string {
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, id)
	if len(name) > 40 {
		name = name[:40]
	}
	return name
}

// addItemComment adds a note, a verdict or both on one item.
func (s *Server) addItemComment(w http.ResponseWriter, in commentInput) {
	items, ierr := s.items()
	var item *Item
	for i := range items {
		if items[i].ID == in.Item {
			item = &items[i]
		}
	}
	switch {
	case item == nil && ierr != nil:
		httpErr(w, 409, ierr)
		return
	case item == nil:
		httpErr(w, 400, fmt.Errorf("there is no item %q", in.Item))
		return
	}
	if in.ItemSHA == "" {
		in.ItemSHA = item.SHA
	} else if !isFullSHA(in.ItemSHA) {
		httpErr(w, 400, errors.New("itemSha must be the item's full sha256"))
		return
	}
	if in.Verdict != "" && !contains(Verdicts(s.Stills.Kind), in.Verdict) {
		httpErr(w, 400, fmt.Errorf("a verdict on a %s is %s, not %q", s.Stills.Kind, strings.Join(Verdicts(s.Stills.Kind), " or "), in.Verdict))
		return
	}
	bare := in.Text == "" && len(in.Shapes) == 0
	if bare && in.Verdict == "" {
		httpErr(w, 400, errors.New("write a note, draw something or give a verdict"))
		return
	}
	png, err := decodePNG(in.Snapshot)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	version := shortSHA(TargetSHA(s.Stills.Path))
	var made Comment
	_, err = s.update(func(d *Doc) error {
		now := time.Now().UTC()
		c := Comment{
			ID: NextID(d), Version: version, Fragment: ItemFragment(in.Shapes),
			Author: s.Author, CreatedAt: now, Status: "open", Text: in.Text, Shapes: in.Shapes,
			Item: in.Item, ItemSHA: in.ItemSHA, Verdict: in.Verdict,
		}
		if bare && !Actionable(in.Verdict) {
			c.Status, c.ResolvedAt = "resolved", &now // a plain approve or keep asks the agent for nothing
		}
		if bare {
			// A new bare verdict replaces the earlier ones on the item. One that no agent has
			// seen yet is taken back in place; one that was sent is resolved with a note.
			kept := d.Comments[:0:0]
			reused := false
			for _, o := range d.Comments {
				if o.Item == in.Item && o.Verdict != "" && !isNote(o) && o.Sent == 0 && o.ItemSHA == in.ItemSHA && !reused {
					c.ID, c.CreatedAt, reused = o.ID, o.CreatedAt, true
					continue
				}
				kept = append(kept, o)
			}
			d.Comments = kept
			for i := range d.Comments {
				o := &d.Comments[i]
				if o.Item == in.Item && o.Verdict != "" && !isNote(*o) && o.Status == "open" {
					SetStatus(o, "resolved", "replaced by "+in.Verdict+" in "+c.ID)
				}
			}
		}
		if png != nil {
			if err := os.MkdirAll(s.Store.SnapDir, 0o755); err != nil {
				return err
			}
			c.Snapshot = filepath.Join(s.Store.SnapDir, fmt.Sprintf("%s_%s.png", c.ID, itemFileName(in.Item)))
			if err := os.WriteFile(c.Snapshot, png, 0o644); err != nil {
				return err
			}
		}
		d.Comments = append(d.Comments, c)
		made = c
		return nil
	})
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	s.changed()
	writeJSON(w, made)
}

// approve records that the person approved the stage, or takes the approval back. Only the
// page calls it: there is deliberately no command for the agent to approve its own work.
func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	if s.Stills == nil {
		http.NotFound(w, r)
		return
	}
	var in struct {
		Items    map[string]string `json:"items"`
		Withdraw bool              `json:"withdraw"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		httpErr(w, 400, err)
		return
	}
	if in.Withdraw {
		_, err := s.Store.Update(func(d *Doc) error { d.Approved = nil; return nil })
		if err != nil {
			httpErr(w, 500, err)
			return
		}
		s.changed()
		writeJSON(w, map[string]any{"approved": false})
		return
	}
	items, ierr := s.items()
	if ierr != nil {
		httpErr(w, 409, ierr)
		return
	}
	var sent *Send
	d, err := s.Store.Update(func(d *Doc) error {
		now := time.Now().UTC()
		Reconcile(d, items, now)
		// What the person approved is what they saw. If an item moved on since, they look again.
		if len(in.Items) != len(items) {
			return errItemChanged
		}
		for _, it := range items {
			if in.Items[it.ID] != it.SHA {
				return errItemChanged
			}
		}
		if b := Blockers(d, items); len(b) > 0 {
			return &blockedError{b}
		}
		d.Approved = &Approval{By: s.Author, At: now, Items: in.Items}
		d.ApprovalCleared = nil
		// The approval is a send, so `wait`, the hook and review_notes see it. It also hands
		// over the open notes nobody has seen yet, such as "more like this" on a reference.
		n := len(d.Sends) + 1
		ids := []string{}
		for i := range d.Comments {
			if c := &d.Comments[i]; c.Status == "open" && c.Sent == 0 {
				c.Sent = n
				ids = append(ids, c.ID)
			}
		}
		d.Sends = append(d.Sends, Send{N: n, At: now, Comments: ids, Approved: true})
		sent = &d.Sends[len(d.Sends)-1]
		return nil
	})
	if err != nil {
		var be *blockedError
		if errors.As(err, &be) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "blockers": be.blockers})
			return
		}
		httpErr(w, statusOf(err), err)
		return
	}
	s.changed()
	writeJSON(w, map[string]any{"approved": d.Approved != nil, "send": sent})
}
