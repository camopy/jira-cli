package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func webConfig(t *testing.T) string {
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
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	return cfg
}

func webEnvelope(t *testing.T, cfg string, args ...string) (map[string]any, string) {
	t.Helper()
	full := append([]string{"run", "../../cmd/jira", "--config", cfg}, args...)
	out, err := exec.Command("go", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("jira %v error = %v\n%s", args, err, out)
	}
	var env struct {
		Meta struct {
			Command string `json:"command"`
		} `json:"meta"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	return env.Data, env.Meta.Command
}

// The --web affordances report the browser URL in the envelope and, in a
// non-interactive context (the test runner), never spawn a browser — opened is
// false. This keeps them safe in CI and agent shells.
func TestWebOpenReportsURLWithoutSpawning(t *testing.T) {
	cfg := webConfig(t)

	for _, tc := range []struct {
		name        string
		args        []string
		wantURL     string
		wantCommand string
	}{
		{"issue view --web", []string{"issue", "view", "KAN-1", "--web", "--output=json"}, "https://acme.atlassian.net/browse/KAN-1", "issue.view"},
		{"jira open", []string{"open", "KAN-1", "--output=json"}, "https://acme.atlassian.net/browse/KAN-1", "open"},
		{"search jql --web", []string{"search", "jql", "project = KAN", "--web", "--output=json"}, "https://acme.atlassian.net/issues/?jql=project+%3D+KAN", "search.jql"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, command := webEnvelope(t, cfg, tc.args...)
			if data["url"] != tc.wantURL {
				t.Fatalf("url = %#v, want %q", data["url"], tc.wantURL)
			}
			if data["opened"] != false {
				t.Fatalf("opened = %#v, want false in non-interactive context", data["opened"])
			}
			if command != tc.wantCommand {
				t.Fatalf("meta.command = %q, want %q", command, tc.wantCommand)
			}
		})
	}
}
