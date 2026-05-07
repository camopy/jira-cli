package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestIssueServiceTransitionsAndSubmit(t *testing.T) {
	var posted bool
	var postedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posted = true
			if err := json.NewDecoder(r.Body).Decode(&postedBody); err != nil {
				t.Fatalf("decode transition body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"transitions":[{"id":"31","name":"Done"}]}`))
	}))
	defer srv.Close()
	service := jira.NewIssueService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	transitions, _, err := service.Transitions(context.Background(), "PROJ-1")
	if err != nil || len(transitions) != 1 {
		t.Fatalf("Transitions() = %+v err=%v", transitions, err)
	}
	if _, err := service.Transition(context.Background(), "PROJ-1", &jira.TransitionRequest{ID: "31", Fields: map[string]any{"resolution": map[string]any{"name": "Done"}}}); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	if !posted {
		t.Fatal("transition was not posted")
	}
	if postedBody["fields"] == nil {
		t.Fatalf("transition fields not posted: %+v", postedBody)
	}
}
