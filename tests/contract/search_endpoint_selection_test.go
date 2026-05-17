package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

// Jira Cloud search MUST use POST /rest/api/3/search/jql (cursor-based)
// and MUST NEVER request the deprecated /rest/api/3/search or
// /rest/api/2/search. A test capturing the actual HTTP path the client
// sends is the only honest guard against drift.
func TestSearchUsesCloudJQLEndpoint(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
	}))
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL))
	svc := jira.NewSearchService(client)
	if _, _, err := svc.JQL(context.Background(), &jira.SearchRequest{JQL: "project = X"}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(seen) == 0 {
		t.Fatal("no requests recorded")
	}
	for _, req := range seen {
		if !strings.Contains(req, "/rest/api/3/search/jql") {
			t.Errorf("search request used unexpected endpoint: %q", req)
		}
		if strings.Contains(req, "/rest/api/3/search") && !strings.Contains(req, "/search/jql") {
			t.Errorf("search hit the DEPRECATED /rest/api/3/search endpoint: %q", req)
		}
		if strings.Contains(req, "/rest/api/2/search") {
			t.Errorf("search hit the DEPRECATED /rest/api/2/search endpoint: %q", req)
		}
	}
}
