package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
)

func TestJiraRichTextTypesAreADFNative(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1}
	issue := jira.Issue{Fields: &jira.IssueFields{Description: &doc}}
	if issue.Fields.Description.Type != "doc" {
		t.Fatalf("description = %+v", issue.Fields.Description)
	}

	commentReq := jira.CommentAddRequest{Body: doc}
	if commentReq.Body.Type != "doc" {
		t.Fatalf("comment request body = %+v", commentReq.Body)
	}

	worklogReq := jira.WorklogAddRequest{TimeSpentSeconds: 60, Comment: &doc}
	if worklogReq.Comment.Type != "doc" {
		t.Fatalf("worklog request comment = %+v", worklogReq.Comment)
	}

	comment := jira.Comment{Body: &doc}
	if comment.Body.Type != "doc" {
		t.Fatalf("comment body = %+v", comment.Body)
	}

	worklog := jira.Worklog{Comment: &doc}
	if worklog.Comment.Type != "doc" {
		t.Fatalf("worklog comment = %+v", worklog.Comment)
	}
}
