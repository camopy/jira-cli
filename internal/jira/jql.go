package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// JQLService wraps Jira Cloud's /jql/* endpoints (currently parse/validate).
type JQLService interface {
	Parse(context.Context, []string, string) ([]ParsedQuery, *Response, error)
}

type jqlService struct {
	client *Client
}

func NewJQLService(client *Client) JQLService {
	return &jqlService{client: client}
}

// ParsedQuery is the per-query result of a parse/validate call: the query as
// Jira read it, any parse errors, any warnings, and the parsed structure (kept
// raw — callers that don't need it ignore it).
type ParsedQuery struct {
	Query     string          `json:"query"`
	Errors    []string        `json:"errors,omitempty"`
	Warnings  []string        `json:"warnings,omitempty"`
	Structure json.RawMessage `json:"structure,omitempty"`
}

type parseRequest struct {
	Queries []string `json:"queries"`
}

type parseResponse struct {
	Queries []ParsedQuery `json:"queries"`
}

// Parse validates each query via POST /jql/parse, returning per-query errors
// and warnings in request order. mode is the validation strictness
// (strict|warn|none); an empty mode defaults to strict. Read-only.
func (s *jqlService) Parse(ctx context.Context, queries []string, mode string) ([]ParsedQuery, *Response, error) {
	if len(queries) == 0 {
		return nil, nil, errors.New("at least one query is required")
	}
	if mode == "" {
		mode = "strict"
	}
	path := withQuery(RESTPath("jql", "parse"), map[string][]string{"validation": {mode}})
	req, err := s.client.NewRequest(ctx, http.MethodPost, path, parseRequest{Queries: queries})
	if err != nil {
		return nil, nil, err
	}
	var result parseResponse
	resp, err := s.client.Do(req, &result)
	return result.Queries, resp, err
}
