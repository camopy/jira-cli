package issue

import (
	"slices"
	"testing"
)

// issue view must expand transitions and editmeta on the read so the payload
// carries the one-read discovery blocks the agent guide documents.
func TestIssueViewGetOptionsExpandDiscoveryBlocks(t *testing.T) {
	expand := issueViewGetOptions().Expand
	for _, want := range []string{"transitions", "editmeta"} {
		if !slices.Contains(expand, want) {
			t.Errorf("issueViewGetOptions().Expand = %v, missing %q", expand, want)
		}
	}
}
