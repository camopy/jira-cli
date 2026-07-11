package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// A comment body arrives as a typed ADF document, which the up-front
// payload sanitization walk leaves untouched — the one-line preview is
// therefore its own terminal-sanitizer boundary and must strip ANSI
// escapes and control bytes that Jira-controlled text could carry.
func TestCommentPreviewSanitizesJiraText(t *testing.T) {
	body := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{{
		Type: "paragraph",
		Content: []adf.Node{{
			// The C1 CSI (U+009B) and DEL runes end the string on purpose:
			// they exercise isControlRune's upper branch (0x7f-0x9f) without
			// the ANSI stripper consuming trailing text as CSI parameters.
			Type: "text",
			Text: "evil\x1b]0;owned\x07 first\rline\ntwo\x08 del\u007fete\u009b",
		}},
	}}}

	var buf bytes.Buffer
	err := WriteCommentListPlain(&buf, "issue.comment.list", map[string]any{
		"comments": []any{map[string]any{
			"id":      "100",
			"body":    body,
			"author":  map[string]any{"display_name": "Alice"},
			"created": "2026-04-01T10:00:00.000+0000",
			"updated": "2026-04-01T10:00:00.000+0000",
		}},
	})
	if err != nil {
		t.Fatalf("WriteCommentListPlain: %v", err)
	}

	got := buf.String()
	for _, banned := range []string{"\x1b", "\x07", "\x08", "\r", "\u007f", "\u009b"} {
		if strings.Contains(got, banned) {
			t.Fatalf("comment preview leaked control byte %q:\n%q", banned, got)
		}
	}
	// The OSC title sequence is consumed whole (its "owned" payload with
	// it), the bare CR is dropped, the newline collapses to a space, and
	// the DEL and C1 CSI runes are dropped in place.
	if !strings.Contains(got, "evil firstline two delete") {
		t.Fatalf("sanitized preview text mangled; got:\n%q", got)
	}
}
