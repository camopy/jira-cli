package config

import (
	"context"
	"fmt"
	"os"
)

// EnvCredentialStore is the credential store for the env secret backend: the
// profile's credential is the JIRA_TOKEN_* environment variable, read at run
// time, and nothing is ever written to disk or a keyring. It exists for
// environments with no usable OS keyring (WSL, headless Linux, containers)
// and for secret injectors like `op run` that provide the token per process.
//
// The same variable is already the cross-backend override checked first by
// ResolveCredential; this store makes it a profile's sole, declared source so
// `auth status` reports it honestly instead of "credential not found" against
// a backend that was never going to hold anything.
type EnvCredentialStore struct{}

// Get reads the credential from the profile's JIRA_TOKEN_* variable. The
// value is returned verbatim — the same semantics as the ResolveCredential
// override path — so a token whose bytes matter is never altered. An unset
// or empty variable is a typed env-credential-unset error naming the exact
// variable, wrapping ErrCredentialNotFound so callers that treat a missing
// credential as benign (logout, transactional prior reads) keep working.
func (EnvCredentialStore) Get(_ context.Context, ref SecretRef) (string, error) {
	key := ref.EnvTokenKey()
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", &CredentialError{
		Type:        ErrorTypeAuth,
		ErrCode:     ErrorCodeEnvCredentialUnset,
		Message:     fmt.Sprintf("environment variable %s is not set for profile %q", key, ref.Profile),
		HintMsg:     fmt.Sprintf("export %s with the profile's API token", key),
		IsRetryable: false,
		Context:     ErrorContext{Backend: string(SecretBackendEnv)},
		Wrapped:     ErrCredentialNotFound,
	}
}

// Put always fails: the env backend cannot write to the parent process's
// environment. Login on the env backend never calls Put — it saves metadata
// only — so reaching this means a code path tried to store into env
// (a migration destination); the error says to manage the variable instead.
func (EnvCredentialStore) Put(_ context.Context, ref SecretRef, _ string) error {
	return envBackendUnwritableError(ref, "store")
}

// Delete removes nothing: the variable belongs to the launching shell or
// secret injector. An unset variable is normalized to ErrCredentialNotFound
// so logout of a not-set profile stays idempotent; a set variable is an
// error telling the user where the credential actually lives.
func (EnvCredentialStore) Delete(_ context.Context, ref SecretRef) error {
	if os.Getenv(ref.EnvTokenKey()) == "" {
		return ErrCredentialNotFound
	}
	return envBackendUnwritableError(ref, "remove")
}

// envBackendUnwritableError reports a write or delete attempted against the
// env backend, which only reads. Validation class: the fix is a different
// command or shell action, not a retry.
func envBackendUnwritableError(ref SecretRef, verb string) *CredentialError {
	key := ref.EnvTokenKey()
	return &CredentialError{
		Type:        ErrorTypeValidation,
		ErrCode:     ErrorCodeEnvBackendReadOnly,
		Message:     fmt.Sprintf("the env backend cannot %s a credential — %s is managed by the environment that launches jira", verb, key),
		HintMsg:     fmt.Sprintf("set or unset %s in the shell or secret injector that runs jira", key),
		IsRetryable: false,
		Context:     ErrorContext{Backend: string(SecretBackendEnv)},
	}
}
