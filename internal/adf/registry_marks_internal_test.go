package adf

import (
	"strings"
	"testing"
)

// Guardrail: the validator enforces that the code mark is mutually
// exclusive with the decorative marks (codeExclusiveMarks), and the
// markdown converter resolves that combination at conversion time. The
// adf-matrix registry note is the only place agents learn the rule before
// authoring — keep it in step with the validator: as long as the exclusion
// set is non-empty, the code mark's note must state the link-only rule.
func TestRegistryCodeMarkNoteMatchesValidatorRule(t *testing.T) {
	if len(codeExclusiveMarks) == 0 {
		t.Skip("validator no longer restricts code-mark combinations")
	}
	for _, e := range registryEntries {
		if e.Kind != KindMark || e.Name != "code" {
			continue
		}
		note := strings.ToLower(e.Notes)
		if !strings.Contains(note, "link") || !strings.Contains(note, "combine") {
			t.Fatalf("code mark registry note must document the link-only combination rule; got %q", e.Notes)
		}
		return
	}
	t.Fatal("code mark not found in registry")
}
