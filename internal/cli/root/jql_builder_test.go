package root

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueListAsJQLUsesProfileDefaultProject(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(`
default_profile = "default"
queries_path = "`+dir+`/queries"

[[profiles]]
name = "default"
base_url = ""
auth_type = "token"
default_project = "SAM1"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	cmd := exec.Command("go", "run", "github.com/matcra587/jira-cli/cmd/jira", "--config", cfg, "issue", "list", "--as-jql", "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --as-jql error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			JQL string `json:"jql"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue list --as-jql output is not JSON: %v\n%s", err, out)
	}
	if !strings.Contains(env.Data.JQL, "project = SAM1") || strings.Contains(env.Data.JQL, "currentUser()") {
		t.Fatalf("issue list default JQL = %q", env.Data.JQL)
	}
}

func TestIssueListAsJQLRawQueryIgnoresProfileDefaultProject(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(`
default_profile = "default"
queries_path = "`+dir+`/queries"

[[profiles]]
name = "default"
base_url = ""
auth_type = "token"
default_project = "SAM1"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	cmd := exec.Command("go", "run", "github.com/matcra587/jira-cli/cmd/jira", "--config", cfg, "issue", "list", "--jql", "project = CUSTOM", "--as-jql", "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --as-jql error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			JQL string `json:"jql"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue list --as-jql output is not JSON: %v\n%s", err, out)
	}
	if env.Data.JQL != "project = CUSTOM" {
		t.Fatalf("issue list raw JQL = %q, want project = CUSTOM", env.Data.JQL)
	}
}

func TestIssueListAsJQLPrintsBuiltQueryWithoutCallingJira(t *testing.T) {
	cmd := exec.Command("go", "run", "github.com/matcra587/jira-cli/cmd/jira", "issue", "list", "--project", "PROJ", "--assignee", "me", "--status", "In Progress", "--as-jql", "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue list --as-jql error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			JQL string `json:"jql"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue list --as-jql output is not JSON: %v\n%s", err, out)
	}
	if env.Data.JQL == "" {
		t.Fatalf("issue list --as-jql missing jql: %s", out)
	}
	for _, want := range []string{"project = PROJ", "assignee = currentUser()", `status = "In Progress"`} {
		if !strings.Contains(env.Data.JQL, want) {
			t.Fatalf("issue list --as-jql missing %q in %q", want, env.Data.JQL)
		}
	}
}
