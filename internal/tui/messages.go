package tui

import (
	"time"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Submit messages — emitted by overlays/views to request a mutation.
// Preserved from the prior API for backward compatibility with integration
// tests; the App handler dispatches each through the MutationService.

type SubmitEditMsg struct {
	IssueKey string
	Fields   map[string]any
}

type SubmitCreateMsg struct {
	Request *jira.IssueCreateRequest
}

type SubmitTransitionMsg struct {
	IssueKey     string
	TransitionID string
	Fields       map[string]any
}

type SubmitCommentMsg struct {
	IssueKey string
	Body     adf.Document
}

type SubmitWorklogMsg struct {
	IssueKey         string
	TimeSpentSeconds int
	Started          string
	Comment          *adf.Document
}

type SubmitCloneMsg struct {
	IssueKey string
	Request  *jira.IssueCloneRequest
}

type SubmitMoveMsg struct {
	IssueKey string
	Request  *jira.IssueMoveRequest
}

type SubmitDeleteMsg struct {
	IssueKey string
	Confirm  bool
}

// IssueSelected is emitted by the issue list when the user opens a row.
type IssueSelected struct {
	Issue jira.Issue
}

// Internal lifecycle messages.

type tickMsg time.Time

type issuesLoadedMsg struct {
	issues []*jira.Issue
	err    error
}

type clearStatusMsg struct{ id int }

type mutationDoneMsg struct {
	kind     string
	issueKey string
	err      error
}
