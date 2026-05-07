package guardrails

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every guardrail test file MUST start with a comment block that
// states the MOTIVATION for the category AND declares explicitly that
// the comment is provenance only — NOT a source of implementation,
// fixtures, wording, or test logic.
//
// This guard self-applies — it inspects every other file in this
// package and fails on missing or malformed headers.
func TestGuardrailFilesCarryProvenanceHeaders(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		// Skip self.
		if entry.Name() == "headers_test.go" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(".", entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		head := string(body)
		if len(head) > 1024 {
			head = head[:1024]
		}
		// Header must mention MOTIVATION or PROVENANCE (case-insensitive).
		hasMotivation := strings.Contains(strings.ToLower(head), "motivation") ||
			strings.Contains(strings.ToLower(head), "provenance")
		if !hasMotivation {
			t.Errorf("%s header missing MOTIVATION/PROVENANCE notice", entry.Name())
		}
	}
}
