package unit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

func TestCredentialStoreResolveDeleteAndRedact(t *testing.T) {
	ctx := context.Background()
	store := config.NewMemoryCredentialStore()
	ref := config.SecretRef{Profile: "default", Backend: config.SecretBackendKeyring, Item: "jira-default"}

	if err := store.Put(ctx, ref, "secret-token"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "secret-token" {
		t.Fatalf("Get() = %q", got)
	}
	status := config.CredentialStatus(ctx, store, ref)
	if !status.Valid || status.Source != "keyring" {
		t.Fatalf("status = %+v", status)
	}
	if strings.Contains(status.Redacted, "secret-token") {
		t.Fatalf("status leaked secret: %+v", status)
	}
	if err := store.Delete(ctx, ref); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(ctx, ref); err == nil {
		t.Fatal("Get() after Delete() error = nil")
	}
}

func TestCredentialEnvFallback(t *testing.T) {
	t.Setenv("JIRA_TOKEN_DEFAULT", "env-token")
	ctx := context.Background()
	store := config.NewMemoryCredentialStore()
	ref := config.SecretRef{Profile: "default", Backend: config.SecretBackendKeyring}

	got, err := config.ResolveCredential(ctx, store, ref)
	if err != nil {
		t.Fatalf("ResolveCredential() error = %v", err)
	}
	if got != "env-token" {
		t.Fatalf("ResolveCredential() = %q", got)
	}
}

type leakingCredentialStore struct{}

func (leakingCredentialStore) Get(context.Context, config.SecretRef) (string, error) {
	return "", errors.New("backend failed with secret-token")
}

func (leakingCredentialStore) Put(context.Context, config.SecretRef, string) error {
	return nil
}

func (leakingCredentialStore) Delete(context.Context, config.SecretRef) error {
	return nil
}

func TestCredentialStatusSanitizesBackendErrors(t *testing.T) {
	status := config.CredentialStatus(context.Background(), leakingCredentialStore{}, config.SecretRef{
		Profile: "default",
		Backend: config.SecretBackendKeyring,
	})
	if status.Valid {
		t.Fatalf("status unexpectedly valid: %+v", status)
	}
	if strings.Contains(status.Error, "secret-token") {
		t.Fatalf("status leaked backend secret: %+v", status)
	}
	if status.Error == "" {
		t.Fatalf("status did not preserve a sanitized error: %+v", status)
	}
}

func TestOnePasswordExpiredSessionGuidesSignin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is Unix-specific")
	}
	dir := t.TempDir()
	op := filepath.Join(dir, "op")
	if err := os.WriteFile(op, []byte("#!/bin/sh\necho 'not signed in' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store := config.OnePasswordStore{Bin: op}
	_, err := store.Get(context.Background(), config.SecretRef{Profile: "default", Backend: config.SecretBackendOnePassword, Vault: "Engineering", Item: "jira"})
	if err == nil {
		t.Fatal("Get() error = nil")
	}
	if !strings.Contains(err.Error(), "op signin") {
		t.Fatalf("expired session error = %v", err)
	}
}
