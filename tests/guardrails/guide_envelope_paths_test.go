// MOTIVATION: the issue-creation envelope nests the new issue under
// data.issue (create) or data.result (clone, move); there is no top-level
// data.key / data.id / data.self. A guide that documents the flat path
// sends scripted agents to a jq extraction that returns empty on a
// successful create, logging spurious failures. These guards pin the
// mutation guides to the real nested paths so the drift cannot return.
// Comments here are PROVENANCE ONLY and MUST NOT be a source of fixtures or
// wording.
package guardrails

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const guideDir = "../../internal/agentguides/guides"

func readGuide(t *testing.T, slug string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(guideDir, slug+".md"))
	if err != nil {
		t.Fatalf("read guide %s: %v", slug, err)
	}
	return string(data)
}

// The create-workflow guide must document the new key under data.issue and
// must not reference a flat data.key.
func TestCreateGuideUsesNestedIssueKey(t *testing.T) {
	body := readGuide(t, "shape-issues")
	if !strings.Contains(body, "data.issue.key") {
		t.Errorf("shape-issues: missing the real key path `data.issue.key`")
	}
	if strings.Contains(body, "`data.key`") {
		t.Errorf("shape-issues: references a flat `data.key`; the key is nested under data.issue")
	}
}

// The clone/move workflow guide shares the destructive-mutation envelope:
// the resulting issue is under data.result, while data.issue echoes the
// source key. It must document data.result.key and not a flat data.id /
// data.self.
func TestResultEnvelopeGuideUsesResultPaths(t *testing.T) {
	body := readGuide(t, "restructure-issues")
	if !strings.Contains(body, "data.result.key") {
		t.Errorf("restructure-issues: missing the real key path `data.result.key`")
	}
	for _, flat := range []string{"`data.id`", "`data.self`", "`data.key`"} {
		if strings.Contains(body, flat) {
			t.Errorf("restructure-issues: references flat %s; the result is nested under data.result", flat)
		}
	}
}
