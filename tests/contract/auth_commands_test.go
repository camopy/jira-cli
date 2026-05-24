package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAuthCommandsAreWired(t *testing.T) {
	for _, args := range [][]string{
		{"auth", "status"},
		{"auth", "refresh"},
		{"auth", "token"},
	} {
		cmd := exec.Command("go", append([]string{"run", "../../cmd/jira"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v error = %v\n%s", args, err, out)
		}
		if strings.TrimSpace(string(out)) == "" {
			t.Fatalf("%v produced no output", args)
		}
	}
}

func TestAuthLoginSwitchAndLogoutMetadataOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cmd := exec.Command(
		"go", "run", "../../cmd/jira",
		"--config", path,
		"auth", "login",
		"--no-input",
		"--profile-name", "work",
		"--base-url", "https://company.atlassian.net",
		"--email", "dev@example.com",
		"--backend", "1password",
		"--vault", "Engineering",
		"--item", "jira-cli-work",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login error = %v\n%s", err, out)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, forbidden := range []string{"api-token-secret", "raw-password", "refresh_token_secret"} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("config leaked %q:\n%s", forbidden, content)
		}
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", path, "--output=json", "auth", "switch", "work")
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("auth switch error = %v\n%s", err, out)
	}
	if !envelopeHasKV(t, out, "active", "work") {
		t.Fatalf("auth switch output = %s", out)
	}

	if runtime.GOOS == "windows" {
		t.Skip("fake op fixture is Unix-specific")
	}
	binDir := t.TempDir()
	op := filepath.Join(binDir, "op")
	if err := os.WriteFile(op, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(fake op) error = %v", err)
	}
	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", path, "--output=json", "auth", "logout", "work")
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("auth logout error = %v\n%s", err, out)
	}
	if !envelopeHasKV(t, out, "profile", "work") {
		t.Fatalf("auth logout output = %s", out)
	}
}

func TestAuthLoginCanCollectMetadataWithoutPrompts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cmd := exec.Command(
		"go", "run", "../../cmd/jira",
		"--config", path,
		"auth", "login",
		"--no-input",
		"--profile-name", "work",
		"--base-url", "https://company.atlassian.net",
		"--email", "dev@example.com",
		"--backend", "keyring",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login error = %v\n%s", err, out)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{`default_profile = "work"`, `base_url = "https://company.atlassian.net"`, `email = "dev@example.com"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("interactive auth login config missing %q:\n%s", want, content)
		}
	}
}

func TestAuthLoginNoInputAcceptsMetadataJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	input := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(input, []byte(`{
  "profile_name": "json-work",
  "base_url": "https://company.atlassian.net",
  "backend": "1password",
  "onepassword_account": "my.1password.com",
  "vault": "Engineering",
  "item": "jira-cli-json-work"
}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", path, "auth", "login", "--no-input", "--json-input", input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login json-input error = %v\n%s", err, out)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{`name = "json-work"`, `auth_type = "token"`, `secret_backend = "1password"`, `onepassword_account = "my.1password.com"`, `vault = "Engineering"`, `item = "jira-cli-json-work"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("auth json-input config missing %q:\n%s", want, content)
		}
	}
}

// JIRA_DEFAULT_PROFILE still selects the active profile when no explicit
// --profile flag is passed: the explicit-lookup hardening must not break
// the env-driven default fallback.
func TestAuthRefreshHonoursEnvDefaultProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://work.atlassian.net"
auth_type = "token"
secret_backend = "keyring"

[[profiles]]
name = "play"
base_url = "https://play.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", path, "--output=json", "auth", "refresh")
	cmd.Env = append(os.Environ(), "JIRA_DEFAULT_PROFILE=play")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth refresh error = %v\n%s", err, out)
	}
	if !envelopeHasKV(t, out, "profile", "play") {
		t.Fatalf("auth refresh did not honor JIRA_DEFAULT_PROFILE:\n%s", out)
	}
}

func TestAuthRefreshReportsNoRefreshFlowForSupportedProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://company.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", path, "auth", "refresh")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth refresh error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"auth_type":"token"`) || !strings.Contains(string(out), `"refreshed":false`) {
		t.Fatalf("auth refresh output = %s", out)
	}
}
