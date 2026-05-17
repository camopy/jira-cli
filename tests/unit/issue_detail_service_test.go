package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestIssueServiceGetDetailExpandsActivity(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"detail"}}`))
	}))
	defer srv.Close()
	service := jira.NewIssueService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	_, _, err := service.Get(context.Background(), "PROJ-1", &jira.IssueGetOptions{Expand: []string{"renderedFields", "changelog"}})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if query == "" {
		t.Fatal("detail get did not include expansion query")
	}
}

func TestIssueServiceGetNormalizesNativeDetailCollections(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
  "key": "PROJ-1",
  "fields": {
    "summary": "Native detail",
    "comment": {"comments": [{"id": "c1"}]},
    "worklog": {"worklogs": [{"id": "w1", "timeSpentSeconds": 60}]},
    "issuelinks": [{"outwardIssue": {"key": "PROJ-2"}}],
    "subtasks": [{"key": "PROJ-3"}],
    "customfield_10001": "Platform"
  }
}`))
	}))
	defer srv.Close()
	service := jira.NewIssueService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	issue, _, err := service.Get(context.Background(), "PROJ-1", nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(issue.Comments) != 1 || *issue.Comments[0].ID != "c1" {
		t.Fatalf("comments not normalized: %+v", issue.Comments)
	}
	if len(issue.Worklogs) != 1 || *issue.Worklogs[0].ID != "w1" {
		t.Fatalf("worklogs not normalized: %+v", issue.Worklogs)
	}
	if len(issue.LinkedIssues) != 1 || *issue.LinkedIssues[0].Key != "PROJ-2" {
		t.Fatalf("links not normalized: %+v", issue.LinkedIssues)
	}
	if len(issue.Subtasks) != 1 || *issue.Subtasks[0].Key != "PROJ-3" {
		t.Fatalf("subtasks not normalized: %+v", issue.Subtasks)
	}
	if _, ok := issue.Fields.CustomFields["customfield_10001"]; !ok {
		t.Fatalf("custom field not preserved: %+v", issue.Fields.CustomFields)
	}
}
