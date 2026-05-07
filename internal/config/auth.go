package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

var ErrCredentialNotFound = errors.New("credential not found")

type SecretRef struct {
	Profile string
	Backend SecretBackend
	Account string
	Vault   string
	Item    string
}

type CredentialStore interface {
	Get(context.Context, SecretRef) (string, error)
	Put(context.Context, SecretRef, string) error
	Delete(context.Context, SecretRef) error
}

type AuthStatus struct {
	Profile  string `json:"profile"`
	Valid    bool   `json:"valid"`
	Source   string `json:"source"`
	Redacted string `json:"redacted"`
	Error    string `json:"error,omitempty"`
}

type MemoryCredentialStore struct {
	mu      sync.Mutex
	secrets map[string]string
}

func NewMemoryCredentialStore() *MemoryCredentialStore {
	return &MemoryCredentialStore{secrets: make(map[string]string)}
}

func (s *MemoryCredentialStore) Get(_ context.Context, ref SecretRef) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.secrets[ref.key()]
	if !ok {
		return "", ErrCredentialNotFound
	}
	return v, nil
}

func (s *MemoryCredentialStore) Put(_ context.Context, ref SecretRef, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[ref.key()] = secret
	return nil
}

func (s *MemoryCredentialStore) Delete(_ context.Context, ref SecretRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.secrets, ref.key())
	return nil
}

func ResolveCredential(ctx context.Context, store CredentialStore, ref SecretRef) (string, error) {
	if v := envToken(ref.Profile); v != "" {
		return v, nil
	}
	return store.Get(ctx, ref)
}

func CredentialStatus(ctx context.Context, store CredentialStore, ref SecretRef) AuthStatus {
	secret, err := ResolveCredential(ctx, store, ref)
	status := AuthStatus{Profile: ref.Profile, Source: string(ref.Backend)}
	if err != nil {
		status.Error = SanitizeCredentialError(err)
		return status
	}
	status.Valid = true
	status.Redacted = RedactSecret(secret)
	if envToken(ref.Profile) != "" {
		status.Source = "env"
	}
	return status
}

func SanitizeCredentialError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrCredentialNotFound) {
		return ErrCredentialNotFound.Error()
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "op signin"):
		return "1Password CLI session expired: run `op signin` and retry"
	case strings.Contains(msg, "1password"):
		return "1Password credential backend failed"
	case strings.Contains(msg, "keyring"):
		return "keyring credential backend failed"
	default:
		return "credential backend failed"
	}
}

func RedactSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return fmt.Sprintf("****%s", secret[len(secret)-4:])
}

func envToken(profile string) string {
	key := "JIRA_TOKEN_" + strings.ToUpper(strings.ReplaceAll(profile, "-", "_"))
	return os.Getenv(key)
}

func (r SecretRef) key() string {
	return string(r.Backend) + ":" + r.Profile + ":" + r.Account + ":" + r.Vault + ":" + r.Item
}
