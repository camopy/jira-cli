// MOTIVATION: "convert-on-submit" is a recurring TUI pattern where the
// editor holds a Markdown string and converts to ADF only at submit
// time, allowing silent semantic loss between user intent and what
// reaches Jira. Comments in this file are PROVENANCE ONLY and MUST
// NOT be a source of implementation, fixtures, wording, or test
// logic.
package guardrails

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The TUI MUST NOT call adf.FromMarkdown — the silent converter drops
// unsupported Markdown without telling the user. Submit paths that
// accept Markdown text use adf.FromMarkdownLossy, which reports what
// was lost instead of discarding it silently.
//
// This guard greps every Go file under internal/tui for the offending
// call and fails if it appears.
func TestTUIDoesNotCallFromMarkdownSilently(t *testing.T) {
	root := "../../internal/tui"
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), "adf.FromMarkdown(") {
			t.Errorf("%s calls adf.FromMarkdown — silent convert-on-submit forbidden, use FromMarkdownLossy", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}
