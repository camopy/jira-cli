package contract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

// In JSON mode, warnings live in the envelope body on stdout. Stdout
// MUST be the only data sink; stderr stays empty unless an error or
// warning routing path needs it.
func TestWarningsInJSONStayInEnvelopeOnStdout(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	w := cli.Warning{Type: "test_warn", Message: "demo", Lossy: true}
	err := cli.RouteWarnings(cli.RouteOptions{
		Stdout:   stdout,
		Stderr:   stderr,
		Mode:     cli.RouteJSON,
		Envelope: cli.Envelope{Meta: cli.Meta{Command: "x", Timestamp: "t"}, Data: map[string]any{"k": "v"}, Errors: []cli.Error{}, Warnings: []cli.Warning{w}},
	})
	if err != nil {
		t.Fatalf("RouteWarnings: %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("JSON mode must not mirror warnings to stderr; got: %s", stderr.String())
	}

	var parsed map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	wa, ok := parsed["warnings"].([]any)
	if !ok || len(wa) != 1 {
		t.Fatalf("warnings missing from envelope: %v", parsed)
	}
}

// In --plain / TTY mode, command data stays on stdout, every warning
// mirrors to stderr as a clog WRN line, and NO warning text leaks into
// stdout (tables, lists, anything).
func TestWarningsInPlainGoToStderrOnly(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	warnings := []cli.Warning{
		{Type: "adf_compatibility", Message: "inlineCard not supported", Field: "description", NodeType: "inlineCard", Lossy: true},
		{Type: "field_not_on_screen", Message: "epic_link dropped", Field: "epic_link", Lossy: true},
	}
	err := cli.RouteWarnings(cli.RouteOptions{
		Stdout:   stdout,
		Stderr:   stderr,
		Mode:     cli.RoutePlain,
		Command:  "issue.create",
		Data:     map[string]any{"key": "JCT-1"},
		Warnings: warnings,
	})
	if err != nil {
		t.Fatalf("RouteWarnings: %v", err)
	}

	stdoutText := stdout.String()
	stderrText := stderr.String()

	for _, leaked := range []string{"inlineCard not supported", "epic_link dropped", "adf_compatibility", "field_not_on_screen"} {
		if strings.Contains(stdoutText, leaked) {
			t.Fatalf("warning text leaked into stdout (%q):\n%s", leaked, stdoutText)
		}
	}
	for _, expected := range []string{"inlineCard not supported", "epic_link dropped"} {
		if !strings.Contains(stderrText, expected) {
			t.Fatalf("stderr missing warning %q.\nstderr:\n%s", expected, stderrText)
		}
	}
	if !strings.Contains(strings.ToUpper(stderrText), "WRN") {
		t.Fatalf("stderr should carry clog WRN markers; got:\n%s", stderrText)
	}
}

// Warning strings echo Jira-controlled text — an unknown ADF node type is
// whatever type string the inbound document declared — so the stderr
// mirror must strip ANSI escapes and control bytes from every field.
func TestWarningsMirroredToStderrAreSanitized(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := cli.RouteWarnings(cli.RouteOptions{
		Stdout:  stdout,
		Stderr:  stderr,
		Mode:    cli.RoutePlain,
		Command: "issue.view",
		Data:    map[string]any{"key": "JCT-1"},
		// Every rendered string field carries an injection so reverting any
		// single sanitizer wrap fails this test. The C1 CSI (U+009B) and DEL
		// runes sit at the end of their values on purpose: they exercise
		// isControlRune's upper branch without the ANSI stripper consuming
		// trailing printable text as sequence parameters.
		Warnings: []cli.Warning{{
			Type:     "unknown_adf_node\x1b[31m",
			Message:  "description ADF construct \x1b[31mevil\x1b[0m dropped",
			Field:    "descri\x07ption",
			Path:     "content[3]\u009b",
			NodeType: "evil\x07type\x1b]8;;http://x\x1b\\",
			MarkType: "mark\u007f",
			Lossy:    true,
		}},
	})
	if err != nil {
		t.Fatalf("RouteWarnings: %v", err)
	}

	stderrText := stderr.String()
	for _, banned := range []string{"\x1b", "\x07", "\u009b", "\u007f"} {
		if strings.Contains(stderrText, banned) {
			t.Fatalf("warning mirror leaked control byte %q to stderr:\n%q", banned, stderrText)
		}
	}
	for _, expected := range []string{
		"description ADF construct evil dropped",
		"unknown_adf_node",
		"description",
		"content[3]",
		"eviltype",
		"mark",
	} {
		if !strings.Contains(stderrText, expected) {
			t.Fatalf("sanitized warning field mangled (want %q); stderr:\n%q", expected, stderrText)
		}
	}
}

// When warnings are empty in plain mode, stderr stays empty.
func TestNoWarningsKeepsStderrSilent(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := cli.RouteWarnings(cli.RouteOptions{
		Stdout:  stdout,
		Stderr:  stderr,
		Mode:    cli.RoutePlain,
		Command: "issue.view",
		Data:    map[string]any{"key": "JCT-1"},
	})
	if err != nil {
		t.Fatalf("RouteWarnings: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr must be silent when no warnings; got: %s", stderr.String())
	}
}
