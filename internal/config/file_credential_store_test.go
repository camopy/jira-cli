package config

import (
	"context"
	"errors"
	"os"
	"testing"
)

func fileCredentialRef(t *testing.T, profile, baseURL string) SecretRef {
	t.Helper()
	ref, err := CredentialIdentity(Profile{Name: profile, BaseURL: baseURL, SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(%q) error = %v", profile, err)
	}
	return ref
}

func TestFileCredentialStoreRoundTripAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	ref := fileCredentialRef(t, "work", "https://company.atlassian.net")

	first := NewFileCredentialStore(dir)
	if err := first.Put(context.Background(), ref, "the-token"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	second := NewFileCredentialStore(dir)
	got, err := second.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "the-token" {
		t.Fatalf("Get() = %q, want the-token", got)
	}
}

func TestFileCredentialStoreUsesKeyringIdentityForKeyringBackend(t *testing.T) {
	dir := t.TempDir()
	withOnePasswordFields := fileCredentialRef(t, "work", "https://company.atlassian.net")
	withOnePasswordFields.Account = "Team"
	withOnePasswordFields.Vault = "Private"
	withOnePasswordFields.Item = "jira-cli-default"
	plain := fileCredentialRef(t, "work", "https://company.atlassian.net")

	store := NewFileCredentialStore(dir)
	if err := store.Put(context.Background(), withOnePasswordFields, "the-token"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := store.Get(context.Background(), plain)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "the-token" {
		t.Fatalf("Get() = %q, want the-token", got)
	}
}

func TestFileCredentialStoreMissingIsTypedError(t *testing.T) {
	ref := fileCredentialRef(t, "work", "https://company.atlassian.net")
	_, err := NewFileCredentialStore(t.TempDir()).Get(context.Background(), ref)
	if err == nil {
		t.Fatal("Get() of missing credential error = nil")
	}
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get() error does not wrap ErrCredentialNotFound: %v", err)
	}
	var ce *CredentialError
	if !errors.As(err, &ce) {
		t.Fatalf("Get() error type = %T, want *CredentialError", err)
	}
	if ce.ErrCode != ErrorCodeCredentialMissing {
		t.Fatalf("Get() code = %q, want %q", ce.ErrCode, ErrorCodeCredentialMissing)
	}
}

func TestFileCredentialStoreFromEnv(t *testing.T) {
	t.Setenv(TestCredentialStoreDirEnv, "  "+t.TempDir()+"  ")
	store, ok := FileCredentialStoreFromEnv()
	if !ok {
		t.Fatal("FileCredentialStoreFromEnv() ok = false")
	}

	ref := fileCredentialRef(t, "work", "https://company.atlassian.net")
	if err := store.Put(context.Background(), ref, "from-env"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "from-env" {
		t.Fatalf("Get() = %q, want from-env", got)
	}
}

func TestFileCredentialStoreWritesUserOnlyFiles(t *testing.T) {
	dir := t.TempDir()
	ref := fileCredentialRef(t, "work", "https://company.atlassian.net")
	store := NewFileCredentialStore(dir)
	if err := store.Put(context.Background(), ref, "the-token"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("stored entries = %d, want 1", len(entries))
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("credential file mode = %v, want 0600", got)
	}
}
