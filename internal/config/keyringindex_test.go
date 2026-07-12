package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func indexTestRef(host, profile string) SecretRef {
	return SecretRef{Host: host, Profile: profile, Backend: SecretBackendKeyring}
}

func TestKeyringIndexAddRemoveRoundTrip(t *testing.T) {
	t.Setenv(TestCredentialStoreDirEnv, t.TempDir())

	if err := keyringIndexAdd(indexTestRef("a.example.net", "work")); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := keyringIndexAdd(indexTestRef("b.example.net", "sandbox")); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Duplicate adds collapse.
	if err := keyringIndexAdd(indexTestRef("a.example.net", "work")); err != nil {
		t.Fatalf("re-add: %v", err)
	}

	refs := (KeyringStore{}).ListStoredRefs()
	got := make([]string, 0, len(refs))
	for _, r := range refs {
		got = append(got, r.Host+"/"+r.Profile)
		if r.Backend != SecretBackendKeyring {
			t.Fatalf("listed ref backend = %q, want keyring", r.Backend)
		}
	}
	if want := []string{"a.example.net/work", "b.example.net/sandbox"}; !slices.Equal(got, want) {
		t.Fatalf("list = %v, want %v (sorted, deduped)", got, want)
	}

	if err := keyringIndexRemove(indexTestRef("a.example.net", "work")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// Removing an absent entry stays a clean no-op.
	if err := keyringIndexRemove(indexTestRef("never.example.net", "ghost")); err != nil {
		t.Fatalf("remove absent: %v", err)
	}
	refs = (KeyringStore{}).ListStoredRefs()
	if len(refs) != 1 || refs[0].Profile != "sandbox" {
		t.Fatalf("after remove = %+v, want only sandbox", refs)
	}
}

func TestKeyringIndexMissingOrBrokenReadsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(TestCredentialStoreDirEnv, dir)

	if refs := (KeyringStore{}).ListStoredRefs(); len(refs) != 0 {
		t.Fatalf("missing index listed %v, want none", refs)
	}
	if err := os.WriteFile(keyringIndexPath(), []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if refs := (KeyringStore{}).ListStoredRefs(); len(refs) != 0 {
		t.Fatalf("broken index listed %v, want none", refs)
	}
	// A broken file heals on the next write.
	if err := keyringIndexAdd(indexTestRef("a.example.net", "work")); err != nil {
		t.Fatalf("add over broken index: %v", err)
	}
	if refs := (KeyringStore{}).ListStoredRefs(); len(refs) != 1 {
		t.Fatalf("healed index listed %v, want one entry", refs)
	}
}

func TestKeyringIndexNamespacedByServiceOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(TestCredentialStoreDirEnv, dir)
	t.Setenv(keyringServiceEnv, "jira-cli-test-ns")

	if err := keyringIndexAdd(indexTestRef("a.example.net", "work")); err != nil {
		t.Fatalf("add: %v", err)
	}
	want := filepath.Join(dir, "keyring-index-jira-cli-test-ns.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("override index file missing at %s: %v", want, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keyring-index.json")); !os.IsNotExist(err) {
		t.Fatal("service override wrote the default-namespace index")
	}
}
