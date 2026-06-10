package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// JQLService wraps Jira Cloud's /jql/* endpoints (parse/validate and
// autocomplete reference data).
type JQLService interface {
	Parse(context.Context, []string, string) ([]ParsedQuery, *Response, error)
	AutocompleteData(context.Context) (JQLReference, *Response, error)
	AutocompleteSuggestions(ctx context.Context, fieldName, fieldValue string) ([]JQLSuggestion, *Response, error)
}

type jqlService struct {
	client *Client
}

func NewJQLService(client *Client) JQLService {
	return &jqlService{client: client}
}

// JQLReference is the instance's JQL metadata from /jql/autocompletedata: the
// fields and functions queryable on this Jira (including custom fields) plus
// the JQL reserved words.
type JQLReference struct {
	Fields        []JQLField
	Functions     []JQLFunction
	ReservedWords []string
}

// JQLField is one queryable field. CustomFieldID is set when the field is a
// custom field, and holds Jira's JQL custom-field token (e.g. cf[10010], the
// same form as Value) — not the customfield_NNNNN REST selector. Its real use
// is as a discriminator: present means custom, absent means a system field.
// Operators are the JQL operators the field supports; Auto reports that the
// field's values can be fetched from the suggestions endpoint.
type JQLField struct {
	Value         string
	DisplayName   string
	CustomFieldID string
	Operators     []string
	Auto          bool
}

// JQLFunction is one JQL function (e.g. currentUser()).
type JQLFunction struct {
	Value       string
	DisplayName string
}

type jqlReferenceWire struct {
	VisibleFieldNames []struct {
		Value       string   `json:"value"`
		DisplayName string   `json:"displayName"`
		Cfid        string   `json:"cfid"`
		Operators   []string `json:"operators"`
		Auto        string   `json:"auto"`
	} `json:"visibleFieldNames"`
	VisibleFunctionNames []struct {
		Value       string `json:"value"`
		DisplayName string `json:"displayName"`
	} `json:"visibleFunctionNames"`
	JqlReservedWords []string `json:"jqlReservedWords"`
}

// AutocompleteData fetches the instance's JQL reference data via
// GET /jql/autocompletedata: every visible field (including custom fields),
// function, and reserved word. Read-only; accessible anonymously on Jira's
// side, though the CLI still needs a configured profile for the base URL.
func (s *jqlService) AutocompleteData(ctx context.Context) (JQLReference, *Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, RESTPath("jql", "autocompletedata"), nil)
	if err != nil {
		return JQLReference{}, nil, err
	}
	var wire jqlReferenceWire
	resp, err := s.client.Do(req, &wire)
	if err != nil {
		return JQLReference{}, resp, err
	}
	ref := JQLReference{ReservedWords: wire.JqlReservedWords}
	for _, f := range wire.VisibleFieldNames {
		ref.Fields = append(ref.Fields, JQLField{
			Value: f.Value, DisplayName: f.DisplayName, CustomFieldID: f.Cfid,
			Operators: f.Operators, Auto: f.Auto == "true",
		})
	}
	for _, fn := range wire.VisibleFunctionNames {
		ref.Functions = append(ref.Functions, JQLFunction{Value: fn.Value, DisplayName: fn.DisplayName})
	}
	return ref, resp, nil
}

// JQLSuggestion is one autocomplete value for a field (e.g. a status name for
// `status =`). Value is what belongs in the query; DisplayName may carry
// markup from Jira and is for display only.
type JQLSuggestion struct {
	Value       string `json:"value"`
	DisplayName string `json:"displayName"`
}

type suggestionsResponse struct {
	Results []JQLSuggestion `json:"results"`
}

// AutocompleteSuggestions fetches live value suggestions for one field via
// GET /jql/autocompletedata/suggestions — the API behind the Jira web UI's
// JQL bar (e.g. fieldName=status returns the instance's status names,
// narrowed by fieldValue as a prefix). Only fields whose reference entry has
// Auto=true support it. Read-only.
func (s *jqlService) AutocompleteSuggestions(ctx context.Context, fieldName, fieldValue string) ([]JQLSuggestion, *Response, error) {
	if fieldName == "" {
		return nil, nil, errors.New("fieldName is required")
	}
	q := map[string][]string{"fieldName": {fieldName}}
	if fieldValue != "" {
		q["fieldValue"] = []string{fieldValue}
	}
	path := withQuery(RESTPath("jql", "autocompletedata", "suggestions"), q)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var result suggestionsResponse
	resp, err := s.client.Do(req, &result)
	return result.Results, resp, err
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
