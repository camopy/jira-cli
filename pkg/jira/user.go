package jira

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ErrUserNotFound is returned by ResolveUser when /user/search yields zero
// matches for the supplied query. The CLI surface maps this to exit 2 per
// the constitution error contract (resource-not-found).
var ErrUserNotFound = errors.New("user not found")

// AmbiguousUserError is returned by ResolveUser when /user/search yields
// 2+ matches and we refuse to pick a winner. Candidates carries every
// returned User so the CLI can render the disambiguation envelope.
type AmbiguousUserError struct {
	Query      string
	Candidates []*User
}

func (e *AmbiguousUserError) Error() string {
	return fmt.Sprintf("ambiguous user %q — %d candidates", e.Query, len(e.Candidates))
}

// User identity returned by /rest/api/3/myself. Subset of the full response —
// only the fields we use for assign-to-me, profile bootstrap and `whoami`.
type CurrentUser struct {
	Self         string     `json:"self,omitempty"`
	AccountID    string     `json:"accountId,omitempty"`
	AccountType  string     `json:"accountType,omitempty"`
	EmailAddress string     `json:"emailAddress,omitempty"`
	DisplayName  string     `json:"displayName,omitempty"`
	Active       bool       `json:"active,omitempty"`
	TimeZone     string     `json:"timeZone,omitempty"`
	Locale       string     `json:"locale,omitempty"`
	AvatarURLs   AvatarURLs `json:"avatarUrls,omitempty"`
}

// AvatarURLs maps Jira's standard avatar size keys to URLs.
type AvatarURLs struct {
	Size16 string `json:"16x16,omitempty"`
	Size24 string `json:"24x24,omitempty"`
	Size32 string `json:"32x32,omitempty"`
	Size48 string `json:"48x48,omitempty"`
}

// PermissionEntry is a single result of /rest/api/3/mypermissions.
type PermissionEntry struct {
	ID             string `json:"id,omitempty"`
	Key            string `json:"key,omitempty"`
	Name           string `json:"name,omitempty"`
	Type           string `json:"type,omitempty"`
	Description    string `json:"description,omitempty"`
	HavePermission bool   `json:"havePermission"`
}

// PermissionsResponse wraps the /mypermissions envelope.
type PermissionsResponse struct {
	Permissions map[string]PermissionEntry `json:"permissions"`
}

// UserService exposes user-identity endpoints.
type UserService interface {
	Myself(context.Context) (*CurrentUser, *Response, error)
	MyPermissions(ctx context.Context, projectKey string, keys []string) (*PermissionsResponse, *Response, error)
	Search(ctx context.Context, query string) ([]*User, *Response, error)
	ResolveUser(ctx context.Context, query string) (string, error)
}

type userService struct {
	client   *Client
	myselfMu sync.Mutex
	myselfID string
}

// NewUserService constructs a UserService bound to the given client.
func NewUserService(client *Client) UserService {
	return &userService{client: client}
}

// Myself returns the authenticated user's identity via /rest/api/3/myself.
func (s *userService) Myself(ctx context.Context) (*CurrentUser, *Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "rest/api/3/myself", nil)
	if err != nil {
		return nil, nil, err
	}
	var u CurrentUser
	resp, err := s.client.Do(req, &u)
	return &u, resp, err
}

// MyPermissions probes /rest/api/3/mypermissions for the current token.
// projectKey is optional ("" = global context). keys is the comma-joined
// list of permission keys (e.g. BROWSE_PROJECTS,CREATE_ISSUES) — required
// by the v3 endpoint, which rejects unfiltered queries.
func (s *userService) MyPermissions(ctx context.Context, projectKey string, keys []string) (*PermissionsResponse, *Response, error) {
	path := "rest/api/3/mypermissions?permissions=" + strings.Join(keys, ",")
	if projectKey != "" {
		path += "&projectKey=" + projectKey
	}
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var out PermissionsResponse
	resp, err := s.client.Do(req, &out)
	return &out, resp, err
}

// Search runs /rest/api/3/user/search?query=<q> and returns the candidate
// list ordered by Atlassian's relevance. Atlassian does NOT guarantee
// uniqueness for emails — callers needing the resolve-or-fail semantics
// should use ResolveUser instead.
func (s *userService) Search(ctx context.Context, query string) ([]*User, *Response, error) {
	q := url.Values{}
	q.Set("query", query)
	req, err := s.client.NewRequest(ctx, http.MethodGet, "rest/api/3/user/search?"+q.Encode(), nil)
	if err != nil {
		return nil, nil, err
	}
	var out []*User
	resp, err := s.client.Do(req, &out)
	return out, resp, err
}

// ResolveUser turns the user-supplied identifier into an accountId per
// the resolver contract:
//   - "me"                → cached /myself accountId
//   - "accountId:<id>"    → parsed locally; no /user/search hop
//   - anything else       → /user/search?query=…
//     0 matches  → ErrUserNotFound
//     1 match    → that accountId
//     2+ matches → *AmbiguousUserError carrying every candidate
//
// Caching the /myself response keeps repeated `--user me` invocations
// (e.g. `watch` then `unwatch` in the same process) from hitting the
// network twice.
func (s *userService) ResolveUser(ctx context.Context, query string) (string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", errors.New("user identifier is required")
	}
	if q == "me" {
		// /myself is identical for the lifetime of this Client, so cache
		// the result and re-issue rather than calling once-and-for-all.
		// We don't use sync.Once here because that would bake the first
		// caller's canceled ctx into every subsequent lookup; instead we
		// retry on the next call when the previous attempt failed.
		s.myselfMu.Lock()
		id, cached := s.myselfID, s.myselfID != ""
		s.myselfMu.Unlock()
		if cached {
			return id, nil
		}
		me, _, err := s.Myself(ctx)
		if err != nil {
			return "", err
		}
		if me.AccountID == "" {
			return "", errors.New("me: /myself returned empty accountId")
		}
		s.myselfMu.Lock()
		s.myselfID = me.AccountID
		s.myselfMu.Unlock()
		return me.AccountID, nil
	}
	if id, ok := strings.CutPrefix(q, "accountId:"); ok {
		if id == "" {
			return "", errors.New("accountId: prefix requires a non-empty id")
		}
		return id, nil
	}
	users, _, err := s.Search(ctx, q)
	if err != nil {
		return "", err
	}
	switch len(users) {
	case 0:
		return "", fmt.Errorf("%w: %q", ErrUserNotFound, q)
	case 1:
		if users[0].AccountID == nil || *users[0].AccountID == "" {
			return "", errors.New("user search match returned empty accountId")
		}
		return *users[0].AccountID, nil
	default:
		return "", &AmbiguousUserError{Query: q, Candidates: users}
	}
}
