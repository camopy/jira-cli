package contract

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchCommandsExposeInlineAndSavedJQL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected search request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"isLast":true,"maxResults":50,"issues":[{"key":"PROJ-1","fields":{"summary":"Search result","status":{"name":"To Do"},"updated":"2026-05-03T10:00:00Z"}}]}`))
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "search", "jql", "project = PROJ")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search jql error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"jql": "project = PROJ"`) || !strings.Contains(string(out), `"source": "inline"`) {
		t.Fatalf("search jql output = %s", out)
	}

	queryDir := filepath.Join(t.TempDir(), "queries")
	if err := os.MkdirAll(queryDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	query := "---\nname: My Bugs\ndescription: Active bugs\nproject: PROJ\n---\nassignee = currentUser()"
	if err := os.WriteFile(filepath.Join(queryDir, "mine.jql"), []byte(query), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "config", "set", "queries_path", queryDir)
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("config set queries_path error = %v\n%s", err, out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "search", "saved", "mine")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search saved error = %v\n%s", err, out)
	}
	for _, want := range []string{`"source": "saved"`, `"name": "My Bugs"`, `"jql": "assignee = currentUser()"`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("search saved output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigThemeCommandShowsAndUpdatesTheme(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	initConfig(t, cfg)

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "config", "theme", "--name", "primer", "--path", "/tmp/theme.toml")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config theme set error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"name": "primer"`) || !strings.Contains(string(out), `"path": "/tmp/theme.toml"`) {
		t.Fatalf("config theme set output = %s", out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "config", "theme")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config theme show error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"name": "primer"`) || !strings.Contains(string(out), `"path": "/tmp/theme.toml"`) {
		t.Fatalf("config theme show output = %s", out)
	}
}

func TestAuthRefreshAndMigrateReportConcreteState(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	initConfig(t, cfg)

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "auth", "refresh")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth refresh error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"refreshed": false`) || !strings.Contains(string(out), `"reason"`) {
		t.Fatalf("auth refresh output = %s", out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "auth", "migrate", "--backend", "1password", "--dry-run")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth migrate error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"target_backend": "1password"`) || !strings.Contains(string(out), `"dry_run": true`) {
		t.Fatalf("auth migrate output = %s", out)
	}
}

func initConfig(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command(
		"go", "run", "../../cmd/jira",
		"--config", path,
		"config", "init",
		"--no-input",
		"--profile", "default",
		"--base-url", "https://company.atlassian.net",
		"--auth-type", "token",
		"--email", "dev@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("config init error = %v\n%s", err, out)
	}
}
