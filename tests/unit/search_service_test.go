package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
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

func TestSearchServiceDefaultFieldsAreMinimal(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
	}))
	defer srv.Close()

	service := jira.NewSearchService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	if _, _, err := service.JQL(context.Background(), &jira.SearchRequest{JQL: "project=PROJ"}); err != nil {
		t.Fatalf("JQL() error = %v", err)
	}
	if _, ok := body["fields"]; ok {
		t.Fatalf("default search payload included fields = %#v; want Jira default id-only shape", body["fields"])
	}
}

func TestSearchServiceUsesExplicitFields(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
	}))
	defer srv.Close()

	service := jira.NewSearchService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	if _, _, err := service.JQL(context.Background(), &jira.SearchRequest{JQL: "project=PROJ", Fields: []string{"key", "summary"}}); err != nil {
		t.Fatalf("JQL() error = %v", err)
	}
	fields, ok := body["fields"].([]any)
	if !ok || len(fields) != 2 || fields[0] != "key" || fields[1] != "summary" {
		t.Fatalf("fields = %#v, want [key summary]", body["fields"])
	}
}

func TestSearchServiceUsesExplicitExpand(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
	}))
	defer srv.Close()

	service := jira.NewSearchService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	if _, _, err := service.JQL(context.Background(), &jira.SearchRequest{
		JQL:    "project=PROJ",
		Expand: []string{"renderedFields", "names", "schema"},
	}); err != nil {
		t.Fatalf("JQL() error = %v", err)
	}
	if got := body["expand"]; got != "renderedFields,names,schema" {
		t.Fatalf("expand = %#v, want renderedFields,names,schema", got)
	}
}

func TestSearchServiceIgnoresOffsetPaginationFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issues":[],"startAt":25,"maxResults":25,"total":100,"isLast":false}`))
	}))
	defer srv.Close()

	service := jira.NewSearchService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	_, resp, err := service.JQL(context.Background(), &jira.SearchRequest{JQL: "project=PROJ"})
	if err != nil {
		t.Fatalf("JQL() error = %v", err)
	}
	if resp.StartAt != 0 || resp.Total != 0 {
		t.Fatalf("offset pagination metadata = startAt:%d total:%d, want zero for token search", resp.StartAt, resp.Total)
	}
	if resp.MaxResults != 50 {
		t.Fatalf("MaxResults = %d, want Jira default page size 50", resp.MaxResults)
	}
	if cursor := resp.NextCursor(); cursor != "" {
		t.Fatalf("NextCursor() = %q, want empty without nextPageToken", cursor)
	}
}
