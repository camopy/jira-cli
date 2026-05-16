package unit

import (
	"context"
	"errors"
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
