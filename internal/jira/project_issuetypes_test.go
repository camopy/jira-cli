package jira

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// ListIssueTypes must page the createmeta issuetypes endpoint end to end
// and return every {id, name} in encounter order: the create form's type
// picker relies on seeing types that live past the first page.
func TestListIssueTypesWalksAllPages(t *testing.T) {
	var paths []string
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/createmeta/JCT/issuetypes") {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		paths = append(paths, r.URL.Path)
		if r.URL.Query().Get("startAt") == "0" {
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1,"total":2,"issueTypes":[{"id":"10001","name":"Bug"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"startAt":1,"maxResults":1,"total":2,"issueTypes":[{"id":"10002","name":"Task"}]}`))
	}))

	svc := NewProjectService(client, 0)
	types, _, err := svc.ListIssueTypes(context.Background(), "JCT")
	if err != nil {
		t.Fatalf("ListIssueTypes: %v", err)
	}

	want := []ProjectIssueType{
		{ID: "10001", Name: "Bug"},
		{ID: "10002", Name: "Task"},
	}
	if len(types) != len(want) {
		t.Fatalf("want %d issue types, got %d: %+v", len(want), len(types), types)
	}
	for i, w := range want {
		if types[i] != w {
			t.Fatalf("issue type %d: want %+v, got %+v", i, w, types[i])
		}
	}
	// Both requests must hit the createmeta issuetypes path — the walk
	// addresses no other endpoint.
	if len(paths) != 2 {
		t.Fatalf("want 2 requests, got %d: %v", len(paths), paths)
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, "/createmeta/JCT/issuetypes") {
			t.Fatalf("request path must hit /createmeta/JCT/issuetypes, got %q", p)
		}
	}
}

// A single page that reports itself complete (startAt+len >= total) must
// end the walk without a second request.
func TestListIssueTypesSinglePage(t *testing.T) {
	requests := 0
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/createmeta/JCT/issuetypes") {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		requests++
		_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":2,"issueTypes":[{"id":"10001","name":"Bug"},{"id":"10002","name":"Task"}]}`))
	}))

	svc := NewProjectService(client, 0)
	types, _, err := svc.ListIssueTypes(context.Background(), "JCT")
	if err != nil {
		t.Fatalf("ListIssueTypes: %v", err)
	}
	if requests != 1 {
		t.Fatalf("a complete single page must not trigger a second request, got %d", requests)
	}
	want := []ProjectIssueType{
		{ID: "10001", Name: "Bug"},
		{ID: "10002", Name: "Task"},
	}
	if len(types) != len(want) {
		t.Fatalf("want %d issue types, got %d: %+v", len(want), len(types), types)
	}
	for i, w := range want {
		if types[i] != w {
			t.Fatalf("issue type %d: want %+v, got %+v", i, w, types[i])
		}
	}
}
