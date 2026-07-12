package jira

import (
	"context"
	"errors"
	"net/http"
	"strings"

	xstrings "github.com/gechr/x/strings"
)

// SearchService runs JQL against Jira Cloud's enhanced search
// (/search/jql), the token-paged endpoint that replaced the deprecated
// offset-based /search. JQL fetches issues a page at a time; ApproximateCount
// estimates a match total without paging.
type SearchService interface {
	JQL(context.Context, *SearchRequest) ([]*Issue, *Response, error)
	ApproximateCount(context.Context, string) (int, *Response, error)
}

type searchService struct {
	client *Client
}

// SearchRequest is the input to SearchService.JQL. Fields selects which issue
// fields to return and Expand which expansions to include; both are sent as the
// endpoint expects (a JSON array and a comma-joined string respectively) by
// payload(). The embedded ListOptions is a fallback source for MaxResults /
// NextPageToken so a caller can pass paging either directly or via ListOptions
// — it carries `json:"-"` because the wire body uses the explicit fields.
type SearchRequest struct {
	JQL           string `json:"jql,omitempty"`
	MaxResults    int    `json:"maxResults,omitempty"`
	NextPageToken string `json:"nextPageToken,omitempty"`
	Fields        []string
	Expand        []string
	ListOptions   `json:"-"`
}

type searchRequestPayload struct {
	JQL           string   `json:"jql,omitempty"`
	MaxResults    int      `json:"maxResults,omitempty"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
	Fields        []string `json:"fields,omitempty"`
	Expand        string   `json:"expand,omitempty"`
}

// NewSearchService constructs a SearchService bound to the given client.
func NewSearchService(client *Client) SearchService {
	return &searchService{client: client}
}

// JQL fetches one page of issues matching the request's query. It POSTs to
// /search/jql — a read that uses POST so a large JQL body fits (the client's
// mutation guard whitelists this path). The returned Response is stamped with
// the token-paging state (TokenPage, NextPageToken, IsLast) so DrainSearch and
// the envelope layer can page without knowing the endpoint's shape.
func (s *searchService) JQL(ctx context.Context, reqBody *SearchRequest) ([]*Issue, *Response, error) {
	if reqBody == nil || reqBody.JQL == "" {
		return nil, nil, errors.New("jql is required")
	}
	req, err := s.client.NewRequest(ctx, http.MethodPost, RESTPath("search", "jql"), reqBody.payload())
	if err != nil {
		return nil, nil, err
	}
	var result SearchResult
	resp, err := s.client.Do(req, &result)
	if resp != nil {
		resp.MaxResults = reqBody.effectivePageSize()
		resp.IsLast = result.IsLast
		resp.NextPageToken = result.NextPageToken
		resp.TokenPage = true
	}
	return result.Issues, resp, err
}

type approximateCountPayload struct {
	JQL string `json:"jql"`
}

type approximateCountResult struct {
	Count int `json:"count"`
}

// ApproximateCount returns Jira's estimated number of issues matching jql via
// POST /search/approximate-count, without fetching any issue page. The count
// is an estimate with no error bound, and the endpoint ignores any ORDER BY.
func (s *searchService) ApproximateCount(ctx context.Context, jql string) (int, *Response, error) {
	if xstrings.IsBlank(jql) {
		return 0, nil, errors.New("jql is required")
	}
	req, err := s.client.NewRequest(ctx, http.MethodPost, RESTPath("search", "approximate-count"), approximateCountPayload{JQL: jql})
	if err != nil {
		return 0, nil, err
	}
	var result approximateCountResult
	resp, err := s.client.Do(req, &result)
	return result.Count, resp, err
}

func (r *SearchRequest) payload() searchRequestPayload {
	fields := compactSearchFields(r.Fields)
	return searchRequestPayload{
		JQL:           r.JQL,
		MaxResults:    r.effectiveMaxResults(),
		NextPageToken: r.effectiveNextPageToken(),
		Fields:        fields,
		Expand:        strings.Join(compactSearchFields(r.Expand), ","),
	}
}

func compactSearchFields(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			out = append(out, field)
		}
	}
	return out
}

func (r *SearchRequest) effectiveMaxResults() int {
	if r == nil {
		return 0
	}
	if r.MaxResults > 0 {
		return r.MaxResults
	}
	return r.ListOptions.MaxResults
}

func (r *SearchRequest) effectivePageSize() int {
	if pageSize := r.effectiveMaxResults(); pageSize > 0 {
		return pageSize
	}
	return 50
}

func (r *SearchRequest) effectiveNextPageToken() string {
	if r == nil {
		return ""
	}
	if r.NextPageToken != "" {
		return r.NextPageToken
	}
	return r.ListOptions.NextPageToken
}
