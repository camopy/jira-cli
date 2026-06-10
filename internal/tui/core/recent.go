package core

// RecentIssue is one jumplist entry: the issue key plus the summary shown in
// the picker.
type RecentIssue struct {
	Key     string
	Summary string
}

// recentCap bounds the jumplist; past it the oldest entry falls off.
const recentCap = 50

// RecentList is the app-wide recently-viewed issue history, most recent
// first. It is shared through the ProgramContext so every section feeds and
// reads the same jumplist. Not safe for concurrent use; the TUI mutates it
// from the single Update goroutine.
type RecentList struct {
	entries []RecentIssue
}

// NewRecentList returns an empty history.
func NewRecentList() *RecentList { return &RecentList{} }

// Touch records a visit: the issue moves to (or enters at) the front with
// the given summary. Empty keys are ignored; an empty summary keeps the one
// already recorded (jumping to an issue from history opens it via a bare-key
// stub, which must not wipe the known summary).
func (r *RecentList) Touch(key, summary string) {
	if key == "" {
		return
	}
	if summary == "" {
		for _, e := range r.entries {
			if e.Key == key {
				summary = e.Summary
				break
			}
		}
	}
	out := make([]RecentIssue, 0, len(r.entries)+1)
	out = append(out, RecentIssue{Key: key, Summary: summary})
	for _, e := range r.entries {
		if e.Key != key {
			out = append(out, e)
		}
	}
	if len(out) > recentCap {
		out = out[:recentCap]
	}
	r.entries = out
}

// List returns the history, most recent first.
func (r *RecentList) List() []RecentIssue { return r.entries }
