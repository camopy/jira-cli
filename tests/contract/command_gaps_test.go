package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
	if !envelopeHasKV(t, out, "jql", "project = PROJ") || !envelopeHasKV(t, out, "source", "inline") {
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
	for _, kv := range []struct {
		key string
		val any
	}{
		{"source", "saved"},
		{"name", "My Bugs"},
		{"jql", "assignee = currentUser()"},
	} {
		if !envelopeHasKV(t, out, kv.key, kv.val) {
			t.Fatalf("search saved output missing %s=%v:\n%s", kv.key, kv.val, out)
		}
	}
}

func TestSearchCommandsRequestSummaryFieldsByDefaultAndFullOnDemand(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected search request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"PROJ-1","fields":{"summary":"Search result","status":{"name":"To Do"},"updated":"2026-05-03T10:00:00Z"}}]}`))
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	bin := buildJiraBinary(t)
	for _, args := range [][]string{
		{"--config", cfg, "--output=json", "search", "jql", "project = PROJ"},
		{"--config", cfg, "--output=json", "search", "jql", "project = PROJ", "--full"},
		{"--config", cfg, "--output=json", "search", "jql", "project = PROJ", "--fields", "key,summary"},
	} {
		cmd := exec.Command(bin, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("jira %v error = %v\n%s", args, err, out)
		}
	}
	if len(bodies) != 3 {
		t.Fatalf("request bodies len = %d, want 3", len(bodies))
	}
	assertFields := func(idx int, want []string) {
		t.Helper()
		raw, ok := bodies[idx]["fields"].([]any)
		if !ok {
			t.Fatalf("body %d fields missing or wrong type: %#v", idx, bodies[idx])
		}
		if len(raw) != len(want) {
			t.Fatalf("body %d fields len = %d, want %d: %#v", idx, len(raw), len(want), raw)
		}
		for i, field := range want {
			if raw[i] != field {
				t.Fatalf("body %d fields[%d] = %v, want %q: %#v", idx, i, raw[i], field, raw)
			}
		}
	}
	assertFields(0, []string{"key", "summary", "status", "assignee", "priority", "updated"})
	assertFields(1, []string{"*all"})
	assertFields(2, []string{"key", "summary"})
}

func TestTokenSearchCommandsEmitTokenPaginationMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected search request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"isLast":false,"nextPageToken":"next-token","issues":[{"key":"PROJ-1","fields":{"summary":"Search result","status":{"name":"To Do"},"updated":"2026-05-03T10:00:00Z"}}]}`))
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
	bin := buildJiraBinary(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"issue-list", []string{"--config", cfg, "--output=json", "issue", "list"}},
		{"search-jql", []string{"--config", cfg, "--output=json", "search", "jql", "project = PROJ"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("jira %v error = %v\n%s", tc.args, err, out)
			}
			var env struct {
				Meta struct {
					Pagination struct {
						StartAt    int    `json:"startAt"`
						MaxResults int    `json:"maxResults"`
						Total      int    `json:"total"`
						IsLast     bool   `json:"isLast"`
						NextCursor string `json:"nextCursor"`
					} `json:"pagination"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(out, &env); err != nil {
				t.Fatalf("decode envelope: %v\n%s", err, out)
			}
			pagination := env.Meta.Pagination
			if pagination.StartAt != 0 || pagination.Total != 0 {
				t.Fatalf("offset pagination fields = startAt:%d total:%d, want zero for token search", pagination.StartAt, pagination.Total)
			}
			if pagination.MaxResults != 50 {
				t.Fatalf("maxResults = %d, want requested/default page size 50", pagination.MaxResults)
			}
			if pagination.IsLast || pagination.NextCursor != "next-token" {
				t.Fatalf("pagination final/cursor = (%v, %q), want false/next-token", pagination.IsLast, pagination.NextCursor)
			}
		})
	}
}

func TestConfigThemeCommandShowsAndUpdatesTheme(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	initConfig(t, cfg)

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "config", "theme", "--name", "dracula", "--path", "/tmp/theme.toml")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config theme set error = %v\n%s", err, out)
	}
	if !envelopeHasKV(t, out, "name", "dracula") || !envelopeHasKV(t, out, "path", "/tmp/theme.toml") {
		t.Fatalf("config theme set output = %s", out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "config", "theme")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config theme show error = %v\n%s", err, out)
	}
	if !envelopeHasKV(t, out, "name", "dracula") || !envelopeHasKV(t, out, "path", "/tmp/theme.toml") {
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
	if !envelopeHasKV(t, out, "refreshed", false) || !envelopeHasKey(t, out, "reason") {
		t.Fatalf("auth refresh output = %s", out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--output=json", "auth", "migrate", "--backend", "1password", "--dry-run")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth migrate error = %v\n%s", err, out)
	}
	if !envelopeHasKV(t, out, "target_backend", "1password") || !envelopeHasKV(t, out, "dry_run", true) {
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
