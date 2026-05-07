package config

import (
	"context"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const keyringService = "jira-cli"

type KeyringStore struct{}

func (KeyringStore) Get(_ context.Context, ref SecretRef) (string, error) {
	v, err := keyring.Get(keyringService, ref.Profile)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrCredentialNotFound
		}
		return "", fmt.Errorf("keyring get %q: %w", ref.Profile, err)
	}
	return v, nil
}

func (KeyringStore) Put(_ context.Context, ref SecretRef, secret string) error {
	return keyring.Set(keyringService, ref.Profile, secret)
}

func (KeyringStore) Delete(_ context.Context, ref SecretRef) error {
	return keyring.Delete(keyringService, ref.Profile)
}
