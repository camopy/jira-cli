package jira

import (
	"context"
	"net/http"

	"github.com/matcra587/jira-cli/internal/jql"
)

// EpicService reads epics and manages epic membership. Jira Cloud has no
// dedicated epic REST resource — an epic is an ordinary issue and membership is
// the child's parent field — so this service is a thin façade over the issue
// and search services, existing to keep the epic vocabulary out of callers.
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

// NewEpicService constructs an EpicService bound to the given client.
func NewEpicService(client *Client) EpicService {
	return &epicService{client: client}
}

// List returns the epics visible to the caller, ordered by the shared epic
// JQL. It requests summary and status up front so the common callers (epic
// list, epic board, cache) do not each pay a second fetch.
func (s *epicService) List(ctx context.Context, opts *ListOptions) ([]*Issue, *Response, error) {
	req := &SearchRequest{
		JQL: jql.EpicListJQL,
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

// Get fetches a single epic. It delegates to the issue service because an epic
// is just an issue; the method exists so callers can read in epic terms.
func (s *epicService) Get(ctx context.Context, key string, opts *IssueGetOptions) (*Issue, *Response, error) {
	return NewIssueService(s.client).Get(ctx, key, opts)
}

// AddIssue makes issueKey a child of epicKey by setting its parent field. Since
// team-managed and next-gen projects model epic membership as the standard
// parent link, a plain issue edit is the correct move — there is no separate
// "add to epic" endpoint to call.
func (s *epicService) AddIssue(ctx context.Context, epicKey, issueKey string) (*Response, error) {
	payload := map[string]any{"fields": map[string]any{"parent": map[string]string{"key": epicKey}}}
	req, err := s.client.NewRequest(ctx, http.MethodPut, RESTPath("issue", issueKey), payload)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

// RemoveIssue detaches issueKey from its epic by clearing the parent field. A
// nil parent is the documented way to unset it; the issue itself is untouched.
func (s *epicService) RemoveIssue(ctx context.Context, issueKey string) (*Response, error) {
	payload := map[string]any{"fields": map[string]any{"parent": nil}}
	req, err := s.client.NewRequest(ctx, http.MethodPut, RESTPath("issue", issueKey), payload)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

// IssuesInEpic returns the epic's children via a parent = <epic> search. The
// key is run through jql.Value so a hostile or unusual key cannot break out of
// the query.
func (s *epicService) IssuesInEpic(ctx context.Context, epicKey string) ([]*Issue, *Response, error) {
	return NewIssueService(s.client).List(ctx, &IssueListOptions{JQL: "parent = " + jql.Value(epicKey)})
}

// StatusCounts tallies issues into the three progress buckets an epic summary
// shows (To Do, In Progress, Done). The three keys are always present so a
// caller can render a stable set even for an empty epic; issues whose status
// falls outside them are counted under their own status name.
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
