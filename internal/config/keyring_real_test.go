//go:build realkeyring && (cgo || windows)

package config

import (
	"context"
	"os"
	"testing"
)

// TestRealKeyringStoreRoundTrip is an opt-in smoke test for the real OS
// keyring backend. It never runs in the normal test suite, and it is skipped
// under CI even if someone accidentally enables the realkeyring tag.
func TestRealKeyringStoreRoundTrip(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("real OS keyring smoke is disabled when CI is set")
	}
	t.Setenv(keyringServiceEnv, "jira-cli-real-keyring-test")
	ref := keyringRef(t, "work", "https://company.atlassian.net")
	store := KeyringStore{}
	t.Cleanup(func() { _ = store.Delete(context.Background(), ref) })

	if err := store.Put(context.Background(), ref, "real-token"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "real-token" {
		t.Fatalf("Get() = %q, want real-token", got)
	}
}
