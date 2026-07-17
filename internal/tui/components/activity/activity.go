package activity

import "slices"

// maxLog caps how many entries Log returns and, by extension, how many the
// registry retains: older records are dropped once this many newer ones exist.
// The operation-log overlay only ever scrolls the recent tail, so an unbounded
// history would waste memory in a long-lived session for no visible benefit.
const maxLog = 100

// Status is a recorded operation's lifecycle state.
type Status int

const (
	StatusPending Status = iota // in flight
	StatusDone                  // resolved successfully
	StatusFailed                // resolved with an error
)

// Entry is one recorded operation. It is a value type: Recent and Log hand out
// copies, so a caller reading the footer or the log can never reach back in and
// mutate the registry's own state.
type Entry struct {
	ID       uint64 // registry-assigned, monotonic from 1
	Pending  string // in-flight text, e.g. "creating issue in PROJ…"
	Done     string // resolved text, e.g. "PROJ-88 created" (set by Finish)
	IssueKey string // the affected issue key when known, for a hyperlink (may be "")
	Status   Status
	Err      error // set by Fail
}

// Registry records mutations for the footer and the operation log. It is
// purely observational: Start/Finish/Fail note what a mutation is doing and how
// it ended, but nothing here ever blocks or de-duplicates a write — the
// caller's own re-entrancy guard stays authoritative, and the registry just
// watches from the side.
//
// All methods are called from the single Bubble Tea Update goroutine (the async
// work runs off-loop in a task command and reports back as a message that is
// applied on that same goroutine), so the entry slice needs no locking, exactly
// as internal/tui/components/task's Manager avoids locking its generation map.
// The registry owns no goroutines.
type Registry struct {
	// entries holds recorded operations in start order (oldest first), so a
	// pending op can be resolved in place at the same ID and Log can simply
	// walk it in reverse for newest-first output. It is bounded to maxLog by
	// dropping the oldest on append.
	entries []Entry
	// lastID is the id issued by the most recent Start (0 before any), so ids
	// run monotonic from 1 even on a zero-value Registry. It only ever
	// increases, so dropping old entries never renumbers survivors and a stale
	// id can never collide with a live one.
	lastID uint64
}

// New returns an empty registry. The first Start will hand out id 1.
func New() *Registry {
	return &Registry{}
}

// Start records a pending operation and returns its id (monotonic from 1). The
// returned id is the caller's handle for the later Finish or Fail; a zero-value
// handle is never a valid id, so "not recorded" is unambiguous. When the log is
// already at capacity the oldest entry is dropped, which never affects ids
// because lastID keeps climbing.
func (r *Registry) Start(pending string) uint64 {
	r.lastID++
	id := r.lastID
	r.entries = append(r.entries, Entry{
		ID:      id,
		Pending: pending,
		Status:  StatusPending,
	})
	if len(r.entries) > maxLog {
		// Drop the oldest so retention stays bounded; the survivors keep
		// their original ids.
		r.entries = r.entries[len(r.entries)-maxLog:]
	}
	return id
}

// Finish resolves op id as done: done is the resolved text, issueKey the
// affected key (or ""). The matching pending entry is updated in place at the
// same id. A no-op if id is unknown — it may have aged out of the bounded log,
// and a purely observational record must never panic on a stale handle.
func (r *Registry) Finish(id uint64, done, issueKey string) {
	if e := r.find(id); e != nil {
		e.Done = done
		e.IssueKey = issueKey
		e.Status = StatusDone
	}
}

// Fail resolves op id as failed with err. Like Finish it updates in place and
// is a no-op for an unknown id.
func (r *Registry) Fail(id uint64, err error) {
	if e := r.find(id); e != nil {
		e.Err = err
		e.Status = StatusFailed
	}
}

// InFlight returns how many recorded operations are still pending.
func (r *Registry) InFlight() int {
	n := 0
	for i := range r.entries {
		if r.entries[i].Status == StatusPending {
			n++
		}
	}
	return n
}

// Recent returns the most-recently-started entry (the highest id, whether still
// pending or already resolved) and true, or a zero Entry and false when nothing
// has been recorded. Entries are kept in start order, so the last one is the
// newest.
func (r *Registry) Recent() (Entry, bool) {
	if len(r.entries) == 0 {
		return Entry{}, false
	}
	return r.entries[len(r.entries)-1], true
}

// Log returns the recorded entries newest-first, capped at the most recent
// maxLog. Retention is already bounded to maxLog, so this walks the retained
// slice in reverse into a fresh copy — the caller gets its own snapshot and
// cannot mutate registry state through it.
func (r *Registry) Log() []Entry {
	out := make([]Entry, 0, len(r.entries))
	for _, e := range slices.Backward(r.entries) {
		out = append(out, e)
	}
	return out
}

// find returns a pointer to the entry with the given id, or nil if none. The
// slice is small (bounded by maxLog) and only touched on the Update goroutine,
// so a linear scan is more than fast enough and avoids a parallel index map.
func (r *Registry) find(id uint64) *Entry {
	for i := range r.entries {
		if r.entries[i].ID == id {
			return &r.entries[i]
		}
	}
	return nil
}
