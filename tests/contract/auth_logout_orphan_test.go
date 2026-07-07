// A credential can outlive its profile: deleting the profile from config does
// not remove the stored secret, and `auth logout` used to refuse the name with
// a profile-not-found error, stranding a live token in the keychain. The entry
// is keyed by site host + profile name, so `--base-url` supplies the missing
// half of that identity and lets logout purge the orphan. Without the flag the
// refusal stands — fabricating a profile for a credential-admin command is the
// typo-safety hole TestAuthLogoutRejectsUnknownPositionalProfile pins shut.
package contract

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

// seedOrphanedCredential stores a token exactly as `auth login` would for a
// keyring-backed profile, into the suite's file-backed credential store.
func seedOrphanedCredential(t *testing.T, name, baseURL string) (config.CredentialStore, config.SecretRef) {
	t.Helper()
	store, ok := config.FileCredentialStoreFromEnv()
	if !ok {
		t.Fatal("contract suite must run with JIRA_TEST_CREDENTIAL_STORE_DIR set (see TestMain)")
	}
	ref, err := config.CredentialIdentity(config.Profile{
		Name:          name,
		BaseURL:       baseURL,
		SecretBackend: config.SecretBackendKeyring,
	})
	if err != nil {
		t.Fatalf("CredentialIdentity() error = %v", err)
	}
	if err := store.Put(context.Background(), ref, "orphan-token"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(context.Background(), ref) })
	return store, ref
}

// configWithoutProfile writes a config whose only profile is NOT the one the
// credential belongs to — the credential's own profile has been deleted.
func configWithoutProfile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "keep"

[[profiles]]
name = "keep"
base_url = "https://keep.atlassian.net"
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

func TestAuthLogoutPurgesOrphanedCredentialWithBaseURL(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := configWithoutProfile(t)
	store, ref := seedOrphanedCredential(t, "ghost", "https://ghost.atlassian.net")

	// The shorthand site spelling must resolve to the same host the login
	// wrote the entry under.
	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"auth", "logout", "ghost", "--base-url", "ghost.atlassian.net", "--force").CombinedOutput()
	if err != nil {
		t.Fatalf("auth logout of an orphaned credential error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Profile string `json:"profile"`
			Removed bool   `json:"removed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	if env.Data.Profile != "ghost" || !env.Data.Removed {
		t.Fatalf("auth logout = %+v, want profile ghost removed=true\n%s", env.Data, out)
	}
	if _, err := store.Get(context.Background(), ref); !errors.Is(err, config.ErrCredentialNotFound) {
		t.Fatalf("orphaned credential still present after logout: Get() error = %v", err)
	}
}

// Without --base-url the deleted profile is still refused — the orphan purge
// must not weaken the unknown-profile refusal into fabrication.
func TestAuthLogoutStillRefusesDeletedProfileWithoutBaseURL(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := configWithoutProfile(t)
	store, ref := seedOrphanedCredential(t, "ghost", "https://ghost.atlassian.net")

	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"auth", "logout", "ghost").CombinedOutput()
	if err == nil {
		t.Fatalf("auth logout of a deleted profile without --base-url succeeded:\n%s", out)
	}
	if got, getErr := store.Get(context.Background(), ref); getErr != nil || got != "orphan-token" {
		t.Fatalf("refused logout must leave the credential untouched: Get() = %q, %v", got, getErr)
	}
}
