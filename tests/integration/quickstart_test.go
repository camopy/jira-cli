package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestQuickstartCommandsWithFakeDefaults(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(`default_profile = "default"

[[profiles]]
name = "default"
auth_type = "token"
secret_backend = "keyring"
default_project = "PROJ"
default_issue_type = "Task"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	commands := [][]string{
		{"schema"},
		{"issue", "list", "--json"},
		// project_key + issue_type derive from profile defaults; summary supplied via flag.
		{"issue", "create", "--summary", "hello", "--dry-run", "--no-input", "--json"},
		{"worklog", "add", "PROJ-1", "--time-spent", "45m", "--dry-run", "--no-input"},
	}
	for _, args := range commands {
		cmdArgs := append([]string{"run", "../../cmd/jira", "--config", cfg}, args...)
		cmd := exec.Command("go", cmdArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jira %v error = %v\n%s", args, err, out)
		}
	}
}
