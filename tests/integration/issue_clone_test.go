package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

// fixtureIssue is the raw JSON Jira returns for GET /rest/api/3/issue/JCT-1.
// It includes server-assigned/lifecycle fields that must NOT appear in the POST.
const fixtureIssue = `{
	"id":   "10001",
	"key":  "JCT-1",
	"self": "https://example.atlassian.net/rest/api/3/issue/10001",
	"fields": {
		"summary":     " source issue",
		"description": null,
		"issuetype":   {"id": "10002", "name": "Task", "subtask": false},
		"project":     {"id": "10000", "key": "JCT", "name": "Kanban Project"},
		"priority":    {"id": "3", "name": "Medium"},
		"assignee":    {"accountId": "user-abc", "displayName": "Alice"},
		"status":      {"name": "To Do"},
		"resolution":  null,
		"resolutiondate": null,
		"created":     "2026-01-01T00:00:00.000+0000",
		"updated":     "2026-01-02T00:00:00.000+0000",
		"creator":     {"accountId": "creator-xyz"},
		"reporter":    {"accountId": "reporter-xyz"},
		"comment":     {"comments": [], "total": 0},
		"worklog":     {"worklogs": [], "total": 0},
		"subtasks":    [],
		"attachment":  [],
		"votes":       {"votes": 0},
		"watches":     {"watchCount": 1},
		"issuelinks":  [],
		"aggregatetimespent": null,
		"aggregatetimeestimate": null,
		"statusCategory": {"id": 2, "name": "To Do"},
		"statuscategorychangedate": "2026-01-01T00:00:00.000+0000",
		"lastViewed":  "2026-01-03T00:00:00.000+0000",
		"issuerestriction": {"issuerestrictions": {}, "shouldDisplay": true},
		"timeestimate": null,
		"timespent": null,
		"timeoriginalestimate": null,
		"workratio": -1,
		"progress":  {"progress": 0, "total": 0},
		"timetracking": {},
		"rankBeforeIssue": null,
		"rankAfterIssue": null,
		"customfield_10016": 5,
		"customfield_10019": "0|i0003z:"
	}
}`

