package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestJiraServicesAgainstHTTPTestServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/search/jql":
			if r.Method != http.MethodPost {
				t.Fatalf("search method = %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-1"}],"isLast":true}`))
		case "/rest/api/3/issue/PROJ-1":
			_, _ = w.Write([]byte(`{"key":"PROJ-1"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	issueService := jira.NewIssueService(client)
	if _, _, err := issueService.List(context.Background(), nil); err != nil {
		t.Fatalf("IssueService.List() error = %v", err)
	}
	if _, _, err := issueService.Get(context.Background(), "PROJ-1", nil); err != nil {
		t.Fatalf("IssueService.Get() error = %v", err)
	}
}
