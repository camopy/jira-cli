package unit

import (
	"os"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func TestOutputDetectorAgentAndNonTTY(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CLAUDE_CODE", "1")
	d := cli.Detect(os.Stdout, false)
	if !d.Agent || d.Mode != cli.ModeCompact {
		t.Fatalf("agent detection = %+v", d)
	}

	clearAgentEnv(t)
	d = cli.Detect(nil, false)
	if d.Mode != cli.ModeJSON {
		t.Fatalf("non-tty detection = %+v", d)
	}
}

func TestOutputDetectorTTYUsesPlainCommandModeNotTUI(t *testing.T) {
	clearAgentEnv(t)
	d := cli.Detect(os.Stdout, false)
	if d.IsTTY && d.Mode != cli.ModePlain {
		t.Fatalf("tty command detection = %+v, want plain command mode", d)
	}
	if d.Mode == cli.ModeTUI {
		t.Fatalf("detector should not use TUI mode for ordinary command output: %+v", d)
	}
}

func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AI_AGENT",
		"AGENT",
		"CODEX_SANDBOX",
		"CODEX_CI",
		"CODEX_THREAD_ID",
		"CODEX",
		"OPENAI_CODEX",
		"GEMINI_CLI",
		"COPILOT_CLI",
		"COPILOT",
		"GITHUB_COPILOT",
		"OPENCODE",
		"CURSOR_TERMINAL",
		"CURSOR_AGENT",
		"CLAUDECODE",
		"CLAUDE_CODE",
	} {
		t.Setenv(key, "")
	}
}
