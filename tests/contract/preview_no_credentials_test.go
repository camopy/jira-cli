// Dry-run and preview paths are local-only: they must validate input and
// render a preview without ever constructing a credentialed Jira client.
// Resolving credentials for a command that will not touch the network is
// both wasteful and a failure mode — a locked keyring or an unauthenticated
// 1Password backend would break a purely local preview.
//
// These tests pin that contract with a 1Password-backed profile and no
// 1Password auth source in the environment: resolving a credential would
// fail. A dry-run path that resolves credentials therefore fails the
// command; a genuine local preview succeeds without touching the backend.
package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

// noOnePasswordAuthEnv returns an environment with no 1Password auth source:
// the service-account token is explicitly cleared. A 1Password-backed profile
// then cannot resolve a credential, so any command that resolves credentials
// fails — which is exactly what a dry-run path must NOT do.
func noOnePasswordAuthEnv() []string {
	return append(os.Environ(), "OP_SERVICE_ACCOUNT_TOKEN=")
}

func TestIssueCreateDryRunDoesNotResolveCredentials(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := onePasswordProfileConfig(t)

	payload := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(payload, []byte(`{"project_key":"PROJ","issue_type":"Task","summary":"Hello"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := exec.Command(bin, "--config", cfg, "issue", "create",
		"--dry-run", "--no-input", "--json-input", payload, "--output=json")
	c.Env = noOnePasswordAuthEnv()
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("issue create --dry-run resolved credentials and failed: %v\n%s", err, out)
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
	bin := buildJiraBinary(t)
	cfg := onePasswordProfileConfig(t)

	c := exec.Command(bin, "--config", cfg, "issue", "edit", "PROJ-1",
		"--summary", "renamed", "--dry-run", "--output=json")
	c.Env = noOnePasswordAuthEnv()
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("issue edit --dry-run resolved credentials and failed: %v\n%s", err, out)
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
