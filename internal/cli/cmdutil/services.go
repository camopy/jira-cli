package cmdutil

import (
	"time"

	"github.com/matcra587/jira-cli/internal/jira"
)

// JiraServiceFactory builds the typed Jira services a command needs from a
// single client. Centralizing construction here keeps every command's
// service wiring identical and gives tests one seam to reason about.
type JiraServiceFactory interface {
	Issue() jira.IssueService
	Search() jira.SearchService
	Worklog() jira.WorklogService
	Project(time.Duration) jira.ProjectService
}

type defaultJiraServiceFactory struct {
	client *jira.Client
}

// ServicesForClient returns the default factory bound to client.
func ServicesForClient(client *jira.Client) JiraServiceFactory {
	return defaultJiraServiceFactory{client: client}
}

// IssueService is a shorthand for ServicesForClient(client).Issue().
func IssueService(client *jira.Client) jira.IssueService {
	return ServicesForClient(client).Issue()
}

// SearchService is a shorthand for ServicesForClient(client).Search().
func SearchService(client *jira.Client) jira.SearchService {
	return ServicesForClient(client).Search()
}

// WorklogService is a shorthand for ServicesForClient(client).Worklog().
func WorklogService(client *jira.Client) jira.WorklogService {
	return ServicesForClient(client).Worklog()
}

func (f defaultJiraServiceFactory) Issue() jira.IssueService {
	return jira.NewIssueService(f.client)
}

func (f defaultJiraServiceFactory) Search() jira.SearchService {
	return jira.NewSearchService(f.client)
}

func (f defaultJiraServiceFactory) Worklog() jira.WorklogService {
	return jira.NewWorklogService(f.client)
}

func (f defaultJiraServiceFactory) Project(ttl time.Duration) jira.ProjectService {
	return jira.NewProjectService(f.client, ttl)
}
