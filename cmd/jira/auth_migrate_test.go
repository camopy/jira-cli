package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// applyMigratedBackends must flip secret_backend only on the profiles that
// actually migrate, identified by the ProfileIndex carried in each
// CredentialMigration. A two-profile batch where one profile migrates and the
// other does not must leave the non-migrating profile's backend untouched.
func TestApplyMigratedBackendsFlipsOnlyMigratingProfiles(t *testing.T) {
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "alpha", SecretBackend: config.SecretBackendKeyring},
			{Name: "beta", SecretBackend: config.SecretBackendOnePassword},
		},
	}
	// Only profile index 0 ("alpha") migrates to 1password; "beta" already
	// uses the target and is not in the migrations slice.
	migrations := []config.CredentialMigration{
		{Profile: "alpha", ProfileIndex: 0},
	}

	applyMigratedBackends(cfg, migrations, config.SecretBackendOnePassword)

	if cfg.Profiles[0].SecretBackend != config.SecretBackendOnePassword {
		t.Fatalf("migrating profile backend = %q, want 1password", cfg.Profiles[0].SecretBackend)
	}
	if cfg.Profiles[1].SecretBackend != config.SecretBackendOnePassword {
		t.Fatalf("non-migrating profile backend = %q, want unchanged 1password", cfg.Profiles[1].SecretBackend)
	}
}

