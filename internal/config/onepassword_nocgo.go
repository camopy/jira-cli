//go:build !cgo && !windows

package config

import (
	"context"
	"errors"
)

var errOnePasswordNoCGO = errors.New("1Password SDK unavailable in no-CGO build")

// OnePasswordStore reports a typed backend error in no-CGO release builds.
type OnePasswordStore struct{}

func (OnePasswordStore) Get(context.Context, SecretRef) (string, error) {
	return "", onePasswordNoCGOError()
}

func (OnePasswordStore) Put(context.Context, SecretRef, string) error {
	return onePasswordNoCGOError()
}

func (OnePasswordStore) Delete(context.Context, SecretRef) error {
	return onePasswordNoCGOError()
}

func onePasswordNoCGOError() error {
	return &CredentialError{
		Type:        ErrorTypeAuth,
		ErrCode:     ErrorCodeOnePasswordUnavailable,
		Message:     "1Password support is unavailable in this build",
		HintMsg:     "use a CGO-enabled source build or choose the keyring or env credential backend",
		IsRetryable: false,
		Context:     ErrorContext{Backend: string(SecretBackendOnePassword)},
		Upstream: &UpstreamProvider{
			Provider:     "onepassword-sdk",
			UpstreamCode: "cgo_disabled",
		},
		Wrapped: errOnePasswordNoCGO,
	}
}
