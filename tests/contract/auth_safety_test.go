package contract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
)

func TestAuthLoginRefusesPromptsInHeadlessJSONMode(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "--output=json", "auth", "login")
	cmd.Stdin = strings.NewReader("work\nhttps://company.atlassian.net\ntoken\ndev@example.com\nkeyring\n\n\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("headless auth login prompted and succeeded:\nstdout=%s", stdout.String())
	}
	// Prompts must not appear on stdout (the JSON path).
	if strings.Contains(stdout.String(), "Profile:") || strings.Contains(stdout.String(), "Secret backend:") {
		t.Fatalf("headless auth login wrote prompts to stdout:\nstdout=%s", stdout.String())
	}
	// clog diagnostic on stderr must mention "--no-input".
	stderrLow := strings.ToLower(stderr.String())
	if !strings.Contains(stderrLow, "err") || !strings.Contains(stderrLow, "no-input") {
		t.Fatalf("headless auth login did not emit clog diagnostic on stderr:\nstderr=%s", stderr.String())
	}
	// --json path must deliver a JSON envelope on stderr.
	var env map[string]any
	decodeErrorEnvelopeFromStderr(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, &env)
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("envelope.errors is empty:\nstderr=%s", stderr.String())
	}
}

func TestAuthLoginDoesNotExposeRawTokenFlag(t *testing.T) {
	bin := buildJiraBinary(t)
	cmd := exec.Command(bin, "auth", "login", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login --help error = %v\n%s", err, out)
	}
	if strings.Contains(string(out), "--token") {
		t.Fatalf("auth login exposes raw token flag:\n%s", out)
	}
}

