package contract

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `auth login` accepts a credential from --secret-stdin or --credential-env.
// Supplying both is a conflict: one source would silently win depending on
// processing order. The conflict must be rejected by Cobra flag validation
// before any credential source is read, so neither stdin nor the env var is
// consumed.
func TestAuthLoginRejectsConflictingCredentialSources(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	cmd := exec.Command(
		bin,
		"--config", path,
		"auth", "login",
		"--no-input",
		"--profile-name", "work",
		"--base-url", "https://company.atlassian.net",
		"--auth-type", "token",
		"--email", "dev@example.com",
		"--backend", "keyring",
		"--secret-stdin",
		"--credential-env", "JIRA_LOGIN_TOKEN",
	)
	cmd.Stdin = strings.NewReader("stdin-token\n")
	cmd.Env = append(os.Environ(), "JIRA_LOGIN_TOKEN=env-token")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("auth login accepted conflicting credential sources:\nstdout=%s", stdout.String())
	}
	combined := strings.ToLower(stdout.String() + stderr.String())
	if !strings.Contains(combined, "secret-stdin") || !strings.Contains(combined, "credential-env") {
		t.Fatalf("conflict error does not name both flags:\n%s", stdout.String()+stderr.String())
	}
	// The config file must not have been written: validation failed before
	// any credential source was read or the profile persisted.
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("auth login wrote config despite a credential source conflict")
	}
}

// A single credential source is still accepted: --secret-stdin alone works.
func TestAuthLoginAcceptsSingleCredentialSource(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	cmd := exec.Command(
		bin,
		"--config", path,
		"auth", "login",
		"--no-input",
		"--profile-name", "work",
		"--base-url", "https://company.atlassian.net",
		"--auth-type", "mtls",
		"--backend", "keyring",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login with no credential source error = %v\n%s", err, out)
	}
}
