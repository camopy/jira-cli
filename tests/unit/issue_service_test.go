package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/jql"
)

func TestIssueServiceListGetPaginationAndRateLimit(t *testing.T) {
	var requests []struct {
		method string
		path   string
		body   map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := struct {
			method string
			path   string
			body   map[string]any
		}{method: r.Method, path: r.URL.String()}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req.body)
		}
		requests = append(requests, req)
		if len(requests) == 1 {
			w.Header().Set("X-RateLimit-Remaining", "3")
			_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"issues":[{"key":"PROJ-1","fields":{"summary":"hello"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"hello"}}`))
	}))
	defer srv.Close()

	service := jira.NewIssueService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	issues, resp, err := service.List(context.Background(), &jira.IssueListOptions{ListOptions: jira.ListOptions{MaxResults: 50}})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(issues) != 1 || *issues[0].Key != "PROJ-1" || resp.Rate.Remaining != 3 {
		t.Fatalf("List() = issues %+v resp %+v", issues, resp)
	}
	if requests[0].method != http.MethodPost || requests[0].path != "/rest/api/3/search/jql" {
		t.Fatalf("List() used removed or unexpected search endpoint: %s %s", requests[0].method, requests[0].path)
	}
	if got := requests[0].body["maxResults"]; got != float64(50) {
		t.Fatalf("List() maxResults body = %#v", requests[0].body)
	}
	if got := requests[0].body["jql"]; got != jql.DefaultIssueListJQL {
		t.Fatalf("List() default jql = %#v, want %q", got, jql.DefaultIssueListJQL)
	}
	fields, ok := requests[0].body["fields"].([]any)
	if !ok {
		t.Fatalf("List() did not request issue list fields: %#v", requests[0].body)
	}
	for _, want := range []string{"key", "summary", "status", "assignee", "priority", "updated"} {
		if !containsAnyString(fields, want) {
			t.Fatalf("List() fields missing %q: %#v", want, fields)
		}
	}
	issue, _, err := service.Get(context.Background(), "PROJ-1", nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if *issue.Fields.Summary != "hello" {
		t.Fatalf("summary = %q", *issue.Fields.Summary)
	}
}

func containsAnyString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
