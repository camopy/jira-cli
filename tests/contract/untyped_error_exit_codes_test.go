// After the legacy substring classifier was retired, an untyped command
// error is always validation (exit 3): message text like "auth_type" or a
// "credential-env" flag name can no longer misroute it into auth/exit 1.
// These drive the real command paths that used to be misclassified and
// assert the exit code end-to-end.
package contract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func jiraConfigBadAuthType(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "default"

[[profiles]]
name = "default"
base_url = "https://x.atlassian.net"
auth_type = "bogus"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

// TestUntypedCommandErrorsExitValidation locks in the exit code for the
// three paths that previously read as auth/exit 1 because their message or
// a flag name contained a classifier trigger substring. Not parallel: the
// probe seam is package-level (touched by sibling tests).
func TestUntypedCommandErrorsExitValidation(t *testing.T) {
	scenarios := []struct {
		name string
		cfg  func(t *testing.T) string
		args []string
	}{
		// profile.go: unsupported auth_type — "auth_type" contains "auth".
		{"bad-auth_type-on-load", jiraConfigBadAuthType, []string{"issue", "view", "PROJ-1"}},
		// keys.go: invalid auth_type on `config set`.
		{
			"config-set-invalid-auth_type",
			func(t *testing.T) string { return jiraConfig(t, "https://x.atlassian.net") },
			[]string{"config", "set", "profiles.default.auth_type", "bogus"},
		},
		// cobra flag-mutex — the flag name "credential-env" contains
		// "credential".
		{
			"flag-mutex-credential-env",
			func(t *testing.T) string { return jiraConfig(t, "https://x.atlassian.net") },
			[]string{"auth", "login", "--no-input", "--base-url", "https://x.atlassian.net", "--email", "a@b.co", "--secret-stdin", "--credential-env", "JIRA_X"},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			cfg := sc.cfg(t)
			err := runCommandInProcess(t, append([]string{"--config", cfg}, sc.args...)...)
			if err == nil {
				t.Fatalf("command %v succeeded; want a validation error", sc.args)
			}
			mapped := cli.MapError(err)
			if mapped.Code != "validation_failed" {
				t.Errorf("code = %q, want validation_failed (error: %v)", mapped.Code, err)
			}
			if got := cli.ExitCode(mapped); got != 3 {
				t.Errorf("exit = %d, want 3 (error: %v)", got, err)
			}
		})
	}
}
