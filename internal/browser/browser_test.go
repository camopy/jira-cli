package browser_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/browser"
)

func TestIssueURL(t *testing.T) {
	for _, tc := range []struct {
		base, key, want string
	}{
		{"https://acme.atlassian.net", "JCT-1", "https://acme.atlassian.net/browse/JCT-1"},
		{"https://acme.atlassian.net/", "JCT-1", "https://acme.atlassian.net/browse/JCT-1"}, // trailing slash trimmed
		{"  https://acme.atlassian.net  ", "  JCT-1  ", "https://acme.atlassian.net/browse/JCT-1"},
		{"", "JCT-1", ""},
		{"https://acme.atlassian.net", "", ""},
	} {
		if got := browser.IssueURL(tc.base, tc.key); got != tc.want {
			t.Errorf("IssueURL(%q, %q) = %q, want %q", tc.base, tc.key, got, tc.want)
		}
	}
}

func TestSearchURL(t *testing.T) {
	for _, tc := range []struct {
		base, jql, want string
	}{
		{"https://acme.atlassian.net", "project = JCT", "https://acme.atlassian.net/issues/?jql=project+%3D+JCT"},
		{"https://acme.atlassian.net/", "assignee = currentUser()", "https://acme.atlassian.net/issues/?jql=assignee+%3D+currentUser%28%29"},
		{"", "project = JCT", ""},
		{"https://acme.atlassian.net", "  ", ""},
	} {
		if got := browser.SearchURL(tc.base, tc.jql); got != tc.want {
			t.Errorf("SearchURL(%q, %q) = %q, want %q", tc.base, tc.jql, got, tc.want)
		}
	}
}
