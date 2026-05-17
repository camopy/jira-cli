package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/pkg/adf"
	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestIssueServiceAddCommentSubmitsADF(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/PROJ-1/comment" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"10001","body":{"type":"doc","version":1,"content":[]}}`))
	}))
	defer srv.Close()

	doc, _, err := adf.FromMarkdownLossy("hello **world**")
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}
	service := jira.NewIssueService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	comment, _, err := service.AddComment(context.Background(), "PROJ-1", &jira.CommentAddRequest{Body: doc})
	if err != nil {
		t.Fatalf("AddComment() error = %v", err)
	}
	if comment.ID == nil || *comment.ID != "10001" {
		t.Fatalf("comment = %+v", comment)
	}
	encoded, _ := json.Marshal(body["body"])
	if !strings.Contains(string(encoded), `"type":"doc"`) || !strings.Contains(string(encoded), "world") {
		t.Fatalf("body was not ADF markdown payload: %s", encoded)
	}
}

func TestCommentServiceListAllExactMaxResultsOnLastPageNotTruncated(t *testing.T) {
	var starts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startAt := r.URL.Query().Get("startAt")
		starts = append(starts, startAt)
		switch startAt {
		case "":
			_, _ = w.Write([]byte(`{"comments":[{"id":"100","body":{"type":"doc","version":1,"content":[]}}],"startAt":0,"maxResults":1,"total":2,"isLast":false}`))
		case "1":
			_, _ = w.Write([]byte(`{"comments":[{"id":"101","body":{"type":"doc","version":1,"content":[]}}],"startAt":1,"maxResults":1,"total":2,"isLast":true}`))
		default:
			t.Fatalf("unexpected startAt %q", startAt)
		}
	}))
	defer srv.Close()

	service := jira.NewCommentService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	result, err := service.ListAll(context.Background(), "PROJ-1", jira.CommentDrainOptions{PageSize: 1, MaxResults: 2})
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(result.Comments) != 2 {
		t.Fatalf("comments len = %d, want 2", len(result.Comments))
	}
	if result.Truncated {
		t.Fatalf("Truncated = true with exact final-page max-results; reason=%q", result.TruncatedReason)
	}
}

func TestCommentServiceListAllExactMaxResultsWithMorePagesIsTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startAt := r.URL.Query().Get("startAt")
		if startAt == "" {
			startAt = "0"
		}
		_, _ = w.Write([]byte(`{"comments":[{"id":"` + startAt + `","body":{"type":"doc","version":1,"content":[]}}],"startAt":` + startAt + `,"maxResults":1,"total":5,"isLast":false}`))
	}))
	defer srv.Close()

	service := jira.NewCommentService(jira.NewClient(jira.WithBaseURL(srv.URL + "/")))
	result, err := service.ListAll(context.Background(), "PROJ-1", jira.CommentDrainOptions{PageSize: 1, MaxResults: 2})
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(result.Comments) != 2 {
		t.Fatalf("comments len = %d, want 2", len(result.Comments))
	}
	if !result.Truncated || result.TruncatedReason != "max_results" {
		t.Fatalf("truncation = (%v, %q), want max_results", result.Truncated, result.TruncatedReason)
	}
}
