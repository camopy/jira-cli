package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// LabelService surfaces Jira's global label list. Labels in Jira are not
// scoped to a project — they're a global string pool — so callers
// generally fetch once per profile and reuse via the local cache.
type LabelService interface {
	List(context.Context, *ListOptions) ([]string, *Response, error)
}

type labelService struct {
	client *Client
}

// NewLabelService constructs a LabelService bound to the given client.
func NewLabelService(client *Client) LabelService {
	return &labelService{client: client}
}

// labelPage is the Jira /rest/api/3/label envelope (alphabetically sorted).
type labelPage struct {
	MaxResults int      `json:"maxResults"`
	StartAt    int      `json:"startAt"`
	Total      int      `json:"total"`
	IsLast     bool     `json:"isLast"`
	Values     []string `json:"values"`
}

// List walks /rest/api/3/label until isLast=true and returns the merged
// alphabetical label list. Caller can scope the page size with opts.MaxResults
// (defaults to 1000, the Jira maximum).
func (s *labelService) List(ctx context.Context, opts *ListOptions) ([]string, *Response, error) {
	page := 1000
	startAt := 0
	if opts != nil {
		if opts.MaxResults > 0 {
			page = opts.MaxResults
		}
		if opts.StartAt > 0 {
			startAt = opts.StartAt
		}
	}
	var labels []string
	var lastResp *Response
	pages := 0
	for {
		q := url.Values{}
		q.Set("startAt", strconv.Itoa(startAt))
		q.Set("maxResults", strconv.Itoa(page))
		path := withQuery(RESTPath("label"), q)
		req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, nil, err
		}
		var p labelPage
		resp, err := s.client.Do(req, &p)
		if err != nil {
			return nil, resp, err
		}
		pages++
		labels = append(labels, p.Values...)
		lastResp = resp
		if p.IsLast {
			break
		}
		if pages >= defaultMaxPages || len(labels) >= defaultMaxResults {
			return nil, lastResp, fmt.Errorf("label pagination exceeded default bounds")
		}
		startAt = nextOffset(startAt, len(p.Values), page, p.MaxResults)
	}
	return labels, lastResp, nil
}
