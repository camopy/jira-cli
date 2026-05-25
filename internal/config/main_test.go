package config

import (
	"os"
	"testing"
)

// TestMain clears JIRA_KEYRING_SERVICE so this package's keyring backend tests
// are deterministic regardless of the ambient environment. Those tests use an
// in-memory mock and assert against the default service name, while KeyringStore
// resolves the service via keyringServiceName(); if a developer or CI had the
// override exported, the stored-under-override value would not match the
// asserted default. Tests that exercise the override itself set it explicitly
// with t.Setenv, which restores this cleared state afterward.
func TestMain(m *testing.M) {
	if err := os.Unsetenv(keyringServiceEnv); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
