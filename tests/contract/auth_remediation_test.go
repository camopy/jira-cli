package contract

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingNonLocalCredentialsReturnStructuredAuthError(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := jiraConfigWithProfile(t, "missingcreds", "https://jira.example.invalid")
	cmd := exec.Command(bin, "--config", cfg, "--profile", "missingcreds", "--output=json", "issue", "list")
	cmd.Env = append(cmd.Environ(), "JIRA_TOKEN_MISSINGCREDS=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("missing non-local credentials succeeded instead of auth error:\nstdout=%s", stdout.String())
	}
	// Machine mode: the failure envelope on stdout carries the credential
	// diagnostic; stderr stays free of a human clog line.
	var env map[string]any
	decodeErrorEnvelopeFromStdout(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, &env)
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty on auth failure:\nstdout=%s", stdout.String())
	}
	if !strings.Contains(strings.ToLower(stdout.String()), "credential") {
		t.Fatalf("envelope did not mention the credential remediation:\nstdout=%s", stdout.String())
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
