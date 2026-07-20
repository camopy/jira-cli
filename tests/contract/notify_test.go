package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// seedNotifyCache writes a fresh notify cache claiming a far-future release,
// so an update is pending without any network access. The layout matches
// clive/notify: $XDG_CACHE_HOME/<binary>/last-update/check.
func seedNotifyCache(t *testing.T) string {
	t.Helper()
	cacheHome := t.TempDir()
	dir := filepath.Join(cacheHome, "jira")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	stamp := []byte(`{"version":1,"track":"","latest":"v99.0.0"}` + "\n")
	if err := os.MkdirAll(filepath.Join(dir, "last-update"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "last-update", "check"), stamp, 0o644); err != nil {
		t.Fatal(err)
	}
	return cacheHome
}

// Machine envelopes must stay byte-clean even when the notify cache says an
// update is pending: piped stderr suppresses the hint, and detected agent
// context disables the check entirely. Both invocations run with a seeded
// pending-update cache and must produce exactly one JSON line on stdout and
// nothing on stderr.
func TestNotifyHintNeverLeaksIntoMachineOutput(t *testing.T) {
	cacheHome := seedNotifyCache(t)
	bin := buildJiraBinary(t)

	for name, extraEnv := range map[string][]string{
		"piped":     {"AGENT=", "AI_AGENT=", "CLAUDECODE=", "CLINE_ACTIVE=", "CODEX_SANDBOX=", "CURSOR_AGENT=", "GEMINI_CLI=", "OPENCODE=", "REPL_ID="},
		"agent-env": {"CLAUDECODE=1"},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(bin, "version", "--output=json")
			cmd.Env = append(cmd.Environ(), "XDG_CACHE_HOME="+cacheHome)
			cmd.Env = append(cmd.Env, extraEnv...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				var ee *exec.ExitError
				if !errors.As(err, &ee) {
					t.Fatalf("jira version: %v", err)
				}
				t.Fatalf("jira version exited %d\nstderr=%s", ee.ExitCode(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty (no update hint in machine output)", stderr.String())
			}
			lines := bytes.Split(bytes.TrimRight(stdout.Bytes(), "\n"), []byte("\n"))
			if len(lines) != 1 {
				t.Fatalf("stdout has %d lines, want exactly the envelope:\n%s", len(lines), stdout.String())
			}
			var env map[string]any
			if err := json.Unmarshal(lines[0], &env); err != nil {
				t.Errorf("stdout is not a JSON envelope: %v\nstdout=%s", err, stdout.String())
			}
		})
	}
}
