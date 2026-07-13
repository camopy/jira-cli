package core

import "github.com/matcra587/jira-cli/internal/tui/components/task"

// The generation-tracked manager lives in the domain-free task package;
// these aliases keep core's established names working for every section
// while the package itself stays liftable into other apps.
type (
	// TaskScope groups async work so a newer task supersedes an older one.
	TaskScope = task.Scope
	// TaskSpec describes a unit of async work run off the UI loop.
	TaskSpec = task.Spec
	// TaskFinishedMsg carries a task result, tagged with its generation.
	TaskFinishedMsg = task.FinishedMsg
	// TaskManager hands out monotonic generations per scope.
	TaskManager = task.Manager
)

// NewTaskManager returns an empty manager.
func NewTaskManager() *TaskManager { return task.New() }
