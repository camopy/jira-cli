package main

import (
	"time"

	"github.com/matcra587/jira-cli/pkg/jira"
)

type jiraServiceFactory interface {
	Issue() jira.IssueService
	Epic() jira.EpicService
	Search() jira.SearchService
	Worklog() jira.WorklogService
	Project(time.Duration) jira.ProjectService
}

type defaultJiraServiceFactory struct {
	client *jira.Client
}

func servicesForClient(client *jira.Client) jiraServiceFactory {
	return defaultJiraServiceFactory{client: client}
}

func issueService(client *jira.Client) jira.IssueService {
	return servicesForClient(client).Issue()
}

func epicService(client *jira.Client) jira.EpicService {
	return servicesForClient(client).Epic()
}

func searchService(client *jira.Client) jira.SearchService {
	return servicesForClient(client).Search()
}

func worklogService(client *jira.Client) jira.WorklogService {
	return servicesForClient(client).Worklog()
}

func (f defaultJiraServiceFactory) Issue() jira.IssueService {
	return jira.NewIssueService(f.client)
}

func (f defaultJiraServiceFactory) Epic() jira.EpicService {
	return jira.NewEpicService(f.client)
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
