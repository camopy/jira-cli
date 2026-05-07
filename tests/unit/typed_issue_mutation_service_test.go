package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestIssueCloneAndMoveUseTypedRequests(t *testing.T) {
	var cloneBody map[string]any
	var moveBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// Clone first GETs the source issue, then POSTs the new one.
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/PROJ-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Original","issuetype":{"name":"Task"},"project":{"key":"PROJ"}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue":
			if err := json.NewDecoder(r.Body).Decode(&cloneBody); err != nil {
				t.Fatalf("decode clone body: %v", err)
			}
			_, _ = w.Write([]byte(`{"key":"PROJ-2"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/rest/api/3/issue/PROJ-1":
			if err := json.NewDecoder(r.Body).Decode(&moveBody); err != nil {
				t.Fatalf("decode move body: %v", err)
			}
			_, _ = w.Write([]byte(`{"key":"PROJ-1"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	service := jira.NewIssueService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	if _, _, err := service.Clone(context.Background(), "PROJ-1", &jira.IssueCloneRequest{Fields: map[string]any{"summary": "Clone"}}); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if _, _, err := service.Move(context.Background(), "PROJ-1", &jira.IssueMoveRequest{Fields: map[string]any{"project": map[string]string{"key": "NEXT"}}}); err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if cloneFields, ok := cloneBody["fields"].(map[string]any); !ok || cloneFields["summary"] != "Clone" {
		t.Fatalf("clone body = %+v", cloneBody)
	}
	if moveFields, ok := moveBody["fields"].(map[string]any); !ok || moveFields["project"] == nil {
		t.Fatalf("move body = %+v", moveBody)
	}
}
