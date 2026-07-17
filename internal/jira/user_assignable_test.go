package jira

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// AssignableSearch must hit /user/assignable/search with BOTH the query and
// the project scoping params, and return the candidates verbatim in the order
// Jira ranked them — the create form's assignee picker renders that order.
func TestAssignableSearchScopesToProjectAndQuery(t *testing.T) {
	var gotQuery, gotProject, gotPath string
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/user/assignable/search") {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		gotProject = r.URL.Query().Get("project")
		_, _ = w.Write([]byte(`[
			{"accountId":"a-1","displayName":"Ada Lovelace"},
			{"accountId":"a-2","displayName":"Alan Turing"}
		]`))
	}))

	svc := NewUserService(client)
	users, _, err := svc.AssignableSearch(context.Background(), "a", "JCT")
	if err != nil {
		t.Fatalf("AssignableSearch: %v", err)
	}

	if !strings.HasSuffix(gotPath, "/user/assignable/search") {
		t.Fatalf("request must hit /user/assignable/search, got %q", gotPath)
	}
	if gotQuery != "a" {
		t.Fatalf("query param: want %q, got %q", "a", gotQuery)
	}
	if gotProject != "JCT" {
		t.Fatalf("project param: want %q, got %q", "JCT", gotProject)
	}

	if len(users) != 2 {
		t.Fatalf("want 2 candidates, got %d: %+v", len(users), users)
	}
	// Candidates ride back in Jira's relevance order, unfiltered.
	if users[0].AccountID == nil || *users[0].AccountID != "a-1" {
		t.Fatalf("first candidate must be a-1 (relevance order preserved), got %+v", users[0])
	}
	if users[1].AccountID == nil || *users[1].AccountID != "a-2" {
		t.Fatalf("second candidate must be a-2, got %+v", users[1])
	}
}
