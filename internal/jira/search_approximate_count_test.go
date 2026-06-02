package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// ApproximateCount POSTs the JQL to /search/approximate-count and returns the
// estimate, without fetching any issue page. It sends exactly {"jql": ...} and
// reads back {"count": N}.
func TestSearchServiceApproximateCount(t *testing.T) {
	var gotBody map[string]any
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rest/api/3/search/approximate-count" {
			t.Fatalf("path = %s, want /rest/api/3/search/approximate-count", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("request body is not JSON: %v (%s)", err, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":4242}`))
	}))

	svc := NewSearchService(client)
	count, _, err := svc.ApproximateCount(context.Background(), "project = ENG AND statusCategory != Done")
	if err != nil {
		t.Fatalf("ApproximateCount: %v", err)
	}
	if count != 4242 {
		t.Fatalf("count = %d, want 4242", count)
	}
	if gotBody["jql"] != "project = ENG AND statusCategory != Done" {
		t.Fatalf("request jql = %v, want the query", gotBody["jql"])
	}
	// Only jql is sent — the endpoint takes nothing else.
	if len(gotBody) != 1 {
		t.Fatalf("request body = %v, want only {jql}", gotBody)
	}
}

// An empty query is rejected locally before any request reaches Jira.
func TestSearchServiceApproximateCountRejectsEmptyJQL(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("approximate-count must not call Jira for an empty query")
	}))
	svc := NewSearchService(client)
	if _, _, err := svc.ApproximateCount(context.Background(), "   "); err == nil || !strings.Contains(err.Error(), "jql is required") {
		t.Fatalf("err = %v, want 'jql is required'", err)
	}
}
