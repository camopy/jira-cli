package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Parse POSTs the queries to /jql/parse with the validation mode as a query
// param and returns the ordered per-query result: the query, its errors, and
// its warnings.
func TestJQLServiceParse(t *testing.T) {
	var gotBody map[string]any
	var gotValidation string
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rest/api/3/jql/parse" {
			t.Fatalf("path = %s, want /rest/api/3/jql/parse", r.URL.Path)
		}
		gotValidation = r.URL.Query().Get("validation")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("request body is not JSON: %v (%s)", err, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queries":[
			{"query":"project = ENG ORDER BY updated DESC","structure":{"orderBy":{}}},
			{"query":"bad =","errors":["Error in the JQL Query: expecting a value"],"warnings":[]}
		]}`))
	}))

	svc := NewJQLService(client)
	results, _, err := svc.Parse(context.Background(), []string{"project = ENG ORDER BY updated DESC", "bad ="}, "strict")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if gotValidation != "strict" {
		t.Fatalf("validation query param = %q, want strict", gotValidation)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if len(results[0].Errors) != 0 {
		t.Fatalf("results[0] should have no errors, got %v", results[0].Errors)
	}
	if len(results[1].Errors) == 0 || !strings.Contains(results[1].Errors[0], "Error in the JQL Query") {
		t.Fatalf("results[1] should carry a parse error, got %+v", results[1])
	}
	queries, _ := gotBody["queries"].([]any)
	if len(queries) != 2 {
		t.Fatalf("request queries = %v, want 2", gotBody["queries"])
	}
}

// An empty mode falls back to a default rather than sending a blank validation
// param; an empty query set is rejected before any request.
func TestJQLServiceParseDefaultsAndGuards(t *testing.T) {
	var gotValidation string
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotValidation = r.URL.Query().Get("validation")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queries":[{"query":"project = ENG"}]}`))
	}))
	svc := NewJQLService(client)
	if _, _, err := svc.Parse(context.Background(), []string{"project = ENG"}, ""); err != nil {
		t.Fatalf("Parse with empty mode: %v", err)
	}
	if gotValidation != "strict" {
		t.Fatalf("empty mode should default to strict, sent %q", gotValidation)
	}

	rejecting := newHTTPHandlerClient(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("parse must not call Jira with no queries")
	}))
	if _, _, err := NewJQLService(rejecting).Parse(context.Background(), nil, "strict"); err == nil || !strings.Contains(err.Error(), "at least one query") {
		t.Fatalf("err = %v, want 'at least one query'", err)
	}
}
