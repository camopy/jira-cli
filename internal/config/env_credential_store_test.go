package config

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/errtax"
)

func envStoreRef(t *testing.T) SecretRef {
	t.Helper()
	ref, err := CredentialIdentity(Profile{
		Name:          "work",
		BaseURL:       "https://acme.atlassian.net",
		SecretBackend: SecretBackendEnv,
	})
	if err != nil {
		t.Fatalf("CredentialIdentity() error = %v", err)
	}
	return ref
}

func TestEnvCredentialStoreGetReadsProfileVariable(t *testing.T) {
	t.Setenv("JIRA_TOKEN_WORK", "env-token")
	got, err := EnvCredentialStore{}.Get(context.Background(), envStoreRef(t))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "env-token" {
		t.Fatalf("Get() = %q, want env-token", got)
	}
}

// An unset variable is a typed env-credential-unset error that names the
// exact variable to export, and wraps ErrCredentialNotFound so callers that
// treat a missing credential as benign (logout, transactional prior reads)
// keep working.
func TestEnvCredentialStoreGetUnsetNamesVariable(t *testing.T) {
	t.Setenv("JIRA_TOKEN_WORK", "")
	_, err := EnvCredentialStore{}.Get(context.Background(), envStoreRef(t))
	if err == nil {
		t.Fatal("Get() error = nil for an unset variable")
	}
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get() error does not wrap ErrCredentialNotFound: %v", err)
	}
	var ce *CredentialError
	if !errors.As(err, &ce) {
		t.Fatalf("Get() error is not a CredentialError: %v", err)
	}
	if ce.ErrCode != errtax.CodeEnvCredentialUnset {
		t.Fatalf("Get() code = %q, want %q", ce.ErrCode, errtax.CodeEnvCredentialUnset)
	}
	if !strings.Contains(ce.Message, "JIRA_TOKEN_WORK") {
		t.Fatalf("Get() message does not name the variable: %q", ce.Message)
	}
}

// The env backend only reads: a write is a typed read-only error, and a
// delete is idempotent when the variable is unset but an error when it is
// set — jira cannot unset the parent process's environment.
func TestEnvCredentialStoreIsReadOnly(t *testing.T) {
	ctx := context.Background()
	ref := envStoreRef(t)

	t.Setenv("JIRA_TOKEN_WORK", "")
	if err := (EnvCredentialStore{}).Put(ctx, ref, "secret"); err == nil {
		t.Fatal("Put() error = nil, want read-only error")
	} else {
		var ce *CredentialError
		if !errors.As(err, &ce) || ce.ErrCode != errtax.CodeEnvBackendReadOnly {
			t.Fatalf("Put() error = %v, want env_backend_read_only", err)
		}
	}
	if err := (EnvCredentialStore{}).Delete(ctx, ref); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Delete() with unset variable = %v, want ErrCredentialNotFound", err)
	}

	t.Setenv("JIRA_TOKEN_WORK", "still-set")
	if err := (EnvCredentialStore{}).Delete(ctx, ref); err == nil {
		t.Fatal("Delete() error = nil with the variable set")
	} else {
		var ce *CredentialError
		if !errors.As(err, &ce) || ce.ErrCode != errtax.CodeEnvBackendReadOnly {
			t.Fatalf("Delete() error = %v, want env_backend_read_only", err)
		}
		if !strings.Contains(ce.Message, "JIRA_TOKEN_WORK") {
			t.Fatalf("Delete() message does not name the variable: %q", ce.Message)
		}
	}
}

// CredentialStatus on an env-backed profile reports a healthy credential
// sourced from the environment, and an unset variable surfaces the message
// naming it (via SanitizeCredentialError) instead of a generic not-found.
func TestEnvCredentialStoreStatus(t *testing.T) {
	ctx := context.Background()
	ref := envStoreRef(t)

	t.Setenv("JIRA_TOKEN_WORK", "env-token")
	status := CredentialStatus(ctx, EnvCredentialStore{}, ref)
	if !status.Valid || status.Source != "env" {
		t.Fatalf("status = %+v, want valid env-sourced credential", status)
	}

	t.Setenv("JIRA_TOKEN_WORK", "")
	status = CredentialStatus(ctx, EnvCredentialStore{}, ref)
	if status.Valid {
		t.Fatalf("status unexpectedly valid with the variable unset: %+v", status)
	}
	if !strings.Contains(status.Error, "JIRA_TOKEN_WORK") {
		t.Fatalf("status error does not name the variable to export: %+v", status)
	}
}
