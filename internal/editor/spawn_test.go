package editor

import (
	"testing"
)

// The resolver precedence MUST be:
//
//	JIRA_EDITOR → configured editor → $EDITOR → $VISUAL → "vi".
//
// configured comes from profile.editor or global config.editor; the
// caller passes a single string argument to Resolve.
func TestResolvePrecedenceJiraEditorWins(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "nano")
	t.Setenv("EDITOR", "vim")
	t.Setenv("VISUAL", "emacs")

	if got := Resolve("code"); got != "nano" {
		t.Fatalf("JIRA_EDITOR must win; got %q", got)
	}
}

func TestResolvePrecedenceConfiguredBeforeEnv(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "")
	t.Setenv("EDITOR", "vim")
	t.Setenv("VISUAL", "emacs")

	if got := Resolve("code"); got != "code" {
		t.Fatalf("configured editor must win over $EDITOR; got %q", got)
	}
}

func TestResolveFallsBackToEditorThenVisualThenVi(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "")

	t.Setenv("EDITOR", "vim")
	t.Setenv("VISUAL", "emacs")
	if got := Resolve(""); got != "vim" {
		t.Fatalf("$EDITOR fallback wrong: %q", got)
	}

	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "emacs")
	if got := Resolve(""); got != "emacs" {
		t.Fatalf("$VISUAL fallback wrong: %q", got)
	}

	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	if got := Resolve(""); got != "vi" {
		t.Fatalf("vi fallback wrong: %q", got)
	}
}

// Existing ResolveEditor() (no args) MUST still work for callers that
// don't have a configured editor — preserves the old API for the
// minority of paths that don't have config access.
func TestResolveEditorBackwardCompatible(t *testing.T) {
	t.Setenv("JIRA_EDITOR", "code")
	if got := ResolveEditor(); got != "code" {
		t.Fatalf("ResolveEditor backcompat broken: %q", got)
	}
}
