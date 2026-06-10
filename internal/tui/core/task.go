package core

import tea "charm.land/bubbletea/v2"

// TaskScope groups async work so that a newer task in the same scope supersedes
// an older one. For example all issue fetches share one scope; if the user
// refreshes twice quickly, only the latest result is accepted.
type TaskScope string

// TaskSpec describes a unit of async work. Run executes off the UI loop (Bubble
// Tea runs the returned command in its own goroutine) and must not touch any
// model state — it returns a result that is delivered back as a TaskFinishedMsg
// and applied on the UI loop.
type TaskSpec struct {
	Scope TaskScope
	Run   func() (any, error)
}

// TaskFinishedMsg carries the result of a task. The App drops it unless its
// generation is still the latest for the scope (see TaskManager.Accept), then
// broadcasts it to every section so the one that owns the scope applies it —
// even if the user has since switched to a different view.
type TaskFinishedMsg struct {
	Scope  TaskScope
	Gen    uint64
	Result any
	Err    error
}

// TaskManager hands out monotonic generations per scope so stale results can be
// discarded. All of its methods are called from the single Bubble Tea update
// goroutine, so the generation map needs no locking: Start runs during Update,
// Accept runs during Update, and the async Run closure never touches it.
//
// There is deliberately no "task started" message: a section that launches work
// sets its own loading state synchronously when it calls StartTask, which avoids
// any ordering race against the finished message for fast tasks.
type TaskManager struct {
	gen map[TaskScope]uint64
}

// NewTaskManager returns an empty manager.
func NewTaskManager() *TaskManager {
	return &TaskManager{gen: make(map[TaskScope]uint64)}
}

// Start bumps the scope's generation and returns a command that runs Run off
// the UI loop and reports the outcome as a TaskFinishedMsg tagged with the
// captured generation. A later Start in the same scope makes this task's
// eventual result stale. The map is lazily initialized so a zero-value manager
// is usable.
func (m *TaskManager) Start(spec TaskSpec) tea.Cmd {
	if m.gen == nil {
		m.gen = make(map[TaskScope]uint64)
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
		return TaskFinishedMsg{Scope: scope, Gen: gen, Result: res, Err: err}
	}
}

// Accept reports whether a finished task is still the latest for its scope.
// A false result means the user moved on and the work should be discarded.
func (m *TaskManager) Accept(scope TaskScope, gen uint64) bool {
	return m.gen[scope] == gen
}

// Generation returns the current generation for a scope (0 if never started).
func (m *TaskManager) Generation(scope TaskScope) uint64 {
	return m.gen[scope]
}
