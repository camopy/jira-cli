package jira

import (
	"context"
	"errors"
	"net/http"
)

type SearchService interface {
	JQL(context.Context, *SearchRequest) ([]*Issue, *Response, error)
}

type searchService struct {
	client *Client
}

type SearchRequest struct {
	JQL           string `json:"jql,omitempty"`
	MaxResults    int    `json:"maxResults,omitempty"`
	NextPageToken string `json:"nextPageToken,omitempty"`
	Fields        []string
	ListOptions   `json:"-"`
}

type searchRequestPayload struct {
	JQL           string   `json:"jql,omitempty"`
	MaxResults    int      `json:"maxResults,omitempty"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
	Fields        []string `json:"fields,omitempty"`
}

func NewSearchService(client *Client) SearchService {
	return &searchService{client: client}
}

func (s *searchService) JQL(ctx context.Context, reqBody *SearchRequest) ([]*Issue, *Response, error) {
	if reqBody == nil || reqBody.JQL == "" {
		return nil, nil, errors.New("jql is required")
	}
	req, err := s.client.NewRequest(ctx, http.MethodPost, "rest/api/3/search/jql", reqBody.payload())
	if err != nil {
		return nil, nil, err
	}
	var result SearchResult
	resp, err := s.client.Do(req, &result)
	if resp != nil {
		resp.StartAt = result.StartAt
		resp.MaxResults = firstNonZero(result.MaxResults, reqBody.effectiveMaxResults())
		resp.Total = result.Total
		resp.IsLast = result.IsLast
		resp.NextPageToken = result.NextPageToken
	}
	return result.Issues, resp, err
}

func (r *SearchRequest) payload() searchRequestPayload {
	// Jira's POST /rest/api/3/search/jql endpoint returns id-only when
	// `fields` is omitted (a behavior change from the deprecated
	// /search endpoint, which defaulted to all). We default to "*all"
	// so the CLI's typed output keeps working without per-call config.
	// Callers that want a smaller payload set Fields explicitly.
	fields := r.Fields
	if len(fields) == 0 {
		fields = []string{"*all"}
	}
	return searchRequestPayload{
		JQL:           r.JQL,
		MaxResults:    r.effectiveMaxResults(),
		NextPageToken: r.effectiveNextPageToken(),
		Fields:        fields,
	}
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

func (r *SearchRequest) effectiveNextPageToken() string {
	if r == nil {
		return ""
	}
	if r.NextPageToken != "" {
		return r.NextPageToken
	}
	return r.ListOptions.NextPageToken
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
