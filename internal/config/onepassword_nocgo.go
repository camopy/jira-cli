//go:build !cgo && !windows

package config

import (
	"context"
)

// OnePasswordSupported reports whether this binary was built with the
// 1Password SDK compiled in. Release archives are no-CGO, so this build
// cannot use the 1Password backend at all; callers use this to fail early
// and to keep the backend out of interactive choices.
func OnePasswordSupported() bool { return false }

// OnePasswordStore reports a typed backend error in no-CGO release builds.
type OnePasswordStore struct{}

func (OnePasswordStore) Get(context.Context, SecretRef) (string, error) {
	return "", OnePasswordUnsupportedBuildError()
}

func (OnePasswordStore) Put(context.Context, SecretRef, string) error {
	return OnePasswordUnsupportedBuildError()
}

func (OnePasswordStore) Delete(context.Context, SecretRef) error {
	return OnePasswordUnsupportedBuildError()
}
