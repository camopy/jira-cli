package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestSearchServiceJQLPaginationAndValidation(t *testing.T) {
	var method, path string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.String()
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-1"}],"isLast":false,"nextPageToken":"next-token"}`))
	}))
	defer srv.Close()
	service := jira.NewSearchService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	issues, resp, err := service.JQL(context.Background(), &jira.SearchRequest{JQL: "project=PROJ", ListOptions: jira.ListOptions{MaxResults: 25}})
	if err != nil || len(issues) != 1 {
		t.Fatalf("JQL() = %+v err=%v", issues, err)
	}
	if method != http.MethodPost || path != "/rest/api/3/search/jql" {
		t.Fatalf("JQL() used removed or unexpected search endpoint: %s %s", method, path)
	}
	if body["jql"] != "project=PROJ" || body["maxResults"] != float64(25) {
		t.Fatalf("JQL() body = %#v", body)
	}
	if resp.NextCursor() != "next-token" {
		t.Fatalf("JQL() next cursor = %q", resp.NextCursor())
	}
	if _, _, err := service.JQL(context.Background(), &jira.SearchRequest{}); err == nil {
		t.Fatal("JQL() error = nil for empty query")
	}
}
