package cli

import (
	"bytes"
	"strings"
	"testing"

	termansi "github.com/gechr/x/ansi"
)

func jqlPreviewData() map[string]any {
	return map[string]any{
		"jql":         "project = DEVOPS ORDER BY updated DESC",
		"url":         "https://acme.atlassian.net/issues/?jql=project+%3D+DEVOPS",
		"detail":      false,
		"precedence":  "none",
		"board_scope": map[string]any{"applied": false},
		"issues":      []map[string]any{},
	}
}

// The --as-jql / jql build preview is a copy/paste affordance: its human
// output must be the bare JQL string and nothing else — no INF prefix, no
// board_scope/detail/precedence diagnostic fields leaking from the envelope.
func TestJQLPreviewPlainPrintsOnlyTheQuery(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"issue.list.jql", "jql.build"} {
		var buf bytes.Buffer
		if err := WriteCommandPlain(&buf, command, jqlPreviewData(), WithPlainTTY(false)); err != nil {
			t.Fatalf("WriteCommandPlain(%s) error = %v", command, err)
		}
		got := termansi.Strip(buf.String())
		if strings.TrimSpace(got) != "project = DEVOPS ORDER BY updated DESC" {
			t.Fatalf("%s: want only the bare JQL, got:\n%q", command, got)
		}
		for _, leak := range []string{"INF", "board_scope", "precedence", "detail", "issues", "url="} {
			if strings.Contains(got, leak) {
				t.Fatalf("%s: preview leaked %q:\n%q", command, leak, got)
			}
		}
	}
}

// --debug restores the operational diagnostics for troubleshooting, while the
// JQL itself stays present.
func TestJQLPreviewPlainDebugRestoresDiagnostics(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "issue.list.jql", jqlPreviewData(), WithPlainTTY(false), WithPlainDebug(true)); err != nil {
		t.Fatalf("WriteCommandPlain error = %v", err)
	}
	got := termansi.Strip(buf.String())
	if !strings.Contains(got, "project = DEVOPS ORDER BY updated DESC") {
		t.Fatalf("debug preview dropped the JQL:\n%q", got)
	}
	// Dotted form, matching every other command's human output — not a raw
	// Go map blob.
	for _, want := range []string{"precedence", "board_scope.applied"} {
		if !strings.Contains(got, want) {
			t.Fatalf("debug preview missing diagnostic %q:\n%q", want, got)
		}
	}
	if strings.Contains(got, "map[") {
		t.Fatalf("debug preview rendered a raw Go map blob instead of dotted fields:\n%q", got)
	}
}

// On a TTY the JQL is wrapped in an OSC 8 hyperlink to the Jira search URL, but
// its visible text stays the bare query so a copy/paste still yields valid JQL.
func TestJQLPreviewPlainTTYRendersHyperlink(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "issue.list.jql", jqlPreviewData(), WithPlainTTY(true)); err != nil {
		t.Fatalf("WriteCommandPlain error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "\x1b]8;;https://acme.atlassian.net/issues/") {
		t.Fatalf("TTY preview did not wrap the JQL in an OSC 8 hyperlink:\n%q", got)
	}
	// The link is underlined on a TTY so it reads as a link — the shared
	// affordance every CLI hyperlink uses.
	if !strings.Contains(got, "\x1b[4m") {
		t.Fatalf("TTY hyperlink was not underlined:\n%q", got)
	}
	if !strings.Contains(termansi.Strip(got), "project = DEVOPS ORDER BY updated DESC") {
		t.Fatalf("hyperlinked JQL lost its visible text:\n%q", got)
	}
}

// Off a TTY the preview stays plain text — no escape sequences — so it pipes
// cleanly.
func TestJQLPreviewPlainNonTTYHasNoEscapes(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "issue.list.jql", jqlPreviewData(), WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteCommandPlain error = %v", err)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("non-TTY preview emitted escape sequences:\n%q", buf.String())
	}
}