// TestClone_GetThenPostWithSanitizedFields verifies that Clone:
//   - GETs the source issue
//   - POSTs a new issue with sanitized fields (no id/key/self/status/created/
//     updated/creator/reporter/resolution/resolutiondate/comment/worklog/
//     subtasks/attachment/votes/watches/issuelinks/aggregate*)
//   - Collapses project and issuetype to their POST-shape
//   - Returns the new issue key
func TestClone_GetThenPostWithSanitizedFields(t *testing.T) {
	var capturedPostBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/JCT-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fixtureIssue))

		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedPostBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"10002","key":"JCT-2","self":"https://example.atlassian.net/rest/api/3/issue/10002"}`))

		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	svc := jira.NewIssueService(client)

	issue, _, err := svc.Clone(context.Background(), "JCT-1", &jira.IssueCloneRequest{})
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if issue == nil || issue.Key == nil || *issue.Key != "JCT-2" {
		t.Fatalf("Clone() key = %v, want JCT-2", issue)
	}
	if capturedPostBody == nil {
		t.Fatal("Clone() did not POST to /rest/api/3/issue")
	}

	fields, _ := capturedPostBody["fields"].(map[string]any)
	if fields == nil {
		t.Fatal("POST body missing 'fields' object")
	}

	// Must-not-appear: server-assigned / lifecycle / computed / collections.
	// Field names below are the literal Jira-shaped keys observed against
	// real Jira Cloud — Jira refuses POST with "not on the appropriate
	// screen" for any of them.
	banned := []string{
		"id", "key", "self",
		"created", "updated", "creator", "reporter",
		"status", "resolution", "resolutiondate",
		"statusCategory", "statuscategorychangedate",
		"lastViewed", "issuerestriction",
		"timeestimate", "timespent", "timeoriginalestimate",
		"workratio", "progress", "timetracking",
		"rankBeforeIssue", "rankAfterIssue",
		"comment", "worklog", "worklogs", "subtasks", "attachment",
		"votes", "watches", "issuelinks",
		"aggregatetimespent", "aggregatetimeestimate",
	}
	for _, b := range banned {
		if _, present := fields[b]; present {
			t.Errorf("POST fields must NOT contain %q but it does", b)
		}
	}

	// Must-appear: summary (carried from source).
	if fields["summary"] != " source issue" {
		t.Errorf("POST fields[summary] = %v, want %q", fields["summary"], " source issue")
	}

	// project must be collapsed to {key: JCT}.
	proj, _ := fields["project"].(map[string]any)
	if proj == nil || proj["key"] != "JCT" {
		t.Errorf("POST fields[project] = %v, want {key:JCT}", fields["project"])
	}
	if _, hasID := proj["id"]; hasID {
		t.Errorf("POST fields[project] should only have 'key', not 'id'")
	}

	// issuetype must be collapsed to {name: Task}.
	it, _ := fields["issuetype"].(map[string]any)
	if it == nil || it["name"] != "Task" {
		t.Errorf("POST fields[issuetype] = %v, want {name:Task}", fields["issuetype"])
	}

	// customfield_10016 must be carried through.
	cf, _ := fields["customfield_10016"].(float64)
	if cf != 5 {
		t.Errorf("POST fields[customfield_10016] = %v, want 5", fields["customfield_10016"])
	}

	// customfield_10019 (lexorank-shaped) must NOT be carried — Jira
	// rejects it on POST asking for the rankBeforeIssue Object form.
	if _, present := fields["customfield_10019"]; present {
		t.Errorf("POST fields[customfield_10019] = %v, want absent (lexorank token)", fields["customfield_10019"])
	}
}

// TestClone_RequestFieldsOverrideSourceFields verifies that fields in
// IssueCloneRequest.Fields win over the values carried from the source issue.
func TestClone_RequestFieldsOverrideSourceFields(t *testing.T) {
	var capturedPostBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/JCT-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fixtureIssue))

		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedPostBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"10003","key":"JCT-3"}`))

		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	svc := jira.NewIssueService(client)

	overrides := map[string]any{"summary": "overridden summary"}
	issue, _, err := svc.Clone(context.Background(), "JCT-1", &jira.IssueCloneRequest{Fields: overrides})
	if err != nil {
		t.Fatalf("Clone() with overrides error = %v", err)
	}
	if issue == nil || issue.Key == nil || *issue.Key != "JCT-3" {
		t.Fatalf("Clone() key = %v, want JCT-3", issue)
	}

	fields, _ := capturedPostBody["fields"].(map[string]any)
	if fields == nil {
		t.Fatal("POST body missing 'fields' object")
	}
	if fields["summary"] != "overridden summary" {
		t.Errorf("POST fields[summary] = %v, want %q", fields["summary"], "overridden summary")
	}
}

// TestClone_DryRunSkipsPost verifies that Clone with DryRun=true returns a
// synthetic DRY-RUN key and does NOT POST to /rest/api/3/issue.
func TestClone_DryRunSkipsPost(t *testing.T) {
	posted := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/JCT-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fixtureIssue))

		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue":
			posted = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"key":"JCT-2"}`))

		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	svc := jira.NewIssueService(client)

	issue, _, err := svc.Clone(context.Background(), "JCT-1", &jira.IssueCloneRequest{DryRun: true})
	if err != nil {
		t.Fatalf("Clone(dry-run) error = %v", err)
	}
	if posted {
		t.Error("Clone(dry-run) must NOT POST to /rest/api/3/issue")
	}
	if issue == nil || issue.Key == nil || *issue.Key != "DRY-RUN" {
		t.Errorf("Clone(dry-run) key = %v, want DRY-RUN", issue)
	}
}
