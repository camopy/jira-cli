package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Resolve returns the editor command to launch, given a configured
// editor (typically pulled from profile.editor or global config.editor).
//
// Precedence:
//
//	JIRA_EDITOR → configured → $EDITOR → $VISUAL → "vi"
//
// JIRA_EDITOR is the per-invocation override; configured is the user's
// pinned preference in jira-cli config; $EDITOR/$VISUAL are the
// platform defaults; "vi" is the last-resort fallback. An empty value
// at any level falls through to the next.
func Resolve(configured string) string {
	if v := strings.TrimSpace(os.Getenv("JIRA_EDITOR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(configured); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("EDITOR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("VISUAL")); v != "" {
		return v
	}
	return "vi"
}

// ResolveEditor is the no-arg variant for callers that don't have
// access to the config layer. Equivalent to Resolve("").
func ResolveEditor() string { return Resolve("") }

func Run(ctx context.Context, editorCommand, path string) error {
	editorCommand = Resolve(editorCommand)
	if editorCommand == "" {
		editorCommand = "vi"
	}
	parts := strings.Fields(editorCommand)
	if len(parts) == 0 {
		return fmt.Errorf("editor command is required")
	}
	args := append(parts[1:], path)
	cmd := exec.CommandContext(ctx, parts[0], args...) //nolint:gosec // Editor command is explicit user configuration and exec.Command does not invoke a shell.
	// Editor subprocess needs the user's terminal stdin to function (vim,
	// nano, etc. read keystrokes). This is NOT the CLI reading stdin —
	// it's plumbing the terminal through to the child. Exempt from the
	// stdin-discipline guard via the suppressing comment below.
	cmd.Stdin = os.Stdin //nolint:forbidigo // editor child process; not a CLI stdin read (stdin-exempt)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run editor %q: %w", parts[0], err)
	}
	return nil
}
