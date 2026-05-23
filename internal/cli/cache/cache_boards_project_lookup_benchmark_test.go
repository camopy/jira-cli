package cache_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
)

func BenchmarkPrimeBoardsDuplicateBoardProjectCache(b *testing.B) {
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
			_, _ = w.Write([]byte(`{"maxResults":50,"isLast":true,"values":[{"key":"ENG"},{"key":"PLAT"}]}`))
		default:
			b.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := cmdutil.PrimeBoards(context.Background(), client, 60, false); err != nil {
			b.Fatal(err)
		}
	}
}