func TestAuthLoginFailedVerifyForNewProfileDoesNotMutateExistingProfile(t *testing.T) {
	var verifyHits atomic.Int32
	rejectingJira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		verifyHits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
	}))
	t.Cleanup(rejectingJira.Close)

	bin := buildJiraBinary(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	serviceHash := sha256.Sum256([]byte(configPath))
	// KeyringStore resolves JIRA_KEYRING_SERVICE on every Get/Put/Delete call.
	// This per-test namespace keeps the seeded and cleanup refs away from both
	// real credentials and other contract runs using the suite default service.
	t.Setenv("JIRA_KEYRING_SERVICE", "jira-cli-contract-"+hex.EncodeToString(serviceHash[:8]))
	probeProfile := "probe-" + hex.EncodeToString(serviceHash[8:12])
	initial := `default_profile = "default"

[[profiles]]
name = "default"
base_url = "https://work.atlassian.net"
auth_type = "token"
email = "user@example.com"
account_id = "account-123"
secret_backend = "keyring"
onepassword_account = "Team"
vault = "Private"
item = "jira-cli-default"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store := cmdutil.CredentialStoreFor(config.SecretBackendKeyring)
	// Use the same identity helper as the binary so the assertions target the
	// exact keyring entries auth login would write or preserve.
	mustRef := func(profile, baseURL string) config.SecretRef {
		t.Helper()
		ref, err := cmdutil.SecretRefFor(config.Profile{
			Name:          profile,
			BaseURL:       config.NormalizeBaseURL(baseURL),
			SecretBackend: config.SecretBackendKeyring,
		}, config.SecretBackendKeyring)
		if err != nil {
			t.Fatalf("SecretRefFor(%q, %q) error = %v", profile, baseURL, err)
		}
		return ref
	}
	defaultRef := mustRef("default", "https://work.atlassian.net")
	probeRef := mustRef(probeProfile, rejectingJira.URL)
	t.Cleanup(func() {
		_ = store.Delete(context.Background(), defaultRef)
		_ = store.Delete(context.Background(), probeRef)
	})

	cmd := exec.Command(bin, "--config", configPath, "--output=json", "auth", "login",
		"--profile-name", "default",
		"--base-url", "https://work.atlassian.net",
		"--email", "user@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_EXISTING_TOKEN",
		"--skip-verify",
		"--no-input",
	)
	const existingCredential = "existing-token-1234"
	cmd.Env = append(os.Environ(), "JIRA_EXISTING_TOKEN="+existingCredential)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed auth login error = %v\n%s", err, out)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() before failed login error = %v", err)
	}
	beforeHash := sha256.Sum256(before)

	cmd = exec.Command(bin, "--config", configPath, "--output=json", "auth", "login",
		"--profile-name", probeProfile,
		"--base-url", rejectingJira.URL,
		"--email", "john.doe@example.com",
		"--backend", "keyring",
		"--credential-env", "JIRA_BAD_TOKEN",
		"--no-input",
	)
	cmd.Env = append(os.Environ(), "JIRA_BAD_TOKEN=wrong-token-9999")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("auth login with rejected new-profile credential succeeded:\n%s", out)
	}
	authFailureExitCode := cli.ExitCode(cli.NewError(cli.ErrorTypeAuth, "auth verification failed"))
	assertExitCode(t, err, authFailureExitCode, "auth verification failures -> exit 1")
	if got := verifyHits.Load(); got == 0 {
		t.Fatalf("auth login failed before reaching credential verification:\n%s", out)
	}

	// auth login verifies the in-memory credential before UpsertProfile,
	// config.Save, or StoreCredentialTransactionally. A failed verify must
	// therefore leave the file and all existing secret refs untouched.
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() after failed login error = %v", err)
	}
	if afterHash := sha256.Sum256(after); afterHash != beforeHash {
		t.Fatalf("failed new-profile login mutated config.toml\nbefore:\n%s\nafter:\n%s", before, after)
	}

	stored, err := config.StoredCredential(context.Background(), store, defaultRef)
	if err != nil {
		t.Fatalf("default profile credential missing after failed login: %v", err)
	}
	if stored != existingCredential {
		t.Fatalf("default profile credential = %q, want %q", stored, existingCredential)
	}
	if _, err := config.StoredCredential(context.Background(), store, probeRef); !errors.Is(err, config.ErrCredentialNotFound) {
		t.Fatalf("failed new-profile login stored a credential for %s: %v", probeProfile, err)
	}
}

// writeTwoProfileConfig writes a config with a real "work" profile so an
// explicit `--profile <typo>` cannot silently resolve to a fabricated
// default-like profile.
func writeTwoProfileConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "work"

[[profiles]]
name = "work"
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

// `auth logout PROFILE` with a typoed positional profile must be refused
// with a profile-not-found error — not fabricated into a synthetic
// profile whose credential namespace then gets touched. The error must
// name the bad profile and must NOT be a keyring/credential-backend
// error (which is what fabrication produces today).
func TestAuthLogoutRejectsUnknownPositionalProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := writeTwoProfileConfig(t)
	c := exec.Command(bin, "--config", cfg, "auth", "logout", "typo")
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err == nil {
		t.Fatalf("auth logout with unknown positional profile succeeded:\nstdout=%s", stdout.String())
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "typo") {
		t.Fatalf("auth logout error does not name the unknown profile:\n%s", combined)
	}
	if strings.Contains(strings.ToLower(combined), "keyring") || strings.Contains(strings.ToLower(combined), "secret not found") {
		t.Fatalf("auth logout fabricated the profile and probed its credential namespace:\n%s", combined)
	}
}

// `auth token --profile <typo>` must be refused with a profile-not-found
// error rather than resolved into a fabricated synthetic profile that then
// reports phantom diagnostics at exit 0. It must name the bad profile.
func TestAuthTokenRejectsUnknownProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := writeTwoProfileConfig(t)
	c := exec.Command(bin, "--config", cfg, "--profile", "typo", "auth", "token")
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err == nil {
		t.Fatalf("auth token with unknown profile succeeded:\nstdout=%s", stdout.String())
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "typo") {
		t.Fatalf("auth token error does not name the unknown profile:\n%s", combined)
	}
}

// `auth refresh --profile <typo>` must be refused with a profile-not-found
// error rather than resolved into a fabricated synthetic profile. It must
// name the bad profile.
func TestAuthRefreshRejectsUnknownProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := writeTwoProfileConfig(t)
	c := exec.Command(bin, "--config", cfg, "--profile", "typo", "auth", "refresh")
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err == nil {
		t.Fatalf("auth refresh with unknown profile succeeded:\nstdout=%s", stdout.String())
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "typo") {
		t.Fatalf("auth refresh error does not name the unknown profile:\n%s", combined)
	}
}

// `auth whoami --save` is a read-modify-write command. It must persist
// only file-backed profile fields: a transient JIRA_PROFILE_* env overlay
// must not be baked into the saved TOML.
func TestAuthWhoamiSaveDoesNotPersistEnvOverlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"acct-saved","emailAddress":"saved@example.com","displayName":"Me","active":true}`))
	}))
	t.Cleanup(srv.Close)

	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `default_profile = "work"

[[profiles]]
name = "work"
base_url = "` + srv.URL + `"
auth_type = "token"
secret_backend = "keyring"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command(bin, "--config", path, "--output=json", "auth", "whoami", "--save")
	cmd.Env = append(os.Environ(),
		"JIRA_TOKEN_WORK=test-token",
		"JIRA_PROFILE_WORK_DEFAULT_ISSUE_TYPE=OverlayType",
		"JIRA_DEFAULT_PROFILE=work",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth whoami --save error = %v\n%s", err, out)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), "OverlayType") {
		t.Fatalf("auth whoami --save persisted JIRA_PROFILE_*_DEFAULT_ISSUE_TYPE env overlay into TOML:\n%s", content)
	}
	// The resolved account_id must still be persisted to the file.
	if !strings.Contains(string(content), `account_id = "acct-saved"`) {
		t.Fatalf("auth whoami --save did not persist the resolved account_id:\n%s", content)
	}
}

// `auth whoami --save` for a profile that exists only via an env overlay
// (not in the config file) must be refused: there is no persisted profile
// to write to, and saving would fabricate one.
func TestAuthWhoamiSaveRefusesEnvOnlyProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"acct-ghost","emailAddress":"ghost@example.com","displayName":"Ghost","active":true}`))
	}))
	t.Cleanup(srv.Close)

	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://work.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command(bin, "--config", path, "--profile", "ghost", "--output=json", "auth", "whoami", "--save")
	cmd.Env = append(os.Environ(),
		"JIRA_TOKEN_GHOST=test-token",
		"JIRA_PROFILE_GHOST_BASE_URL="+srv.URL,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("auth whoami --save for an env-only profile succeeded:\nstdout=%s", stdout.String())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), "ghost") {
		t.Fatalf("auth whoami --save fabricated the env-only profile in TOML:\n%s", content)
	}
}

