package jira

import (
	"context"
	"net/http"
)

type EpicService interface {
	List(context.Context, *ListOptions) ([]*Issue, *Response, error)
	Get(context.Context, string, *IssueGetOptions) (*Issue, *Response, error)
	AddIssue(context.Context, string, string) (*Response, error)
	RemoveIssue(context.Context, string) (*Response, error)
	IssuesInEpic(context.Context, string) ([]*Issue, *Response, error)
}

type epicService struct {
	client *Client
}

func NewEpicService(client *Client) EpicService {
	return &epicService{client: client}
}

func (s *epicService) List(ctx context.Context, opts *ListOptions) ([]*Issue, *Response, error) {
	req := &SearchRequest{
		JQL: "issuetype=Epic",
		// Without an explicit Fields list Jira's /search/jql returns only
		// the keys — callers (epic list, epic board, cache epics) all want
		// at least summary and status, so request them up front.
		Fields: []string{"summary", "status"},
	}
	if opts != nil {
		req.ListOptions = *opts
	}
	return NewSearchService(s.client).JQL(ctx, req)
}

func (s *epicService) Get(ctx context.Context, key string, opts *IssueGetOptions) (*Issue, *Response, error) {
	return NewIssueService(s.client).Get(ctx, key, opts)
}

func (s *epicService) AddIssue(ctx context.Context, epicKey, issueKey string) (*Response, error) {
	payload := map[string]any{"fields": map[string]any{"parent": map[string]string{"key": epicKey}}}
	req, err := s.client.NewRequest(ctx, http.MethodPut, "rest/api/3/issue/"+issueKey, payload)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

func (s *epicService) RemoveIssue(ctx context.Context, issueKey string) (*Response, error) {
	payload := map[string]any{"fields": map[string]any{"parent": nil}}
	req, err := s.client.NewRequest(ctx, http.MethodPut, "rest/api/3/issue/"+issueKey, payload)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

func (s *epicService) IssuesInEpic(ctx context.Context, epicKey string) ([]*Issue, *Response, error) {
	return NewIssueService(s.client).List(ctx, &IssueListOptions{JQL: "parent=" + epicKey})
}

func StatusCounts(issues []*Issue) map[string]int {
	counts := map[string]int{"To Do": 0, "In Progress": 0, "Done": 0}
	for _, issue := range issues {
		if issue == nil || issue.Fields == nil || issue.Fields.Status == nil || issue.Fields.Status.Name == nil {
			continue
		}
		counts[*issue.Fields.Status.Name]++
	}
	return counts
}
