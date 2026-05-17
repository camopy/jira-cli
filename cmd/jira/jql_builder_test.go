package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestJQLBuilderBuildsCommonDeveloperFilters(t *testing.T) {
	query, err := jqlBuildOptions{
		Projects:   []string{"PROJ"},
		Assignee:   "me",
		Epics:      []string{"EPIC-1"},
		Statuses:   []string{"In Progress", "To Do"},
		Priorities: []string{"High"},
		Labels:     []string{"backend"},
		OrderBy:    "updated",
		Descending: true,
	}.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := `project = PROJ AND assignee = currentUser() AND parent = EPIC-1 AND status in ("In Progress", "To Do") AND priority = High AND labels = backend ORDER BY updated DESC`
	if query != want {
		t.Fatalf("Build() = %q, want %q", query, want)
	}
}

func TestJQLBuilderDefaultsToBoundedIssueList(t *testing.T) {
	query, err := (jqlBuildOptions{}).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if query != jira.DefaultIssueListJQL {
		t.Fatalf("Build() = %q, want %q", query, jira.DefaultIssueListJQL)
	}
	if strings.Contains(query, "currentUser()") {
		t.Fatalf("default query should not imply --mine: %q", query)
	}
}

func TestJQLBuilderAppliesSortWithoutAdditionalFilters(t *testing.T) {
	query, err := (jqlBuildOptions{OrderBy: "status", Descending: true}).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := "updated >= -365d ORDER BY status DESC"
	if query != want {
		t.Fatalf("Build() = %q, want %q", query, want)
	}
}

func TestJQLBuilderValidatesSortWithoutAdditionalFilters(t *testing.T) {
	_, err := (jqlBuildOptions{OrderBy: "updated DESC; DROP"}).Build()
	if err == nil {
		t.Fatal("Build() accepted unsafe order-by field")
	}
}

func TestIssueListJQLCombinesRawJQLWithBuilderFilters(t *testing.T) {
	query, err := issueListJQL("project = RAW", jqlBuildOptions{Projects: []string{"BUILT"}})
	if err != nil {
		t.Fatalf("issueListJQL() error = %v", err)
	}
	if query != `project = BUILT AND (project = RAW)` {
		t.Fatalf("issueListJQL() = %q", query)
	}
}

func TestIssueListJQLLeavesRawJQLAloneWithoutImplicitFilters(t *testing.T) {
	query, err := issueListJQL("project = RAW ORDER BY created DESC", jqlBuildOptions{})
	if err != nil {
		t.Fatalf("issueListJQL() error = %v", err)
	}
	if query != "project = RAW ORDER BY created DESC" {
		t.Fatalf("issueListJQL() = %q", query)
	}
}

func TestIssueMineJQLCombinesRawJQLWithAssignee(t *testing.T) {
	query, err := issueListJQL("status = Open OR priority = High ORDER BY created DESC", jqlBuildOptions{Assignee: "me"})
	if err != nil {
		t.Fatalf("issueListJQL() error = %v", err)
	}
	want := `assignee = currentUser() AND (status = Open OR priority = High) ORDER BY created DESC`
	if query != want {
		t.Fatalf("issueListJQL() = %q, want %q", query, want)
	}
}

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

	cmd := exec.Command("go", "run", ".", "--config", cfg, "issue", "list", "--as-jql", "--output=json")
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

	cmd := exec.Command("go", "run", ".", "--config", cfg, "issue", "list", "--jql", "project = CUSTOM", "--as-jql", "--output=json")
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
	cmd := exec.Command("go", "run", ".", "issue", "list", "--project", "PROJ", "--assignee", "me", "--status", "In Progress", "--as-jql", "--output=json")
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
