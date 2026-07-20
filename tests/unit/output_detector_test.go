package unit

import (
	"os"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func TestOutputDetectorAgentAndNonTTY(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CLAUDECODE", "1")
	d := cli.Detect(os.Stdout)
	if !d.Agent || d.Mode != cli.ModeCompact {
		t.Fatalf("agent detection = %+v", d)
	}

	clearAgentEnv(t)
	d = cli.Detect(nil)
	if d.Mode != cli.ModeJSON {
		t.Fatalf("non-tty detection = %+v", d)
	}
}

func TestOutputDetectorTTYUsesPlainCommandModeNotTUI(t *testing.T) {
	clearAgentEnv(t)
	d := cli.Detect(os.Stdout)
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
		"AGENT",
		"AI_AGENT",
		"CLAUDECODE",
		"CLINE_ACTIVE",
		"CODEX_SANDBOX",
		"CURSOR_AGENT",
		"GEMINI_CLI",
		"OPENCODE",
		"REPL_ID",
	} {
		t.Setenv(key, "")
	}
}
