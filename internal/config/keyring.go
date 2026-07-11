package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	// defaultKeyringService is the secret-service "service" name jira-cli
	// stores credentials under in normal use.
	defaultKeyringService = "jira-cli"
	// keyringServiceEnv overrides that service name. It exists for explicit
	// keyring smoke tests and custom test harnesses that must confine real
	// keyring reads, writes, and deletes to a throwaway namespace instead of
	// the developer's actual "jira-cli" credentials. Production never sets it.
	// A blank or unset value uses defaultKeyringService; a wrong value fails
	// safe — a lookup simply misses (ErrCredentialNotFound), it never surfaces
	// another namespace's secret.
	keyringServiceEnv = "JIRA_KEYRING_SERVICE"
)

// keyringServiceName resolves the secret-service service name, honoring the
// keyringServiceEnv override (see its doc) and otherwise returning
// defaultKeyringService.
func keyringServiceName() string {
	if s := strings.TrimSpace(os.Getenv(keyringServiceEnv)); s != "" {
		return s
	}
	return defaultKeyringService
}

// KeyringStore stores credentials in the OS keyring under a readable
// "<site-host>/<profile>" entry name. A credential belongs to a site and a
// profile; the entry name is that identity verbatim, with no digest.
type KeyringStore struct{}

// Get reads the credential for ref from its "<host>/<profile>" keyring entry.
// A missing entry is reported as a typed credential-missing error (wrapping
// ErrCredentialNotFound) that names the profile and points at `auth login`.
// There is no legacy fallback: a credential stored by a pre-namespacing
// release under a bare profile name is not auto-resolved — the user logs in
// once after upgrading.
func (KeyringStore) Get(_ context.Context, ref SecretRef) (string, error) {
	v, err := keyring.Get(keyringServiceName(), ref.KeyringName())
	if err == nil {
		return v, nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return "", credentialMissingError(ref.Profile)
	}
	return "", KeyringUnavailableError(fmt.Errorf("keyring get %q: %w", ref.Profile, err))
}

// Put writes the credential under the "<host>/<profile>" keyring entry.
func (KeyringStore) Put(_ context.Context, ref SecretRef, secret string) error {
	if err := keyring.Set(keyringServiceName(), ref.KeyringName(), secret); err != nil {
		return KeyringUnavailableError(fmt.Errorf("keyring set %q: %w", ref.Profile, err))
	}
	return nil
}

// Delete removes the credential's "<host>/<profile>" keyring entry — the entry
// jira-cli itself created. A missing entry is normalized to
// ErrCredentialNotFound rather than surfaced as a backend failure, so logout
// is idempotent.
func (KeyringStore) Delete(_ context.Context, ref SecretRef) error {
	err := keyring.Delete(keyringServiceName(), ref.KeyringName())
	if err == nil {
		return nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrCredentialNotFound
	}
	return KeyringUnavailableError(fmt.Errorf("keyring delete %q: %w", ref.Profile, err))
}

// KeyringUnavailableError classifies a non-not-found keyring failure as a
// typed keyring-unavailable error. On WSL and headless Linux the raw failure
// is a D-Bus message ("The name org.freedesktop.secrets was not provided by
// any .service files") that reads as noise and — worse — classifies as a
// generic validation failure; typing it here gives it an auth class and a
// hint that names the actual way out (the env backend, or installing a
// Secret Service). The raw cause is kept as the wrapped error, never copied
// into the message, so backend wording cannot leak into envelopes.
func KeyringUnavailableError(wrapped error) *CredentialError {
	return &CredentialError{
		Type:        ErrorTypeAuth,
		ErrCode:     ErrorCodeKeyringUnavailable,
		Message:     "the OS keyring is unavailable — no Secret Service, Keychain, or Credential Manager answered",
		HintMsg:     "set the profile's JIRA_TOKEN_* variable and use the env backend, or install a Secret Service (e.g. gnome-keyring) and retry",
		IsRetryable: false,
		Context:     ErrorContext{Backend: string(SecretBackendKeyring)},
		Wrapped:     wrapped,
	}
}

// KeyringAvailable reports whether the OS keyring can service requests at
// all. It probes with a read of a never-written entry: a clean not-found
// means the keyring answered (available); any other failure — no Secret
// Service on the D-Bus (WSL, headless Linux), an unsupported platform —
// means it cannot store anything. Interactive login uses it to offer only
// backends that can actually work, and store paths use it to fail before a
// token is prompted for or verified.
//
// When the test file-store override is active (TestCredentialStoreDirEnv),
// keyring-backed profiles never touch the real keyring, so the backend
// counts as available.
func KeyringAvailable() bool {
	if _, ok := FileCredentialStoreFromEnv(); ok {
		return true
	}
	_, err := keyring.Get(keyringServiceName(), "jira-cli-availability-probe")
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}
