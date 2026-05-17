package jira

import (
	"context"
	"errors"
	"net/http"
	"strings"
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
		resp.MaxResults = reqBody.effectivePageSize()
		resp.IsLast = result.IsLast
		resp.NextPageToken = result.NextPageToken
		resp.TokenPage = true
	}
	return result.Issues, resp, err
}

func (r *SearchRequest) payload() searchRequestPayload {
	fields := compactSearchFields(r.Fields)
	return searchRequestPayload{
		JQL:           r.JQL,
		MaxResults:    r.effectiveMaxResults(),
		NextPageToken: r.effectiveNextPageToken(),
		Fields:        fields,
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
