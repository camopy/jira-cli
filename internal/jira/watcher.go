package jira

import (
	"context"
	"net/http"
	"net/url"
)

// WatchersResponse is the shape Atlassian returns from
// GET /rest/api/3/issue/{key}/watchers — the watcher list plus the
// caller-perspective `IsWatching` flag and instance-level `WatchCount`.
type WatchersResponse struct {
	IsWatching bool    `json:"isWatching"`
	WatchCount int     `json:"watchCount"`
	Watchers   []*User `json:"watchers"`
}

// WatcherService manages watchers on an issue.
type WatcherService interface {
	List(ctx context.Context, issueKey string) (*WatchersResponse, *Response, error)
	Add(ctx context.Context, issueKey, accountID string) (*Response, error)
	Remove(ctx context.Context, issueKey, accountID string) (*Response, error)
}

type watcherService struct {
	client *Client
}

// NewWatcherService constructs a WatcherService bound to the given client.
func NewWatcherService(client *Client) WatcherService {
	return &watcherService{client: client}
}

// List returns the issue's watcher state. Atlassian only surfaces watcher
// emails when the caller has both the necessary token scope (read:user:jira
// granular / read:jira-user classic) and the watched user hasn't restricted
// their privacy — the User.EmailAddress field is nilable here for that
// reason.
func (s *watcherService) List(ctx context.Context, issueKey string) (*WatchersResponse, *Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, RESTPath("issue", issueKey, "watchers"), nil)
	if err != nil {
		return nil, nil, err
	}
	var out WatchersResponse
	resp, err := s.client.Do(req, &out)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// Add registers `accountID` as a watcher on `issueKey`.
//
// Atlassian's POST /watchers expects a *raw JSON string* body
// (`"<accountId>"`), not a JSON object — http-contract.md captures this
// quirk. Sending an object yields 400 with "expected JSON string". Passing
// the account ID string directly to Client.NewRequest preserves that wire
// shape because the JSON encoder marshals it as a string literal.
func (s *watcherService) Add(ctx context.Context, issueKey, accountID string) (*Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodPost, RESTPath("issue", issueKey, "watchers"), accountID)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

// Remove deregisters `accountID` from the issue's watcher list.
// Atlassian's DELETE expects the accountId in the query string, not
// the body — request shape mirrors the spec/http-contract.md quirk
// table. Idempotent server-side: removing a non-watcher returns 204.
func (s *watcherService) Remove(ctx context.Context, issueKey, accountID string) (*Response, error) {
	q := url.Values{}
	q.Set("accountId", accountID)
	req, err := s.client.NewRequest(ctx, http.MethodDelete, withQuery(RESTPath("issue", issueKey, "watchers"), q), nil)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}
