package issue

import (
	"slices"
	"testing"
)

// issue view must expand transitions and editmeta on the default read so the
// payload carries the one-read discovery blocks the agent guide documents.
func TestIssueViewGetOptionsExpandDiscoveryBlocks(t *testing.T) {
	opts := issueViewGetOptions(nil)
	for _, want := range []string{"transitions", "editmeta"} {
		if !slices.Contains(opts.Expand, want) {
			t.Errorf("issueViewGetOptions(nil).Expand = %v, missing %q", opts.Expand, want)
		}
	}
	if len(opts.Fields) != 0 {
		t.Errorf("issueViewGetOptions(nil).Fields = %v, want empty", opts.Fields)
	}
}

// A --fields read is a deliberate slim payload: the fields parameter is set,
// the render expansions survive (Jira narrows them to the requested fields),
// and the edit-support expansions are dropped.
func TestIssueViewGetOptionsFieldsNarrowsRead(t *testing.T) {
	opts := issueViewGetOptions([]string{"summary", "status"})
	if got, want := opts.Fields, []string{"summary", "status"}; !slices.Equal(got, want) {
		t.Errorf("issueViewGetOptions(fields).Fields = %v, want %v", got, want)
	}
	for _, want := range []string{"renderedFields", "names", "schema"} {
		if !slices.Contains(opts.Expand, want) {
			t.Errorf("issueViewGetOptions(fields).Expand = %v, missing %q", opts.Expand, want)
		}
	}
	for _, banned := range []string{"transitions", "operations", "editmeta"} {
		if slices.Contains(opts.Expand, banned) {
			t.Errorf("issueViewGetOptions(fields).Expand = %v, must not carry %q", opts.Expand, banned)
		}
	}
}
