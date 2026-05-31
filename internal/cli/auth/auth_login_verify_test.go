package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gechr/clog"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// runAuthLoginInProcess executes `auth login` against an in-process command
// tree with a mocked keyring and clog output captured to a buffer, so the
// full RunE — credential resolution, verification, storage, and the envelope
// — is exercised without touching the real OS keyring, the real config, or
// the real terminal.
func runAuthLoginInProcess(t *testing.T, mode cli.Mode, configPath string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	keyring.MockInit()

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	clog.SetOutput(clog.NewOutput(errBuf, clog.ColorNever))
	t.Cleanup(func() { clog.SetOutput(clog.NewOutput(os.Stderr, clog.ColorAuto)) })

	root := &cobra.Command{Use: "jira"}
	pf := root.PersistentFlags()
	pf.String("profile", "default", "")
	pf.String("config", configPath, "")
	pf.String("output", "auto", "")
	pf.Bool("no-input", true, "")
	pf.String("color", "never", "")
	pf.BoolP("debug", "d", false, "")
	pf.BoolP("interactive", "i", false, "")
	root.AddCommand(authLoginCommand())
	root.SetOut(outBuf)
	root.SetErr(errBuf)

	ctx := cmdutil.WithDetector(context.Background(), cli.Detection{Mode: mode, IsTTY: mode == cli.ModePlain})
	ctx = cmdutil.WithCredentialWarnSink(ctx)

	root.SetArgs(append([]string{"login"}, args...))
	err = root.ExecuteContext(ctx)
	return outBuf.String(), errBuf.String(), err
}

// envelopeData parses the JSON envelope on stdout and returns its data object.
func envelopeData(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, stdout)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope has no data object:\n%s", stdout)
	}
	return data
}

// keyringTokenFor reports the token stored for a keyring-backed profile, or
// the not-found error when no credential was persisted.
func keyringTokenFor(t *testing.T, name, baseURL string) (string, error) {
	t.Helper()
	ref, refErr := cmdutil.SecretRefFor(
		config.Profile{Name: name, BaseURL: config.NormalizeBaseURL(baseURL), SecretBackend: config.SecretBackendKeyring},
		config.SecretBackendKeyring,
	)
	if refErr != nil {
		t.Fatalf("SecretRefFor() error = %v", refErr)
	}
	return cmdutil.CredentialStoreFor(config.SecretBackendKeyring).Get(context.Background(), ref)
}

// A successful login verifies the token against /myself, stores it, persists
// the resolved accountId into the profile, and reports the authenticated
// identity in the envelope — so a freshly stored credential is known-good and
// `--assignee me` works immediately without a separate `auth whoami`.
func TestAuthLoginVerifiesAndPersistsIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "ada@example.com" || pass != "good-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"acc-99","displayName":"Ada Lovelace","emailAddress":"ada@example.com"}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "good-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	stdout, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "acc-99") || !strings.Contains(stdout, "Ada Lovelace") {
		t.Fatalf("login envelope missing verified identity:\n%s", stdout)
	}

	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if !strings.Contains(string(content), `account_id = "acc-99"`) {
		t.Fatalf("login did not persist the verified account_id:\n%s", content)
	}

	token, getErr := keyringTokenFor(t, "default", srv.URL)
	if getErr != nil {
		t.Fatalf("verified credential was not stored: %v", getErr)
	}
	if token != "good-token" {
		t.Fatalf("stored token = %q, want good-token", token)
	}
}

// A token Jira rejects must abort the login: nothing is stored and no
// identity is persisted, so a failed login never leaves an unusable
// credential behind in the keyring.
func TestAuthLoginAbortsWhenVerificationFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "wrong-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, _, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatal("auth login succeeded with a rejected token; want a verification error")
	}
	// A 401 means the email+token pair is wrong; say so plainly rather than
	// surfacing Jira's cryptic "Client must be authenticated" text.
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "invalid") || !strings.Contains(low, "token") || !strings.Contains(low, "email") {
		t.Fatalf("rejected-credential error should name the invalid token/email plainly: %v", err)
	}
	if strings.Contains(err.Error(), "Client must be authenticated") {
		t.Fatalf("error leaked Jira's raw 401 text instead of a clear message: %v", err)
	}

	if _, getErr := keyringTokenFor(t, "default", srv.URL); !errors.Is(getErr, config.ErrCredentialNotFound) {
		t.Fatalf("rejected token was stored anyway: err=%v", getErr)
	}
	if content, readErr := os.ReadFile(configPath); readErr == nil && strings.Contains(string(content), srv.URL) {
		t.Fatalf("failed login persisted the profile:\n%s", content)
	}
}

