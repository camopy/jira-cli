package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/config"
)

// A headless env-backend login verifies the token read from the profile's
// JIRA_TOKEN_* variable, persists metadata only, and reports which variable
// the credential comes from — nothing is written to any secret store.
func TestAuthLoginEnvBackendVerifiesFromVariable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, ok := r.BasicAuth()
		if !ok || pass != "env-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"acc-7","displayName":"Ada Lovelace","emailAddress":"ada@example.com"}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TOKEN_DEFAULT", "env-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	stdout, stderr, err := runAuthLoginInProcess(
		t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "env",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}
	data := envelopeData(t, stdout)
	if stored, _ := data["stored_secret"].(bool); stored {
		t.Fatalf("env-backend login reported a stored secret:\n%s", stdout)
	}
	if verified, _ := data["verified"].(bool); !verified {
		t.Fatalf("env-backend login did not verify the variable's token:\n%s", stdout)
	}
	if data["credential_env"] != "JIRA_TOKEN_DEFAULT" {
		t.Fatalf("credential_env = %v, want JIRA_TOKEN_DEFAULT:\n%s", data["credential_env"], stdout)
	}
	if data["secret_backend"] != string(config.SecretBackendEnv) {
		t.Fatalf("secret_backend = %v, want env:\n%s", data["secret_backend"], stdout)
	}

	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if !strings.Contains(string(content), `secret_backend = "env"`) {
		t.Fatalf("profile did not persist the env backend:\n%s", content)
	}
	if strings.Contains(string(content), "env-token") {
		t.Fatalf("config file leaked the token:\n%s", content)
	}
}

// The env backend stores nothing, so supplying a secret to store is a
// contradiction the login rejects instead of silently dropping the value.
func TestAuthLoginEnvBackendRejectsSuppliedSecret(t *testing.T) {
	t.Setenv("JIRA_TEST_TOKEN", "some-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, _, err := runAuthLoginInProcess(
		t, cli.ModeJSON, configPath,
		"--base-url", "https://acme.atlassian.net",
		"--email", "ada@example.com",
		"--backend", "env",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatal("auth login accepted a supplied secret for the env backend")
	}
	if !strings.Contains(err.Error(), "JIRA_TOKEN_*") {
		t.Fatalf("rejection should explain where the env backend reads from: %v", err)
	}
	if content, readErr := os.ReadFile(configPath); readErr == nil && strings.Contains(string(content), "acme.atlassian.net") {
		t.Fatalf("rejected login persisted the profile:\n%s", content)
	}
}

// With the variable unset, the login still succeeds — the metadata is valid
// — but warns that nothing can authenticate until the variable is exported.
func TestAuthLoginEnvBackendWarnsWhenVariableUnset(t *testing.T) {
	t.Setenv("JIRA_TOKEN_DEFAULT", "")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	stdout, _, err := runAuthLoginInProcess(
		t, cli.ModeJSON, configPath,
		"--base-url", "https://acme.atlassian.net",
		"--email", "ada@example.com",
		"--backend", "env",
	)
	if err != nil {
		t.Fatalf("auth login error = %v", err)
	}
	if !strings.Contains(stdout, "JIRA_TOKEN_DEFAULT") {
		t.Fatalf("login envelope should warn about the unset variable:\n%s", stdout)
	}
	data := envelopeData(t, stdout)
	if verified, _ := data["verified"].(bool); verified {
		t.Fatalf("nothing was verifiable, yet the login claims verified:\n%s", stdout)
	}
}

// Re-pointing a keyring profile at the env backend is a metadata change, not
// a credential relocation, so login allows it without `auth migrate`.
func TestAuthLoginAllowsSwitchToEnvBackend(t *testing.T) {
	t.Setenv("JIRA_TOKEN_WORK", "env-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://acme.atlassian.net"
auth_type = "token"
email = "ada@example.com"
secret_backend = "keyring"
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	stdout, stderr, err := runAuthLoginInProcess(
		t, cli.ModeJSON, configPath,
		"--profile-name", "work",
		"--base-url", "https://acme.atlassian.net",
		"--backend", "env",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}
	if data := envelopeData(t, stdout); data["secret_backend"] != string(config.SecretBackendEnv) {
		t.Fatalf("secret_backend = %v, want env:\n%s", data["secret_backend"], stdout)
	}
}

// Switching between two storing backends without a fresh token still refuses
// and points at `auth migrate` — the env exemption must not have loosened it.
func TestAuthLoginStillRefusesStoringBackendSwitch(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://acme.atlassian.net"
auth_type = "token"
email = "ada@example.com"
secret_backend = "keyring"
vault = "Engineering"
item = "jira-cli-work"
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, _, err := runAuthLoginInProcess(
		t, cli.ModeJSON, configPath,
		"--profile-name", "work",
		"--base-url", "https://acme.atlassian.net",
		"--backend", "1password",
		"--vault", "Engineering",
		"--item", "jira-cli-work",
	)
	if err == nil {
		t.Fatal("auth login switched storing backends without a migration")
	}
	if !strings.Contains(err.Error(), "auth migrate") {
		t.Fatalf("refusal should point at auth migrate: %v", err)
	}
}

// A base URL that names no Atlassian tenant must fail login verification
// with the site-not-found identity — naming the host, not pretending the
// site is "temporarily unavailable" (Atlassian's misleading 404 body) —
// and persist nothing.
func TestAuthLoginReportsNonexistentSite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Atl-Missing-Tcs", "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorCode":"OTHER","errorMessage":"Site temporarily unavailable"}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "some-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, _, err := runAuthLoginInProcess(
		t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatal("auth login succeeded against a nonexistent site")
	}
	if !strings.Contains(err.Error(), "no Atlassian site exists at") {
		t.Fatalf("error should say the site does not exist: %v", err)
	}
	if strings.Contains(err.Error(), "temporarily unavailable") {
		t.Fatalf("error kept Atlassian's misleading outage wording: %v", err)
	}
	if content, readErr := os.ReadFile(configPath); readErr == nil && strings.Contains(string(content), srv.URL) {
		t.Fatalf("failed login persisted the profile:\n%s", content)
	}
}
