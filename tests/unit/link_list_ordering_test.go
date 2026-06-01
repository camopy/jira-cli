// Unit test for the `(direction, type.name, other_issue.key)` ASC
// sort applied to a fixture mixing inward+outward links of the same
// and different types.
package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestSortLinksDirectionTypeKey(t *testing.T) {
	links := []jira.IssueLinkView{
		{ID: "1", Direction: "outward", Type: jira.IssueLinkType{Name: "Relates"}, OtherIssue: jira.IssueRef{Key: "KAN-200"}},
		{ID: "2", Direction: "inward", Type: jira.IssueLinkType{Name: "Blocks"}, OtherIssue: jira.IssueRef{Key: "KAN-100"}},
		{ID: "3", Direction: "outward", Type: jira.IssueLinkType{Name: "Blocks"}, OtherIssue: jira.IssueRef{Key: "KAN-300"}},
		{ID: "4", Direction: "outward", Type: jira.IssueLinkType{Name: "Blocks"}, OtherIssue: jira.IssueRef{Key: "KAN-250"}},
		{ID: "5", Direction: "inward", Type: jira.IssueLinkType{Name: "Cloners"}, OtherIssue: jira.IssueRef{Key: "KAN-50"}},
	}
	jira.SortIssueLinks(links)
	wantOrder := []string{"2", "5", "4", "3", "1"}
	for i, link := range links {
		if link.ID != wantOrder[i] {
			t.Fatalf("position %d: got id=%s want=%s; full order = %+v", i, link.ID, wantOrder[i], links)
		}
	}
}

func TestSortLinksStableOnTies(t *testing.T) {
	links := []jira.IssueLinkView{
		{ID: "first", Direction: "outward", Type: jira.IssueLinkType{Name: "Blocks"}, OtherIssue: jira.IssueRef{Key: "KAN-1"}},
		{ID: "second", Direction: "outward", Type: jira.IssueLinkType{Name: "Blocks"}, OtherIssue: jira.IssueRef{Key: "KAN-1"}},
	}
	jira.SortIssueLinks(links)
	if links[0].ID != "first" || links[1].ID != "second" {
		t.Fatalf("stable sort violated: %+v", links)
	}
}

func TestSortLinksEmpty(t *testing.T) {
	var links []jira.IssueLinkView
	jira.SortIssueLinks(links)
	if len(links) != 0 {
		t.Fatalf("len changed: %+v", links)
	}
}
