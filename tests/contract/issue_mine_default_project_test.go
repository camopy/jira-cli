package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mineDefaultProjectConfig(t *testing.T, defaultProject string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	body := `default_profile = "default"
queries_path = "` + dir + `/queries"

[[profiles]]
name = "default"
base_url = "https://acme.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
`
	if defaultProject != "" {
		body += "default_project = \"" + defaultProject + "\"\n"
	}
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	return cfg
}

func mineJQL(t *testing.T, cfg string, extraArgs ...string) string {
	t.Helper()
	args := append([]string{"run", "../../cmd/jira", "--config", cfg, "issue", "mine", "--as-jql", "--output=json"}, extraArgs...)
	out, err := exec.Command("go", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("issue mine --as-jql error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			JQL string `json:"jql"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue mine output is not JSON: %v\n%s", err, out)
	}
	return env.Data.JQL
}

// issue mine scopes to the profile default_project, --project overrides it, and
// no default leaves the query unscoped.
func TestIssueMineScopesToDefaultProject(t *testing.T) {
	got := mineJQL(t, mineDefaultProjectConfig(t, "ACME"))
	if got != "project = ACME AND assignee = currentUser() ORDER BY updated DESC" {
		t.Fatalf("mine jql = %q", got)
	}

	got = mineJQL(t, mineDefaultProjectConfig(t, "ACME"), "--project", "OTHER")
	if got != "project = OTHER AND assignee = currentUser() ORDER BY updated DESC" {
		t.Fatalf("mine --project override jql = %q", got)
	}

	got = mineJQL(t, mineDefaultProjectConfig(t, ""))
	if got != "assignee = currentUser() ORDER BY updated DESC" {
		t.Fatalf("mine without default_project jql = %q", got)
	}
}
