package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNonDryRunMutationsRequireConfiguredJiraClient(t *testing.T) {
	cfg := emptyBaseURLConfig(t)
	bin := buildJiraBinary(t)
	editPayload := writeIssueEditPayload(t)
	createPayload := writeIssueCreatePayload(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"issue create", []string{"issue", "create", "--no-input", "--json-input", createPayload, "--json"}},
		{"issue edit", []string{"issue", "edit", "PROJ-1", "--no-input", "--json-input", editPayload, "--json"}},
		{"issue comment", []string{"issue", "comment", "PROJ-1", "--body-markdown", "hello", "--no-input", "--json"}},
		{"worklog add", []string{"worklog", "add", "PROJ-1", "--time-spent", "45m", "--no-input", "--json"}},
		{"epic add", []string{"epic", "add", "PROJ-1", "EPIC-1", "--no-input", "--json"}},
		{"epic remove", []string{"epic", "remove", "PROJ-1", "--no-input", "--json"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, append([]string{"--config", cfg}, tc.args...)...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err == nil {
				t.Fatalf("%s succeeded without configured Jira client:\nstdout=%s", tc.name, stdout.String())
			}
			// clog diagnostic on stderr must mention "base URL".
			if !strings.Contains(strings.ToLower(stderr.String()), "err") || !strings.Contains(stderr.String(), "base URL") {
				t.Fatalf("%s stderr missing base URL clog diagnostic:\nstderr=%s", tc.name, stderr.String())
			}
			// --json path must deliver a JSON envelope on stdout.
			var env map[string]any
			if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
				t.Fatalf("%s stdout is not valid JSON: %v\nstdout=%s", tc.name, jsonErr, stdout.String())
			}
			errs, _ := env["errors"].([]any)
			if len(errs) == 0 {
				t.Fatalf("%s envelope.errors is empty:\nstdout=%s", tc.name, stdout.String())
			}
		})
	}
}

func writeIssueEditPayload(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "edit.json")
	if err := os.WriteFile(path, []byte(`{"fields":{"summary":"x"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func writeIssueCreatePayload(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(path, []byte(`{"project_key":"PROJ","issue_type":"Task","summary":"hello"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func emptyBaseURLConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "default"

[[profiles]]
name = "default"
auth_type = "token"
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
