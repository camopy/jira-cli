package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestPrimeBoardsDeduplicatesProjectLookupsByBoardID(t *testing.T) {
	var projectCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"maxResults": 50,
				"isLast":     true,
				"values": []map[string]any{
					{"id": 42, "name": "Engineering Sprint", "type": "scrum"},
					{"id": 42, "name": "Engineering Sprint Copy", "type": "scrum"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/rest/agile/1.0/board/") && strings.HasSuffix(r.URL.Path, "/project"):
			projectCalls.Add(1)
			_, _ = w.Write([]byte(`{"maxResults":50,"isLast":true,"values":[{"key":"ENG"},{"key":"PLAT"}]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	file, warnings, err := primeBoards(context.Background(), jira.NewClient(jira.WithBaseURL(srv.URL+"/")), 60, false)
	if err != nil {
		t.Fatalf("primeBoards() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("primeBoards() warnings = %+v, want none", warnings)
	}
	if got := projectCalls.Load(); got != 1 {
		t.Fatalf("project lookups = %d, want 1 for duplicate board id", got)
	}
	if len(file.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(file.Items))
	}
	for _, item := range file.Items {
		if strings.Join(item.ProjectKeys, ",") != "ENG,PLAT" {
			t.Fatalf("project keys = %v, want [ENG PLAT]", item.ProjectKeys)
		}
	}
}
