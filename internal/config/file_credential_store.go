package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TestCredentialStoreDirEnv enables a subprocess-visible credential store for
// tests. Production code never sets it; when present, command construction can
// use FileCredentialStore instead of probing the developer's OS keyring.
const TestCredentialStoreDirEnv = "JIRA_TEST_CREDENTIAL_STORE_DIR"

// FileCredentialStore stores credentials in one directory using hashed file
// names derived from the same SecretRef identity as the keyring backend.
type FileCredentialStore struct {
	dir string
}

// NewFileCredentialStore returns a file-backed credential store rooted at dir.
func NewFileCredentialStore(dir string) FileCredentialStore {
	return FileCredentialStore{dir: dir}
}

// FileCredentialStoreFromEnv returns a test credential store when
// TestCredentialStoreDirEnv is set to a non-blank directory.
func FileCredentialStoreFromEnv() (CredentialStore, bool) {
	dir := strings.TrimSpace(os.Getenv(TestCredentialStoreDirEnv))
	if dir == "" {
		return nil, false
	}
	return NewFileCredentialStore(dir), true
}

func (s FileCredentialStore) entryPath(ref SecretRef) (string, error) {
	dir := strings.TrimSpace(s.dir)
	if dir == "" {
		return "", errors.New("test credential store directory is empty")
	}
	sum := sha256.Sum256([]byte(fileCredentialStoreKey(ref)))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".secret"), nil
}

func fileCredentialStoreKey(ref SecretRef) string {
	if ref.Backend == SecretBackendKeyring {
		return string(ref.Backend) + ":" + ref.KeyringName()
	}
	return ref.key()
}

// Get reads a credential from the file store.
func (s FileCredentialStore) Get(_ context.Context, ref SecretRef) (string, error) {
	path, err := s.entryPath(ref)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is a hashed SecretRef entry inside the test credential directory.
	if err == nil {
		return string(b), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", credentialMissingError(ref.Profile)
	}
	return "", fmt.Errorf("test credential get %q: %w", ref.Profile, err)
}

// Put writes a credential to the file store with user-only permissions.
func (s FileCredentialStore) Put(_ context.Context, ref SecretRef, secret string) error {
	path, err := s.entryPath(ref)
	if err != nil {
		return err
	}
	if err := atomicWrite(path, []byte(secret)); err != nil {
		return fmt.Errorf("test credential put %q: %w", ref.Profile, err)
	}
	return nil
}

// Delete removes a credential from the file store.
func (s FileCredentialStore) Delete(_ context.Context, ref SecretRef) error {
	path, err := s.entryPath(ref)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrCredentialNotFound
		}
		return fmt.Errorf("test credential delete %q: %w", ref.Profile, err)
	}
	return nil
}
