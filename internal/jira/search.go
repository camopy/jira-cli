package jira

import (
	"context"
	"errors"
	"net/http"
	"strings"

	xstrings "github.com/gechr/x/strings"
)

type SearchService interface {
	JQL(context.Context, *SearchRequest) ([]*Issue, *Response, error)
	ApproximateCount(context.Context, string) (int, *Response, error)
}

type searchService struct {
	client *Client
}

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

func NewSearchService(client *Client) SearchService {
	return &searchService{client: client}
}

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
