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

func TestMissingNonLocalCredentialsReturnStructuredAuthError(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := jiraConfigWithProfile(t, "missingcreds", "https://jira.example.invalid")
	cmd := exec.Command(bin, "--config", cfg, "--profile", "missingcreds", "--json", "issue", "list")
	cmd.Env = append(cmd.Environ(), "JIRA_TOKEN_MISSINGCREDS=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("missing non-local credentials succeeded instead of auth error:\nstdout=%s", stdout.String())
	}
	// clog diagnostic on stderr must mention "credential".
	stderrLow := strings.ToLower(stderr.String())
	if !strings.Contains(stderrLow, "err") || !strings.Contains(stderrLow, "credential") {
		t.Fatalf("auth failure did not emit clog diagnostic on stderr:\nstderr=%s", stderr.String())
	}
	// --json path must deliver a JSON envelope on stdout.
	var env map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%s", jsonErr, stdout.String())
	}
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty on auth failure:\nstdout=%s", stdout.String())
	}
}

func jiraConfigWithProfile(t *testing.T, profile, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "` + profile + `"
queries_path = "` + filepath.ToSlash(t.TempDir()) + `/queries"

[[profiles]]
name = "` + profile + `"
base_url = "` + baseURL + `"
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
