package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestWorklogServiceAndCommentMutation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"worklogs":[{"id":"1","timeSpentSeconds":60}]}`))
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"2","timeSpentSeconds":120}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	service := jira.NewWorklogService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	logs, _, err := service.List(context.Background(), "PROJ-1", nil)
	if err != nil || len(logs) != 1 {
		t.Fatalf("List() = %+v err=%v", logs, err)
	}
	added, _, err := service.Add(context.Background(), "PROJ-1", &jira.WorklogAddRequest{TimeSpentSeconds: 120})
	if err != nil || *added.ID != "2" {
		t.Fatalf("Add() = %+v err=%v", added, err)
	}
	if _, err := service.Delete(context.Background(), "PROJ-1", "2"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}
