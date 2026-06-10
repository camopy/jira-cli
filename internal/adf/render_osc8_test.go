package adf_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// Rendered ADF text MUST present issue keys and bare URLs as
// activatable OSC 8 hyperlinks when emitted to a TTY, and degrade to
// plain text in non-TTY contexts.
//
// Spec: "OSC 8 hyperlinks in TTY, plain text in non-TTY".
// Per-profile base URL is required to construct the issue-key links.
func TestRenderADFEmitsOSC8WhenTerminalAndPlainOtherwise(t *testing.T) {
	doc, _, err := adf.Parse([]byte(`{
		"type": "doc", "version": 1, "content": [
			{"type": "paragraph", "content": [
				{"type": "text", "text": "see "},
				{"type": "text", "text": "JCT-7"},
				{"type": "text", "text": " or visit "},
				{"type": "text", "text": "https://example.com/info"}
			]}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	tty := adf.RenderActivatable(doc, adf.RenderOptions{IsTerminal: true, BaseURL: "https://example.atlassian.net"})
	plain := adf.RenderActivatable(doc, adf.RenderOptions{IsTerminal: false, BaseURL: "https://example.atlassian.net"})

	// TTY output MUST contain the OSC 8 hyperlink introducer.
	if !strings.Contains(tty, "\x1b]8;;") {
		t.Fatalf("expected OSC 8 escape in TTY render; got: %q", tty)
	}
	// Both URL and key should produce a link sequence.
	if !strings.Contains(tty, "https://example.com/info") {
		t.Fatalf("URL missing from TTY render: %q", tty)
	}
	if !strings.Contains(tty, "https://example.atlassian.net/browse/JCT-7") {
		t.Fatalf("issue-key did not link to base URL; got: %q", tty)
	}

	// Plain output MUST NOT contain any OSC 8 escapes.
	if strings.Contains(plain, "\x1b]8;;") {
		t.Fatalf("plain render leaked OSC 8 escapes; got: %q", plain)
	}
	// Plain output MUST still contain the visible text.
	if !strings.Contains(plain, "JCT-7") {
		t.Fatalf("plain render missing issue key text: %q", plain)
	}
	if !strings.Contains(plain, "https://example.com/info") {
		t.Fatalf("plain render missing URL text: %q", plain)
	}
}
