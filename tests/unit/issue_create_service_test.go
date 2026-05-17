package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestIssueServiceCreateValidation(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode create body: %v", err)
		}
		_, _ = w.Write([]byte(`{"key":"PROJ-2"}`))
	}))
	defer srv.Close()
	service := jira.NewIssueService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	issue, _, err := service.Create(context.Background(), &jira.IssueCreateRequest{Summary: "hello"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if *issue.Key != "PROJ-2" {
		t.Fatalf("created key = %q", *issue.Key)
	}
	fields, ok := got["fields"].(map[string]any)
	if !ok {
		t.Fatalf("create body missing fields object: %+v", got)
	}
	if fields["summary"] != "hello" {
		t.Fatalf("create fields summary = %+v", fields)
	}
	if _, _, err := service.Create(context.Background(), &jira.IssueCreateRequest{}); err == nil {
		t.Fatal("Create() error = nil for missing summary")
	}
}
