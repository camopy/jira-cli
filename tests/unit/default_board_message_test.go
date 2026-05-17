package unit

// Pin the literal wording of the missing-default error message. The
// contract test asserts this string in the envelope; this unit test is
// a faster RED-line for any future drift.
//
// Wording lives in pkg/jira.DefaultBoardMissingMessage so the
// boardScopeFromFlags helper can call it without re-defining the text.

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestDefaultBoardMissingMessageLiteralWording(t *testing.T) {
	got := jira.DefaultBoardMissingMessage("default", "Engineering Sprint")
	want := `default_board "Engineering Sprint" not found in boards cache — run "jira cache boards --refresh" or unset with "jira config set profiles.default.default_board ''"`
	if got != want {
		t.Fatalf("DefaultBoardMissingMessage drift:\n got: %s\nwant: %s", got, want)
	}
}

func TestDefaultBoardMissingMessageInterpolatesProfile(t *testing.T) {
	got := jira.DefaultBoardMissingMessage("work", "Platform")
	want := `default_board "Platform" not found in boards cache — run "jira cache boards --refresh" or unset with "jira config set profiles.work.default_board ''"`
	if got != want {
		t.Fatalf("DefaultBoardMissingMessage profile interpolation drift:\n got: %s\nwant: %s", got, want)
	}
}
