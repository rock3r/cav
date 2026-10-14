package review

import (
	"os"
	"time"
)

// Status is what a review file says about its target now. ReadStatus fills it without
// changing any file.
type Status struct {
	Kind       Kind
	ReviewFile string
	HasReview  bool // the review file exists
	Approved   bool
	// ApprovalUnknown is set when the target or its review file cannot be read, so cav cannot
	// tell whether an approval still holds. Reason says why. Approved is then false.
	ApprovalUnknown bool
	Reason          string
	Items           int      // items of a stills target, without the settings of the whole file
	Redo            []string // items marked redo or drop
	Open            []string // ids of open notes
	Cleared         string   // why the approval was removed, while there is none
}

// ReadStatus reads the review of target the way `cav review export` does, but in memory
// only: an approval that no longer holds shows as removed, and nothing is written.
func ReadStatus(target string) Status {
	st := Status{Redo: []string{}, Open: []string{}}
	unknown := func(reason string) Status {
		st.ApprovalUnknown, st.Reason, st.Approved = true, reason, false
		return st
	}
	kind, err := KindOf(target)
	if err != nil {
		return unknown(err.Error())
	}
	st.Kind = kind
	if err := CheckOwner(target, kind); err != nil {
		return unknown(err.Error())
	}
	store := Open(target)
	st.ReviewFile = store.DocPath
	d, err := store.Load()
	if err != nil {
		return unknown(err.Error())
	}
	if _, err := os.Stat(store.DocPath); err == nil {
		st.HasReview = true
	}
	for _, c := range d.Comments {
		if c.Status == "open" && isNote(c) {
			st.Open = append(st.Open, c.ID)
		}
	}
	if kind == KindVideo {
		return st
	}
	items, err := Items(kind, target)
	if err != nil {
		return unknown(err.Error())
	}
	for _, it := range items {
		if !it.Meta {
			st.Items++
		}
	}
	Reconcile(d, items, time.Now())
	states := ItemStates(d, items)
	for _, it := range items {
		if Blocking(states[it.ID].Verdict) {
			st.Redo = append(st.Redo, it.ID)
		}
	}
	st.Approved = d.Approved != nil
	if !st.Approved && d.ApprovalCleared != nil {
		st.Cleared = d.ApprovalCleared.Reason
	}
	return st
}