func TestAuthLoginRejectedCredentialMapsRemediationToHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "wrong-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, _, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatal("auth login succeeded with rejected credentials")
	}

	mapped := cli.MapError(err)
	if mapped.Hint == "" {
		t.Fatalf("mapped error hint is empty: %+v", mapped)
	}
	if strings.Contains(mapped.Message, "check the email") || strings.Contains(mapped.Message, "--skip-verify") {
		t.Fatalf("message still contains remediation instead of only symptom: %+v", mapped)
	}
	if !strings.Contains(mapped.Hint, "check the email") || !strings.Contains(mapped.Hint, "--skip-verify") {
		t.Fatalf("hint does not carry credential remediation: %+v", mapped)
	}
}

// A transient/server failure during verification is NOT a bad credential: it
// must not be reported as an invalid token, and it should point at
// --skip-verify so the user can store the credential and verify later.
func TestAuthLoginVerificationServerErrorIsNotReportedAsBadCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"errorMessage":"Site temporarily unavailable"}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "good-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, _, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatal("auth login succeeded despite an unreachable verification endpoint")
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "invalid") {
		t.Fatalf("a server-side verification failure must not be reported as invalid credentials: %v", err)
	}
	if !strings.Contains(low, "skip-verify") {
		t.Fatalf("non-credential verify failure should point at --skip-verify: %v", err)
	}
	if _, getErr := keyringTokenFor(t, "default", srv.URL); !errors.Is(getErr, config.ErrCredentialNotFound) {
		t.Fatalf("credential stored despite a failed verification: %v", getErr)
	}
}

func TestAuthLoginVerificationServerErrorMapsRemediationToHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"errorMessage":"Site temporarily unavailable"}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "good-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, _, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatal("auth login succeeded despite unreachable verification endpoint")
	}

	mapped := cli.MapError(err)
	if mapped.Hint == "" {
		t.Fatalf("mapped error hint is empty: %+v", mapped)
	}
	if strings.Contains(mapped.Message, "--skip-verify") || strings.Contains(mapped.Message, "retry") {
		t.Fatalf("message still contains remediation instead of only symptom: %+v", mapped)
	}
	if !strings.Contains(mapped.Hint, "retry") || !strings.Contains(mapped.Hint, "--skip-verify") {
		t.Fatalf("hint does not carry verification remediation: %+v", mapped)
	}
}

// --skip-verify stores the credential without contacting Jira, for offline
// setup or when the verification endpoint is unreachable. The server must
// never be hit.
func TestAuthLoginSkipVerifyDoesNotContactServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("--skip-verify contacted the Jira server")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "unchecked-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	stdout, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("auth login --skip-verify error = %v\nstderr=%s", err, stderr)
	}
	token, getErr := keyringTokenFor(t, "default", srv.URL)
	if getErr != nil {
		t.Fatalf("--skip-verify did not store the credential: %v", getErr)
	}
	if token != "unchecked-token" {
		t.Fatalf("stored token = %q, want unchecked-token", token)
	}
	// The envelope must distinguish a deliberate skip from a verification
	// that did not happen for some other reason: skip_verify is true and
	// verified is false.
	data := envelopeData(t, stdout)
	if data["skip_verify"] != true {
		t.Fatalf("envelope skip_verify = %v, want true:\n%s", data["skip_verify"], stdout)
	}
	if data["verified"] != false {
		t.Fatalf("envelope verified = %v, want false under --skip-verify:\n%s", data["verified"], stdout)
	}
}

