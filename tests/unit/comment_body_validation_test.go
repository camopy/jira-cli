package unit

import (
	"context"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Comment body must be non-empty before any HTTP call. The service-
// layer Add and Edit methods enforce this so callers (cmd/jira binding,
// future scripts) can't silently 400 on Atlassian.
//
// "Empty" means:
//   - nil body
//   - ADF Document with zero content nodes (whitespace-only or null doc)
//
// Both Add and Edit use the same CommentBody type so the rules are shared.

func TestCommentServiceAddRejectsNilBodyLocally(t *testing.T) {
	svc := jira.NewCommentService(jira.NewClient())
	_, _, err := svc.Add(context.Background(), "PROJ-1", &jira.CommentBody{})
	if err == nil {
		t.Fatal("err = nil; want validation error on nil body")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "body") {
		t.Errorf("err = %v; want a message about the missing body", err)
	}
}

func TestCommentServiceAddRejectsEmptyADFDocLocally(t *testing.T) {
	svc := jira.NewCommentService(jira.NewClient())
	empty := adf.Document{Type: "doc", Version: 1}
	_, _, err := svc.Add(context.Background(), "PROJ-1", &jira.CommentBody{ADF: &empty})
	if err == nil {
		t.Fatal("err = nil; want validation error on empty ADF doc")
	}
}

func TestCommentServiceEditRejectsNilBodyLocally(t *testing.T) {
	// Edit also requires a body — Atlassian PUT must carry one. Visibility-only
	// changes still need a body submission per the API contract; the cmd layer
	// can choose to fetch-then-PUT for a future "visibility-only edit" feature
	// but the service stays strict.
	svc := jira.NewCommentService(jira.NewClient())
	_, _, err := svc.Edit(context.Background(), "PROJ-1", "55", &jira.CommentBody{}, jira.VisibilityChange{Mode: jira.VisibilityKeep})
	if err == nil {
		t.Fatal("err = nil; want validation error on nil body")
	}
}

func TestCommentServiceEditRejectsEmptyADFDocLocally(t *testing.T) {
	svc := jira.NewCommentService(jira.NewClient())
	empty := adf.Document{Type: "doc", Version: 1}
	_, _, err := svc.Edit(context.Background(), "PROJ-1", "55", &jira.CommentBody{ADF: &empty}, jira.VisibilityChange{Mode: jira.VisibilityKeep})
	if err == nil {
		t.Fatal("err = nil; want validation error on empty ADF doc")
	}
}
