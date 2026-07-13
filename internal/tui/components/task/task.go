package task

import tea "charm.land/bubbletea/v2"

// Scope groups async work so that a newer task in the same scope supersedes
// an older one. For example all issue fetches share one scope; if the user
// refreshes twice quickly, only the latest result is accepted.
type Scope string

// Spec describes a unit of async work. Run executes off the UI loop (Bubble
// Tea runs the returned command in its own goroutine) and must not touch any
// model state — it returns a result that is delivered back as a FinishedMsg
// and applied on the UI loop.
type Spec struct {
	Scope Scope
	Run   func() (any, error)
}

// FinishedMsg carries the result of a task. The app drops it unless its
// generation is still the latest for the scope (see Manager.Accept), then
// routes it to whichever component owns the scope — even if the user has
// since switched to a different view.
type FinishedMsg struct {
	Scope  Scope
	Gen    uint64
	Result any
	Err    error
}

// Manager hands out monotonic generations per scope so stale results can be
// discarded. All of its methods are called from the single Bubble Tea update
// goroutine, so the generation map needs no locking: Start runs during
// Update, Accept runs during Update, and the async Run closure never touches
// it.
type Manager struct {
	gen map[Scope]uint64
}

// New returns an empty manager.
func New() *Manager {
	return &Manager{gen: make(map[Scope]uint64)}
}

// Start bumps the scope's generation and returns a command that runs Run off
// the UI loop and reports the outcome as a FinishedMsg tagged with the
// captured generation. A later Start in the same scope makes this task's
// eventual result stale. The map is lazily initialized so a zero-value
// manager is usable.
func (m *Manager) Start(spec Spec) tea.Cmd {
	if m.gen == nil {
		m.gen = make(map[Scope]uint64)
	}
	m.gen[spec.Scope]++
	gen := m.gen[spec.Scope]
	scope := spec.Scope
	run := spec.Run
	return func() tea.Msg {
		var (
			res any
			err error
		)
		if run != nil {
			res, err = run()
		}
		return FinishedMsg{Scope: scope, Gen: gen, Result: res, Err: err}
	}
}

// Accept reports whether a finished task is still the latest for its scope.
// A false result means the user moved on and the work should be discarded.
func (m *Manager) Accept(scope Scope, gen uint64) bool {
	return m.gen[scope] == gen
}

// Generation returns the current generation for a scope (0 if never started).
func (m *Manager) Generation(scope Scope) uint64 {
	return m.gen[scope]
}