// `auth migrate --profile <typo>` must be refused before the migration
// loop runs. A typoed --profile previously matched no profile and the
// command returned success with an empty profiles list.
func TestAuthMigrateRejectsUnknownProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://work.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command(bin, "--config", path, "--profile", "typo", "--output=json", "auth", "migrate", "--backend", "1password")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("auth migrate --profile typo succeeded as a no-op:\nstdout=%s", stdout.String())
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "typo") {
		t.Fatalf("auth migrate error does not name the unknown profile:\n%s", combined)
	}
}

// `auth whoami --save` must fail closed when a JIRA_PROFILE_<NAME>_BASE_URL
// overlay is set for the profile being saved. The overlay would otherwise
// redirect the request — including the profile's credential — to a tenant
// other than the file-backed one, so --save is refused before any request
// is made and the error names the offending env var.
func TestAuthWhoamiSaveRefusesBaseURLEnvOverlay(t *testing.T) {
	// Server A is the file-backed tenant. The handler fails the test if
	// it is contacted at all: --save must be refused before any request.
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("auth whoami --save contacted the file-backed tenant despite a base_url env overlay")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(serverA.Close)
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"acct-server-b","emailAddress":"b@example.com","displayName":"B","active":true}`))
	}))
	t.Cleanup(serverB.Close)

	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `default_profile = "work"

[[profiles]]
name = "work"
base_url = "` + serverA.URL + `"
auth_type = "token"
secret_backend = "keyring"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command(bin, "--config", path, "--output=json", "auth", "whoami", "--save")
	cmd.Env = append(os.Environ(),
		"JIRA_TOKEN_WORK=test-token",
		"JIRA_PROFILE_WORK_BASE_URL="+serverB.URL,
		"JIRA_DEFAULT_PROFILE=work",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("auth whoami --save succeeded despite a base_url env overlay:\nstdout=%s", stdout.String())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() != 3 {
		t.Fatalf("auth whoami --save exit code = %d, want 3\nstdout=%s\nstderr=%s", exit.ExitCode(), stdout.String(), stderr.String())
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "JIRA_PROFILE_WORK_BASE_URL") {
		t.Fatalf("auth whoami --save error does not name the offending env var:\n%s", combined)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), "acct-server-b") {
		t.Fatalf("auth whoami --save persisted the env-overlay tenant's account_id into TOML:\n%s", content)
	}
}