// In headless mode the base URL is only validated by cfg.Validate, which runs
// after verification. A non-loopback http base URL must be rejected BEFORE the
// credential is sent, so the token is never leaked over cleartext.
func TestAuthLoginRejectsCleartextBaseURLBeforeSendingToken(t *testing.T) {
	t.Setenv("JIRA_TEST_TOKEN", "secret-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, _, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", "http://insecure.example.invalid",
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatal("auth login accepted a cleartext non-loopback base URL")
	}
	// The failure must be a base-URL validation error, not a network error
	// from having already dialed the cleartext host. Assert on the command's
	// own wrapper ("jira base URL") rather than ValidateBaseURL's wording, and
	// pair it with the keyring check below as the structural invariant.
	if !strings.Contains(strings.ToLower(err.Error()), "jira base url") {
		t.Fatalf("error is not a base-URL validation failure (token may have been sent): %v", err)
	}
	if _, getErr := keyringTokenFor(t, "default", "http://insecure.example.invalid"); !errors.Is(getErr, config.ErrCredentialNotFound) {
		t.Fatalf("credential stored despite an unsafe base URL: %v", getErr)
	}
}

// A profile name that cannot be encoded into a credential key must be rejected
// at input time — before the token is sent to Jira for verification and before
// anything is stored — not late at credential-store time. The verification
// server records whether it was contacted: an input-time rejection never dials
// it.
func TestAuthLoginRejectsUnsafeProfileNameBeforeSendingToken(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"acc-1","displayName":"Ada","emailAddress":"ada@example.com"}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "secret-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, _, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--profile-name", "Bad Name",
		"--base-url", srv.URL,
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatal("auth login accepted a namespace-unsafe profile name")
	}
	var ce *config.CredentialError
	if !errors.As(err, &ce) || ce.ErrCode != config.ErrorCodeCredentialNamespaceCollision {
		t.Fatalf("error is not a namespace-collision validation failure: %v", err)
	}
	// No network call means nothing was verified and nothing was stored: an
	// unsafe name cannot form a SecretRef, so the credential never reaches a
	// backend.
	if hit {
		t.Fatal("token was sent to Jira before the profile name was validated")
	}
}

// LoadOrInit seeds an empty "default" profile when it creates a config. A
// login that configures a different profile name on that fresh config must not
// leave that seed behind: an unconfigured default lingers as a phantom profile
// and makes `auth status` report it as unhealthy. End state for a fresh
// custom-name login is exactly one profile — the one that was configured.
func TestAuthLoginFreshConfigCustomNameLeavesNoSeededDefault(t *testing.T) {
	t.Setenv("JIRA_TEST_TOKEN", "tok")
	configPath := filepath.Join(t.TempDir(), "config.toml") // does not exist yet

	_, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--profile-name", "work",
		"--base-url", "https://company.atlassian.net",
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}
	cfg, loadErr := config.Load(config.WithPath(configPath))
	if loadErr != nil {
		t.Fatalf("Load() error = %v", loadErr)
	}
	var names []string
	for _, p := range cfg.Profiles {
		names = append(names, p.Name)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].Name != "work" {
		t.Fatalf("fresh custom-name login left profiles %v, want exactly [work]", names)
	}
	if cfg.DefaultProfile != "work" {
		t.Fatalf("default_profile = %q, want work", cfg.DefaultProfile)
	}
}

// The seed cleanup must only fire when a different name is configured: logging
// into the default profile on a fresh config keeps it, configured normally.
func TestAuthLoginFreshConfigDefaultNameKeepsProfile(t *testing.T) {
	t.Setenv("JIRA_TEST_TOKEN", "tok")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", "https://company.atlassian.net",
		"--email", "ada@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}
	cfg, loadErr := config.Load(config.WithPath(configPath))
	if loadErr != nil {
		t.Fatalf("Load() error = %v", loadErr)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].Name != "default" {
		t.Fatalf("fresh default login should yield exactly [default], got %d profiles", len(cfg.Profiles))
	}
	if cfg.Profiles[0].BaseURL == "" {
		t.Fatalf("default profile was not configured (empty base URL)")
	}
}

// An offline re-login (--skip-verify) that changes the account email must not
// keep the previous profile's account_id: it belongs to the old account and
// would mis-target `--assignee me`. It is dropped for `auth whoami --save` to
// repopulate.
func TestAuthLoginSkipVerifyClearsStaleAccountIDOnEmailChange(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	initial := `default_profile = "default"

[[profiles]]
  name = "default"
  base_url = "https://company.atlassian.net"
  auth_type = "token"
  email = "old@example.com"
  account_id = "old-account-id"
  secret_backend = "keyring"
  refresh_interval = 30
  timeout = 30
  workday_seconds = 28800
`
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("JIRA_TEST_TOKEN", "new-token")
	_, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", "https://company.atlassian.net",
		"--email", "new@example.com",
		"--credential-env", "JIRA_TEST_TOKEN",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}
	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if strings.Contains(string(content), "old-account-id") {
		t.Fatalf("stale account_id survived an offline email change:\n%s", content)
	}
}