// probeRemoteAuth must build its Jira client through the same constructor
// normal commands use, so a per-profile request timeout actually bounds the
// probe's live calls. Against a server that stalls far longer than the
// configured timeout, the probe must fail fast rather than hang on the
// default client timeout.
func TestProbeRemoteAuthHonorsProfileTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stall well past the 1s profile timeout, but release the connection
		// as soon as the client cancels so the server closes promptly.
		select {
		case <-time.After(8 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	cmd := &cobra.Command{Use: "probe-test"}
	cmd.SetContext(context.Background())
	rootCmd.AddCommand(cmd)
	defer rootCmd.RemoveCommand(cmd)

	profile := config.Profile{
		Name:           "slow",
		BaseURL:        srv.URL, // httptest binds 127.0.0.1, treated as local
		AuthType:       config.AuthTypeToken,
		Email:          "dev@example.com",
		TimeoutSeconds: 1,
	}

	done := make(chan map[string]any, 1)
	go func() {
		done <- probeRemoteAuth(cmd, profile, "")
	}()

	select {
	case out := <-done:
		myself, _ := out["myself"].(map[string]any)
		if myself == nil {
			t.Fatalf("probe output missing myself section: %+v", out)
		}
		if ok, _ := myself["ok"].(bool); ok {
			t.Fatalf("probe unexpectedly succeeded against a stalled server: %+v", out)
		}
		errText := strings.ToLower(fmt.Sprint(myself["error"]))
		if !strings.Contains(errText, "timeout") &&
			!strings.Contains(errText, "deadline") &&
			!strings.Contains(errText, "client.timeout") {
			t.Fatalf("probe error did not look like a timeout: %q", errText)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not honor the 1s profile timeout — it hung on the default client timeout")
	}
}

// A credential resolution warning belongs to the command that produced it.
// Each command invocation owns a fresh per-command warning sink, so a
// legacy-keyring-fallback warning recorded while resolving one command's
// credential must not appear in a later command's envelope.
func TestCredentialWarningDoesNotLeakAcrossCommands(t *testing.T) {
	// mkCmd builds a command with a fresh per-command credential-warning sink,
	// mirroring what PersistentPreRunE installs for a real invocation.
	mkCmd := func() (*cobra.Command, *bytes.Buffer) {
		var buf bytes.Buffer
		cmd := &cobra.Command{Use: "warn-scope-test"}
		cmd.SetContext(withCredentialWarnSink(context.Background()))
		cmd.SetOut(&buf)
		rootCmd.AddCommand(cmd)
		t.Cleanup(func() { rootCmd.RemoveCommand(cmd) })
		return cmd, &buf
	}

	envelopeWarnings := func(t *testing.T, buf *bytes.Buffer) []any {
		t.Helper()
		var env map[string]any
		if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
			t.Fatalf("envelope is not JSON: %v\n%s", err, buf.String())
		}
		warns, _ := env["warnings"].([]any)
		return warns
	}

	// Command A resolves a legacy credential — its sink records the warning.
	cmdA, bufA := mkCmd()
	recordCredentialWarnings(cmdA, []string{
		`profile "work" credential resolved from a legacy keyring entry`,
	})
	if err := writeEnvelope(cmdA, "auth.token", map[string]any{"profile": "work"}); err != nil {
		t.Fatalf("command A envelope error = %v", err)
	}
	if len(envelopeWarnings(t, bufA)) == 0 {
		t.Fatal("command A envelope dropped its own credential warning")
	}

	// Command B resolves nothing. Its sink is fresh and empty, so its
	// envelope must carry no warning — nothing leaks from command A.
	cmdB, bufB := mkCmd()
	if err := writeEnvelope(cmdB, "auth.status", map[string]any{"profile": "other"}); err != nil {
		t.Fatalf("command B envelope error = %v", err)
	}
	if warns := envelopeWarnings(t, bufB); len(warns) != 0 {
		t.Fatalf("command B envelope leaked a warning from command A: %v", warns)
	}
}

// revokeOldCredentialOnRelogin must REVOKE — not merely warn about — the old
// keyring credential when an auth login re-points an existing profile at a
// different Jira site. The credential under the old key must be gone.
func TestRevokeOldCredentialOnReloginRemovesOldKeyringCredential(t *testing.T) {
	keyring.MockInit()
	cmd := &cobra.Command{Use: "relogin-host-change-test"}
	cmd.SetContext(context.Background())

	previous := config.Profile{Name: "work", BaseURL: "https://old.atlassian.net", SecretBackend: config.SecretBackendKeyring}
	updated := config.Profile{Name: "work", BaseURL: "https://new.atlassian.net", SecretBackend: config.SecretBackendKeyring}

	oldRef, err := secretRefFor(previous, previous.SecretBackend)
	if err != nil {
		t.Fatalf("secretRefFor(previous) error = %v", err)
	}
	if err := credentialStoreFor(config.SecretBackendKeyring).Put(cmd.Context(), oldRef, "old-token"); err != nil {
		t.Fatalf("seed keyring Put error = %v", err)
	}

	if note := revokeOldCredentialOnRelogin(cmd, previous, updated); note != "" {
		t.Fatalf("revokeOldCredentialOnRelogin() returned a cleanup-failure note: %q", note)
	}
	if _, getErr := credentialStoreFor(config.SecretBackendKeyring).Get(cmd.Context(), oldRef); !errIsCredentialNotFound(getErr) {
		t.Fatalf("old keyring credential was not revoked after re-login: err=%v", getErr)
	}
}

// A secret-backend change (keyring -> 1Password) re-points the credential to a
// different backend; the OLD backend's credential must be revoked.
func TestRevokeOldCredentialOnReloginRevokesOldBackend(t *testing.T) {
	keyring.MockInit()
	cmd := &cobra.Command{Use: "relogin-backend-change-test"}
	cmd.SetContext(context.Background())

	previous := config.Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: config.SecretBackendKeyring}
	updated := config.Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: config.SecretBackendOnePassword, Vault: "Engineering"}

	oldRef, err := secretRefFor(previous, previous.SecretBackend)
	if err != nil {
		t.Fatalf("secretRefFor(previous) error = %v", err)
	}
	if err := credentialStoreFor(config.SecretBackendKeyring).Put(cmd.Context(), oldRef, "old-token"); err != nil {
		t.Fatalf("seed keyring Put error = %v", err)
	}

	if note := revokeOldCredentialOnRelogin(cmd, previous, updated); note != "" {
		t.Fatalf("revokeOldCredentialOnRelogin() returned a cleanup-failure note: %q", note)
	}
	if _, getErr := credentialStoreFor(config.SecretBackendKeyring).Get(cmd.Context(), oldRef); !errIsCredentialNotFound(getErr) {
		t.Fatalf("old keyring credential was not revoked after a backend change: err=%v", getErr)
	}
}

// revokeOldCredentialOnRelogin stays silent when the credential identity is
// unchanged: nothing was re-pointed, nothing to revoke.
func TestRevokeOldCredentialOnReloginSilentWhenIdentityUnchanged(t *testing.T) {
	keyring.MockInit()
	cmd := &cobra.Command{Use: "relogin-stable-test"}
	cmd.SetContext(context.Background())

	previous := config.Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: config.SecretBackendKeyring}
	updated := config.Profile{Name: "work", BaseURL: "company", SecretBackend: config.SecretBackendKeyring}

	oldRef, err := secretRefFor(previous, previous.SecretBackend)
	if err != nil {
		t.Fatalf("secretRefFor(previous) error = %v", err)
	}
	if err := credentialStoreFor(config.SecretBackendKeyring).Put(cmd.Context(), oldRef, "the-token"); err != nil {
		t.Fatalf("seed keyring Put error = %v", err)
	}

	if note := revokeOldCredentialOnRelogin(cmd, previous, updated); note != "" {
		t.Fatalf("revokeOldCredentialOnRelogin() = %q for an unchanged identity, want \"\"", note)
	}
	// The credential must remain — it was not re-pointed.
	if _, getErr := credentialStoreFor(config.SecretBackendKeyring).Get(cmd.Context(), oldRef); getErr != nil {
		t.Fatalf("revoked a credential whose identity did not change: err=%v", getErr)
	}
}

