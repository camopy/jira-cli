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
	// keyringServiceEnv overrides that service name. It exists so the
	// end-to-end contract suite — which drives the real binary and therefore
	// the real OS keyring — can confine its reads, writes, and deletes to a
	// throwaway namespace instead of the developer's actual "jira-cli"
	// credentials. Production never sets it. A blank or unset value uses
	// defaultKeyringService; a wrong value fails safe — a lookup simply misses
	// (ErrCredentialNotFound), it never surfaces another namespace's secret.
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
	return "", fmt.Errorf("keyring get %q: %w", ref.Profile, err)
}

// Put writes the credential under the "<host>/<profile>" keyring entry.
func (KeyringStore) Put(_ context.Context, ref SecretRef, secret string) error {
	return keyring.Set(keyringServiceName(), ref.KeyringName(), secret)
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
	return fmt.Errorf("keyring delete %q: %w", ref.Profile, err)
}
