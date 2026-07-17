package core

import "github.com/matcra587/jira-cli/internal/jira"

// Services is the narrow seam Sections use to reach Jira. It exposes only the
// service interfaces the TUI needs, so sections can be unit-tested with fakes
// without constructing a real client.
//
// Only the interface lives in core: it depends solely on the jira domain
// package. The concrete adapter that wraps the CLI's service factory belongs in
// the wiring layer (added at cutover), so core never imports the cli packages
// and no import cycle can form.
type Services interface {
	Issues() jira.IssueService
	Search() jira.SearchService
	JQL() jira.JQLService
	Users() jira.UserService
	Worklogs() jira.WorklogService
	// Projects backs the create form's issue-type list (ListIssueTypes).
	Projects() jira.ProjectService
	// Labels backs the create form's label suggestions.
	Labels() jira.LabelService
}
