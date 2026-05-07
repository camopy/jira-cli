// `jira issue edit ISSUE-KEY` MUST refuse to spawn the interactive
// editor when an agent context is detected. Without this guard, an
// LLM-driven agent that runs `jira issue edit` falls through to the
// editor flow, which it cannot drive — symptoms range from a stuck
// process to silent data loss when EDITOR=code (which forks and
// returns immediately, racing the parent's tempfile cleanup).
//
// The pinned remediation message points the agent at flag-based
// inputs (--summary / --json-input) so the agent knows how to
// proceed without an editor.
package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIssueEditRefusesEditorInAgentMode(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	c := exec.Command(bin, "--config", cfg, "issue", "edit", "PROJ-1", "--json")
	// CLAUDECODE=1 trips the agent detector in internal/cli/detector.go
	// without requiring a real Claude Code harness. Any agent env var
	// from that detector's set would work; this one matches the user's
	// own session and is the most realistic simulation.
	c.Env = append(os.Environ(), "CLAUDECODE=1")
	out, err := c.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit when agent context tries to open editor; got success:\n%s", out)
	}
	combined := string(out)

	// Validation error → exit 3. Substring match on the envelope's
	// errors[].message. The remediation MUST name at least one
	// flag-based alternative so the agent knows how to recover.
	wantSubstrings := []string{
		"interactive",
		"--summary",
		"--json-input",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(combined, want) {
			t.Errorf("agent-mode refusal output missing %q\n--- combined ---\n%s", want, combined)
		}
	}
}

func TestIssueEditAcceptsFlagBasedInputInAgentMode(t *testing.T) {
	// Counterpoint to the refusal test: when the agent passes
	// --summary (or any field flag), the editor path is bypassed
	// entirely and the command should proceed normally even with the
	// agent env var set. This guards against an over-broad refusal.
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)

	c := exec.Command(bin, "--config", cfg, "issue", "edit", "PROJ-1",
		"--summary", "renamed", "--dry-run", "--json")
	c.Env = append(os.Environ(), "CLAUDECODE=1")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("agent-mode + --summary should succeed (no editor needed); got error %v:\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("envelope is not JSON: %v\n%s", err, out)
	}
	data, _ := env["data"].(map[string]any)
	if dryRun, _ := data["dry_run"].(bool); !dryRun {
		t.Fatalf("expected dry_run=true in envelope; got data=%+v", data)
	}
}
