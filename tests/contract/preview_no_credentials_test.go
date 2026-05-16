// Dry-run and preview paths are local-only: they must validate input and
// render a preview without ever constructing a credentialed Jira client.
// Resolving credentials for a command that will not touch the network is
// both wasteful and a failure mode — a locked keyring or an offline
// 1Password backend would break a purely local preview.
//
// These tests pin that contract with a fake `op` binary that records any
// invocation and exits non-zero. If a dry-run path resolves credentials
// through the 1Password backend, the fake `op` runs: the sentinel file
// appears and/or the command fails. A genuine local preview never calls
// it.
package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// onePasswordProfileConfig writes a config whose active profile has a
// real base URL and a 1Password secret backend, so any client
// construction must resolve a credential through `op`.
func onePasswordProfileConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://company.atlassian.net"
auth_type = "token"
secret_backend = "1password"
vault = "Engineering"
item = "jira-cli-work"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

// failingOnePasswordEnv installs a fake `op` on PATH that records the
// fact that it was called (by writing sentinelPath) and then exits
// non-zero, simulating an unavailable credential backend. It returns the
// environment slice to attach to the command and the sentinel path.
func failingOnePasswordEnv(t *testing.T) (env []string, sentinel string) {
	t.Helper()
	binDir := t.TempDir()
	sentinel = filepath.Join(t.TempDir(), "op-was-called")
	op := filepath.Join(binDir, "op")
	script := "#!/bin/sh\ntouch " + shellQuote(sentinel) + "\necho 'op: not signed in' >&2\nexit 1\n"
	if err := os.WriteFile(op, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(fake op) error = %v", err)
	}
	env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OP_SERVICE_ACCOUNT_TOKEN=",
	)
	return env, sentinel
}

func TestIssueCreateDryRunDoesNotResolveCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake op fixture is Unix-specific")
	}
	bin := buildJiraBinary(t)
	cfg := onePasswordProfileConfig(t)
	env, sentinel := failingOnePasswordEnv(t)

	payload := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(payload, []byte(`{"project_key":"PROJ","issue_type":"Task","summary":"Hello"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := exec.Command(bin, "--config", cfg, "issue", "create",
		"--dry-run", "--no-input", "--json-input", payload, "--json")
	c.Env = env
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("issue create --dry-run resolved credentials and failed: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(sentinel); statErr == nil {
		t.Fatalf("issue create --dry-run invoked the 1Password credential backend:\n%s", out)
	}
	var env2 map[string]any
	if jsonErr := json.Unmarshal(out, &env2); jsonErr != nil {
		t.Fatalf("issue create --dry-run output is not JSON: %v\n%s", jsonErr, out)
	}
	data, _ := env2["data"].(map[string]any)
	if dryRun, _ := data["dry_run"].(bool); !dryRun {
		t.Fatalf("issue create --dry-run envelope missing dry_run: %+v", data)
	}
}

func TestIssueEditDryRunDoesNotResolveCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake op fixture is Unix-specific")
	}
	bin := buildJiraBinary(t)
	cfg := onePasswordProfileConfig(t)
	env, sentinel := failingOnePasswordEnv(t)

	c := exec.Command(bin, "--config", cfg, "issue", "edit", "PROJ-1",
		"--summary", "renamed", "--dry-run", "--json")
	c.Env = env
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("issue edit --dry-run resolved credentials and failed: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(sentinel); statErr == nil {
		t.Fatalf("issue edit --dry-run invoked the 1Password credential backend:\n%s", out)
	}
	var env2 map[string]any
	if jsonErr := json.Unmarshal(out, &env2); jsonErr != nil {
		t.Fatalf("issue edit --dry-run output is not JSON: %v\n%s", jsonErr, out)
	}
	data, _ := env2["data"].(map[string]any)
	if dryRun, _ := data["dry_run"].(bool); !dryRun {
		t.Fatalf("issue edit --dry-run envelope missing dry_run: %+v", data)
	}
}
