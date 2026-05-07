// MOTIVATION: "convert-on-submit" is a recurring TUI pattern where the
// editor holds a Markdown string and converts to ADF only at submit
// time, allowing silent semantic loss between user intent and what
// reaches Jira. Comments in this file are PROVENANCE ONLY and MUST
// NOT be a source of implementation, fixtures, wording, or test
// logic.
package guardrails

import (
	"os"
	"strings"
	"testing"
)

// The TUI submit path MUST NOT call adf.FromMarkdown — the
// editor.Editor produces the ADF document as the user types, and the
// App receives Document directly via InputSubmitted. Calling
// FromMarkdown at submit is exactly the "convert-on-submit step" the
// architecture forbids.
//
// This guard greps internal/tui/app.go for the offending call and
// fails if it reappears.
func TestTUIAppDoesNotCallFromMarkdownAtSubmit(t *testing.T) {
	body, err := os.ReadFile("../../internal/tui/app.go")
	if err != nil {
		t.Fatalf("read app.go: %v", err)
	}
	text := string(body)
	if strings.Contains(text, "adf.FromMarkdown(") {
		t.Errorf("internal/tui/app.go calls adf.FromMarkdown — convert-on-submit step forbidden")
	}
}