// revokeOldCredentialOnRelogin stays silent for a brand-new profile: a profile
// with no prior base_url was never persisted, so there is no old credential.
func TestRevokeOldCredentialOnReloginSilentForFreshProfile(t *testing.T) {
	keyring.MockInit()
	cmd := &cobra.Command{Use: "relogin-fresh-test"}
	cmd.SetContext(context.Background())

	previous := config.Profile{Name: "work"} // never persisted: no base_url
	updated := config.Profile{Name: "work", BaseURL: "https://new.atlassian.net", SecretBackend: config.SecretBackendKeyring}

	if note := revokeOldCredentialOnRelogin(cmd, previous, updated); note != "" {
		t.Fatalf("revokeOldCredentialOnRelogin() = %q for a fresh profile, want \"\"", note)
	}
}

// A cleanup failure revoking the old credential must be SURFACED as a note,
// not swallowed — and must not be returned as a login-blocking error.
func TestRevokeOldCredentialSurfacesCleanupFailure(t *testing.T) {
	oldRef := config.SecretRef{Profile: "work", Backend: config.SecretBackendKeyring, Host: "old.atlassian.net"}
	newRef := config.SecretRef{Profile: "work", Backend: config.SecretBackendKeyring, Host: "new.atlassian.net"}
	store := deleteFailingStore{}

	note := revokeOldCredential(context.Background(), store, oldRef, newRef)
	if note == "" {
		t.Fatal("revokeOldCredential() = \"\" when cleanup failed, want a surfaced cleanup-failure note")
	}
	if !strings.Contains(note, "old.atlassian.net") {
		t.Fatalf("cleanup-failure note does not name the old credential: %q", note)
	}
}

// revokeOldCredential stays silent when the identity is unchanged: the store
// is never even consulted.
func TestRevokeOldCredentialSilentWhenIdentityUnchanged(t *testing.T) {
	ref := config.SecretRef{Profile: "work", Backend: config.SecretBackendKeyring, Host: "company.atlassian.net"}
	// A store whose Delete fails — it must never be called when identity is
	// unchanged, so a silent result proves it was skipped.
	if note := revokeOldCredential(context.Background(), deleteFailingStore{}, ref, ref); note != "" {
		t.Fatalf("revokeOldCredential() = %q for an unchanged identity, want \"\"", note)
	}
}

// deleteFailingStore is a CredentialStore whose Delete always fails, so the
// cleanup-failure surfacing path can be exercised.
type deleteFailingStore struct{}

func (deleteFailingStore) Get(context.Context, config.SecretRef) (string, error) {
	return "old-token", nil
}

func (deleteFailingStore) Put(context.Context, config.SecretRef, string) error {
	return nil
}

func (deleteFailingStore) Delete(context.Context, config.SecretRef) error {
	return errors.New("backend delete failed")
}

func errIsCredentialNotFound(err error) bool {
	return err != nil && errors.Is(err, config.ErrCredentialNotFound)
}

// The reverse case: the second profile migrates while the first does not.
func TestApplyMigratedBackendsTargetsCorrectIndex(t *testing.T) {
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "alpha", SecretBackend: config.SecretBackendKeyring},
			{Name: "beta", SecretBackend: config.SecretBackendKeyring},
		},
	}
	migrations := []config.CredentialMigration{
		{Profile: "beta", ProfileIndex: 1},
	}

	applyMigratedBackends(cfg, migrations, config.SecretBackendOnePassword)

	if cfg.Profiles[0].SecretBackend != config.SecretBackendKeyring {
		t.Fatalf("non-migrating profile backend = %q, want unchanged keyring", cfg.Profiles[0].SecretBackend)
	}
	if cfg.Profiles[1].SecretBackend != config.SecretBackendOnePassword {
		t.Fatalf("migrating profile backend = %q, want 1password", cfg.Profiles[1].SecretBackend)
	}
}
