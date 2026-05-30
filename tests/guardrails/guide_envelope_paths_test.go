// MOTIVATION: the issue-creation envelope nests the new issue under
// data.issue (create, subtask) or data.result (clone); there is no
// top-level data.key / data.id / data.self. A guide that documents the
// flat path sends scripted agents to a jq extraction that returns empty on
// a successful create, logging spurious failures. These guards pin each
// create-family guide to the real nested path so the drift cannot return.
// Comments here are PROVENANCE ONLY and MUST NOT be a source of fixtures or
// wording.
package guardrails

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const guideDir = "../../internal/cli/agent/guide"

func readGuide(t *testing.T, slug string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(guideDir, slug+".md"))
	if err != nil {
		t.Fatalf("read guide %s: %v", slug, err)
	}
	return string(data)
}

// The create and subtask guides must document the new key under data.issue
// and must not reference a flat data.key.
func TestCreateGuidesUseNestedIssueKey(t *testing.T) {
	for _, slug := range []string{"create_issue", "create_subtask"} {
		body := readGuide(t, slug)
		if !strings.Contains(body, "data.issue.key") {
			t.Errorf("%s: missing the real key path `data.issue.key`", slug)
		}
		if strings.Contains(body, "`data.key`") {
			t.Errorf("%s: references a flat `data.key`; the key is nested under data.issue", slug)
		}
	}
}

// The clone and move guides share the destructive-mutation envelope: the
// resulting issue is under data.result, while data.issue echoes the source
// key. They must document data.result.key and not a flat data.id / data.self.
func TestResultEnvelopeGuidesUseResultPaths(t *testing.T) {
	for _, slug := range []string{"clone_issue", "move_issue"} {
		body := readGuide(t, slug)
		if !strings.Contains(body, "data.result.key") {
			t.Errorf("%s: missing the real key path `data.result.key`", slug)
		}
		for _, flat := range []string{"`data.id`", "`data.self`", "`data.key`"} {
			if strings.Contains(body, flat) {
				t.Errorf("%s: references flat %s; the result is nested under data.result", slug, flat)
			}
		}
	}
}