// A Jira Cloud token is useless without the account email that forms the
// basic-auth username, so supplying a credential without an email must be
// refused before any network call or storage — even under --skip-verify.
func TestAuthLoginRequiresEmailWhenCredentialSupplied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("email validation should precede any verification call")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "orphan-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--profile-name", "noemail",
		"--base-url", srv.URL,
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err == nil {
		t.Fatalf("auth login stored a credential with no email; want a validation error\nstderr=%s", stderr)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "email") {
		t.Fatalf("error does not mention the missing email: %v", err)
	}
	if _, getErr := keyringTokenFor(t, "noemail", srv.URL); !errors.Is(getErr, config.ErrCredentialNotFound) {
		t.Fatalf("credential stored despite missing email: err=%v", getErr)
	}
}

// --json-input carries headless profile metadata; combined with an
// interactive login it would silently overwrite values AFTER the user has
// reviewed and confirmed them, so the combination must be rejected up front.
func TestAuthLoginRejectsJSONInputInInteractiveMode(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	jsonPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(jsonPath, []byte(`{"base_url":"https://other.atlassian.net"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, _, err := runAuthLoginInProcess(t, cli.ModePlain, configPath,
		"--no-input=false",
		"--json-input", jsonPath,
	)
	if err == nil {
		t.Fatal("interactive auth login accepted --json-input")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "json-input") || !strings.Contains(low, "no-input") {
		t.Fatalf("error does not name the conflicting flags: %v", err)
	}
}

// A headless re-login that supplies a credential but omits --email must
// inherit the persisted profile's email (via the profile merge) rather than
// failing the email-required guard.
func TestAuthLoginHeadlessReloginInheritsPersistedEmail(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	initial := `default_profile = "default"

[[profiles]]
  name = "default"
  base_url = "https://company.atlassian.net"
  auth_type = "token"
  email = "keep@example.com"
  secret_backend = "keyring"
  refresh_interval = 30
  timeout = 30
  workday_seconds = 28800
`
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("JIRA_TEST_TOKEN", "rotated-token")
	_, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", "https://company.atlassian.net",
		"--credential-env", "JIRA_TEST_TOKEN",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("headless re-login with a persisted email failed: %v\nstderr=%s", err, stderr)
	}
	token, getErr := keyringTokenFor(t, "default", "https://company.atlassian.net")
	if getErr != nil {
		t.Fatalf("credential not stored on inherited-email re-login: %v", getErr)
	}
	if token != "rotated-token" {
		t.Fatalf("stored token = %q, want rotated-token", token)
	}
}

// A 2xx /myself response that carries no accountId must not let a stale
// account_id survive an email change: without a fresh accountId for the new
// identity, the carried-over value would mis-target `--assignee me`.
func TestAuthLoginVerifiedWithoutAccountIDClearsStaleOnEmailChange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`)) // authenticated, but no accountId
	}))
	defer srv.Close()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	initial := `default_profile = "default"

[[profiles]]
  name = "default"
  base_url = "` + srv.URL + `"
  auth_type = "token"
  email = "old@example.com"
  account_id = "stale-account-id"
  secret_backend = "keyring"
  refresh_interval = 30
  timeout = 30
  workday_seconds = 28800
`
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("JIRA_TEST_TOKEN", "new-token")
	_, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", srv.URL,
		"--email", "new@example.com",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}
	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if strings.Contains(string(content), "stale-account-id") {
		t.Fatalf("stale account_id survived a verified-but-idless email change:\n%s", content)
	}
}

