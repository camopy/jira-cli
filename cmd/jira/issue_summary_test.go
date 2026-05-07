package main

import (
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestIssueSummaryAlwaysIncludesSpecKeys(t *testing.T) {
	got := issueSummary(&jira.Issue{
		Key: jira.String("PROJ-1"),
		Fields: &jira.IssueFields{
			Summary: jira.String("Hello"),
			Status:  &jira.Status{Name: jira.String("In Progress")},
			Updated: jira.String("2026-05-03T10:00:00Z"),
		},
	})
	for _, key := range []string{"key", "summary", "status", "updated", "assignee", "priority"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("issueSummary missing %q: %#v", key, got)
		}
	}
}

func TestIssueSummaryAssigneeIsNilWhenUnassigned(t *testing.T) {
	got := issueSummary(&jira.Issue{
		Key:    jira.String("PROJ-1"),
		Fields: &jira.IssueFields{Summary: jira.String("Hi")},
	})
	if got["assignee"] != nil {
		t.Fatalf("assignee = %#v, want nil", got["assignee"])
	}
}

func TestIssueSummaryAssigneeUsesOnlyAccountIDAndDisplayName(t *testing.T) {
	got := issueSummary(&jira.Issue{
		Key: jira.String("PROJ-1"),
		Fields: &jira.IssueFields{
			Assignee: &jira.User{
				AccountID:    jira.String("acc-1"),
				DisplayName:  jira.String("Riley Chen"),
				EmailAddress: jira.String("riley@example.com"),
			},
		},
	})
	assignee, ok := got["assignee"].(map[string]any)
	if !ok {
		t.Fatalf("assignee = %#v, want map[string]any", got["assignee"])
	}
	if assignee["account_id"] != "acc-1" {
		t.Fatalf("account_id = %#v", assignee["account_id"])
	}
	if assignee["display_name"] != "Riley Chen" {
		t.Fatalf("display_name = %#v", assignee["display_name"])
	}
	for _, leaked := range []string{"email_address", "emailAddress", "accountId", "displayName"} {
		if _, ok := assignee[leaked]; ok {
			t.Fatalf("assignee leaked %q: %#v", leaked, assignee)
		}
	}
}

func TestIssueSummaryPriorityIsNilWhenAbsent(t *testing.T) {
	got := issueSummary(&jira.Issue{Key: jira.String("PROJ-1"), Fields: &jira.IssueFields{}})
	if got["priority"] != nil {
		t.Fatalf("priority = %#v, want nil", got["priority"])
	}
}

func TestIssueSummaryPriorityIsStringWhenPresent(t *testing.T) {
	got := issueSummary(&jira.Issue{
		Key:    jira.String("PROJ-1"),
		Fields: &jira.IssueFields{Priority: &jira.Priority{Name: jira.String("High")}},
	})
	if got["priority"] != "High" {
		t.Fatalf("priority = %#v, want \"High\"", got["priority"])
	}
}
