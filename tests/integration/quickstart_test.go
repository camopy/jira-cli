package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// Build once and execute the binary per command: `go run` per command
	// compiles the CLI repeatedly, and under a parallel CI run (lint and
	// security tasks contending for CPU and the Go build locks) four
	// compiles overran the test timeout.
	bin := filepath.Join(t.TempDir(), "jira")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "../../cmd/jira")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v\n%s", err, out)
	}
	commands := [][]string{
		{"agent", "schema"},
		{"issue", "list", "--output=json"},
		// project_key + issue_type derive from profile defaults; summary supplied via flag.
		{"issue", "create", "--summary", "hello", "--dry-run", "--no-input", "--output=json"},
		{"worklog", "add", "PROJ-1", "--time-spent", "45m", "--dry-run", "--no-input"},
	}
	for _, args := range commands {
		cmd := exec.Command(bin, append([]string{"--config", cfg}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jira %v error = %v\n%s", args, err, out)
		}
	}
}