// A re-login must store the credential under the profile's own backend, not
// the --backend flag default. Omitting --backend on a 1Password profile must
// never leak the token into the keyring; if 1Password is unavailable the login
// errors rather than silently falling back.
func TestAuthLoginReloginUsesProfileBackendNotFlagDefault(t *testing.T) {
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "") // force the 1Password backend to fail fast
	configPath := filepath.Join(t.TempDir(), "config.toml")
	initial := `default_profile = "default"

[[profiles]]
  name = "default"
  base_url = "https://company.atlassian.net"
  auth_type = "token"
  email = "dev@example.com"
  secret_backend = "1password"
  vault = "Engineering"
  item = "jira-cli-default"
  refresh_interval = 30
  timeout = 30
  workday_seconds = 28800
`
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("JIRA_TEST_TOKEN", "op-token")
	// No --backend: the profile's 1Password backend must be used for storage.
	_, _, _ = runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", "https://company.atlassian.net",
		"--credential-env", "JIRA_TEST_TOKEN",
		"--skip-verify",
	)
	if _, getErr := keyringTokenFor(t, "default", "https://company.atlassian.net"); !errors.Is(getErr, config.ErrCredentialNotFound) {
		t.Fatalf("credential leaked to the keyring instead of the profile's 1password backend: %v", getErr)
	}
}

// An explicit --backend that conflicts with the profile's stored backend would
// silently relocate a live secret; that is `auth migrate`'s job. Login must
// refuse it, leave the stored backend untouched, and write nothing.
func TestAuthLoginRejectsBackendSwitchOnRelogin(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	initial := `default_profile = "default"

[[profiles]]
  name = "default"
  base_url = "https://company.atlassian.net"
  auth_type = "token"
  email = "dev@example.com"
  secret_backend = "1password"
  vault = "Engineering"
  item = "jira-cli-default"
  refresh_interval = 30
  timeout = 30
  workday_seconds = 28800
`
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("JIRA_TEST_TOKEN", "switch-token")
	_, _, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", "https://company.atlassian.net",
		"--email", "dev@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
		"--skip-verify",
	)
	if err == nil {
		t.Fatal("auth login silently switched an existing profile's backend")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "migrate") {
		t.Fatalf("error does not direct the user to auth migrate: %v", err)
	}
	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if !strings.Contains(string(content), `secret_backend = "1password"`) {
		t.Fatalf("conflicting login mutated the stored backend:\n%s", content)
	}
	if _, getErr := keyringTokenFor(t, "default", "https://company.atlassian.net"); !errors.Is(getErr, config.ErrCredentialNotFound) {
		t.Fatalf("conflicting login wrote a credential to the keyring: %v", getErr)
	}
}

// An explicit --backend that MATCHES the profile's stored backend is not a
// conflict and must be allowed.
func TestAuthLoginAllowsMatchingExplicitBackend(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	initial := `default_profile = "default"

[[profiles]]
  name = "default"
  base_url = "https://company.atlassian.net"
  auth_type = "token"
  email = "dev@example.com"
  secret_backend = "keyring"
  refresh_interval = 30
  timeout = 30
  workday_seconds = 28800
`
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("JIRA_TEST_TOKEN", "same-backend-token")
	_, stderr, err := runAuthLoginInProcess(t, cli.ModeJSON, configPath,
		"--base-url", "https://company.atlassian.net",
		"--email", "dev@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
		"--skip-verify",
	)
	if err != nil {
		t.Fatalf("matching explicit --backend was rejected: %v\nstderr=%s", err, stderr)
	}
	token, getErr := keyringTokenFor(t, "default", "https://company.atlassian.net")
	if getErr != nil {
		t.Fatalf("credential not stored for a matching-backend re-login: %v", getErr)
	}
	if token != "same-backend-token" {
		t.Fatalf("stored token = %q, want same-backend-token", token)
	}
}

// On a human terminal a successful login echoes the authenticated identity so
// the user sees who they logged in as, not just a silent prompt return.
func TestAuthLoginAnnouncesAuthenticatedUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"acc-7","displayName":"Grace Hopper","emailAddress":"grace@example.com"}`))
	}))
	defer srv.Close()

	t.Setenv("JIRA_TEST_TOKEN", "good-token")
	configPath := filepath.Join(t.TempDir(), "config.toml")

	_, stderr, err := runAuthLoginInProcess(t, cli.ModePlain, configPath,
		"--base-url", srv.URL,
		"--email", "grace@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_TEST_TOKEN",
	)
	if err != nil {
		t.Fatalf("auth login error = %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stderr, "Grace Hopper") {
		t.Fatalf("login did not announce the authenticated user:\n%s", stderr)
	}
}
