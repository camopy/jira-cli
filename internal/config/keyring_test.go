//go:build cgo || windows

package config

import (
	"context"
	"errors"
	"strings"
	"testing"

	onepassword "github.com/1password/onepassword-sdk-go"
	keyring "github.com/zalando/go-keyring"
)

// keyringRef builds a SecretRef the way CredentialIdentity does, so tests use
// the same site+profile keyring key the production code derives.
func keyringRef(t *testing.T, profile, baseURL string) SecretRef {
	t.Helper()
	ref, err := CredentialIdentity(Profile{Name: profile, BaseURL: baseURL, SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(%q) error = %v", profile, err)
	}
	return ref
}

// A keyring credential round-trips: Put then Get returns the stored secret
// under the readable site+profile key.
func TestKeyringPutGetRoundTrip(t *testing.T) {
	keyring.MockInit()
	ref := keyringRef(t, "work", "https://company.atlassian.net")

	if err := (KeyringStore{}).Put(context.Background(), ref, "the-token"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	// The secret lives under the readable key — no digest.
	if v, err := keyring.Get(keyringService, "company.atlassian.net/work"); err != nil || v != "the-token" {
		t.Fatalf("keyring entry at company.atlassian.net/work = (%q,%v)", v, err)
	}
	got, err := (KeyringStore{}).Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "the-token" {
		t.Fatalf("Get() = %q, want the-token", got)
	}
}

// A missing keyring credential is reported as a typed CredentialError carrying
// the credential_missing code, an actionable message naming the profile, and
// wrapping ErrCredentialNotFound so callers can still match it with errors.Is.
func TestKeyringGetMissingIsTypedError(t *testing.T) {
	keyring.MockInit()
	ref := keyringRef(t, "work", "https://company.atlassian.net")

	_, err := (KeyringStore{}).Get(context.Background(), ref)
	if err == nil {
		t.Fatal("Get() of a missing credential error = nil")
	}
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get() error does not wrap ErrCredentialNotFound: %v", err)
	}
	var ce *CredentialError
	if !errors.As(err, &ce) {
		t.Fatalf("Get() error type = %T, want *CredentialError", err)
	}
	if ce.ErrCode != ErrorCodeCredentialMissing {
		t.Fatalf("Get() error code = %q, want credential_missing", ce.ErrCode)
	}
	if !strings.Contains(ce.Message, "work") {
		t.Fatalf("error message does not name the profile: %q", ce.Message)
	}
	if !strings.Contains(strings.ToLower(ce.Message+" "+ce.HintMsg), "auth login") {
		t.Fatalf("error does not point the user to `auth login`: message=%q hint=%q", ce.Message, ce.HintMsg)
	}
}

// There is NO legacy fallback: a credential stored by a pre-namespacing
// release under the bare profile name is NOT auto-resolved.
func TestKeyringGetHasNoLegacyFallback(t *testing.T) {
	keyring.MockInit()
	ref := keyringRef(t, "work", "https://company.atlassian.net")
	// Seed a bare-profile-name entry, the old pre-namespacing layout.
	if err := keyring.Set(keyringService, "work", "stale-legacy-token"); err != nil {
		t.Fatalf("seed bare entry error = %v", err)
	}

	_, err := (KeyringStore{}).Get(context.Background(), ref)
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get() resolved a legacy bare-profile entry; want ErrCredentialNotFound, got %v", err)
	}
}

// Delete removes the credential's own site+profile key. A missing entry is
// idempotent. Delete never touches any other key.
func TestKeyringDeleteRemovesOwnKeyOnly(t *testing.T) {
	keyring.MockInit()
	ref := keyringRef(t, "work", "https://company.atlassian.net")
	other := keyringRef(t, "work", "https://other.atlassian.net")
	if err := (KeyringStore{}).Put(context.Background(), ref, "ours"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}
	if err := (KeyringStore{}).Put(context.Background(), other, "theirs"); err != nil {
		t.Fatalf("seed other Put error = %v", err)
	}

	if err := (KeyringStore{}).Delete(context.Background(), ref); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := (KeyringStore{}).Get(context.Background(), ref); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Delete() did not remove its own key: %v", err)
	}
	// The other site's credential is untouched.
	if v, err := (KeyringStore{}).Get(context.Background(), other); err != nil || v != "theirs" {
		t.Fatalf("Delete() disturbed an unrelated site's credential: v=%q err=%v", v, err)
	}
	// A second Delete of the now-absent key is idempotent.
	if err := (KeyringStore{}).Delete(context.Background(), ref); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Delete() of an absent key = %v, want ErrCredentialNotFound", err)
	}
}

// RevokeProfileCredential deletes the credential jira-cli owns and reports
// removed=true with no informational note.
func TestRevokeProfileCredentialRemovesCredential(t *testing.T) {
	keyring.MockInit()
	ref := keyringRef(t, "work", "https://company.atlassian.net")
	if err := (KeyringStore{}).Put(context.Background(), ref, "the-token"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}

	removed, note, err := RevokeProfileCredential(context.Background(), KeyringStore{}, ref)
	if err != nil {
		t.Fatalf("RevokeProfileCredential() error = %v", err)
	}
	if !removed {
		t.Fatal("RevokeProfileCredential() reported removed=false for a stored credential")
	}
	if note != "" {
		t.Fatalf("RevokeProfileCredential() of a keyring credential returned a note: %q", note)
	}
	if _, err := (KeyringStore{}).Get(context.Background(), ref); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("credential still resolves after revocation: %v", err)
	}
}

// RevokeProfileCredential of a profile with no stored credential reports
// removed=false and no error: revocation is idempotent.
func TestRevokeProfileCredentialAbsentIsIdempotent(t *testing.T) {
	keyring.MockInit()
	ref := keyringRef(t, "work", "https://company.atlassian.net")

	removed, _, err := RevokeProfileCredential(context.Background(), KeyringStore{}, ref)
	if err != nil {
		t.Fatalf("RevokeProfileCredential() error = %v", err)
	}
	if removed {
		t.Fatal("RevokeProfileCredential() reported removed=true for an absent credential")
	}
}

// Revoking a 1Password-backed credential never destroys the 1Password item —
// not even one jira-cli created. The managed credential field is cleared, the
// item and every other field survive, and an informational note naming the
// item is returned so the caller can surface it.
func TestRevokeProfileCredentialKeepsOnePasswordItem(t *testing.T) {
	client := newFakeOnePasswordClient()
	// A pre-existing item carrying an unrelated user field.
	client.items["item-1"] = onepassword.Item{
		ID: "item-1", VaultID: "vault-1", Title: "my-jira-login",
		Fields: []onepassword.ItemField{{ID: "user-note", Title: "note", Value: "keep me"}},
	}
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "company.atlassian.net", Account: "Team", Vault: "Engineering", Item: "my-jira-login"}
	// jira-cli writes its credential field into the item.
	if err := store.Put(context.Background(), ref, "the-token"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}

	removed, note, err := RevokeProfileCredential(context.Background(), store, ref)
	if err != nil {
		t.Fatalf("RevokeProfileCredential() error = %v", err)
	}
	if !removed {
		t.Fatal("RevokeProfileCredential() reported removed=false after clearing the credential field")
	}
	if note == "" {
		t.Fatal("RevokeProfileCredential() of a 1Password credential returned no informational note")
	}
	if !strings.Contains(note, "my-jira-login") {
		t.Fatalf("RevokeProfileCredential() note does not name the kept item: %q", note)
	}
	survivor, ok := client.items["item-1"]
	if !ok {
		t.Fatal("RevokeProfileCredential() destroyed a 1Password item")
	}
	for _, f := range survivor.Fields {
		if f.ID == onePasswordCredentialFieldID {
			t.Fatalf("RevokeProfileCredential() left the managed credential field behind: %+v", survivor.Fields)
		}
	}
}

// Revoking a 1Password-backed credential stored in an item jira-cli itself
// created still keeps the item — only the managed credential field is cleared
// — and returns the informational note.
func TestRevokeProfileCredentialKeepsOnePasswordItemEvenWhenCreatedByJiraCLI(t *testing.T) {
	client := newFakeOnePasswordClient()
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "company.atlassian.net", Account: "Team", Vault: "Engineering", Item: "jira-cli-company.atlassian.net-work", ItemIsDefault: true}
	// Put creates the item.
	if err := store.Put(context.Background(), ref, "the-token"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}

	removed, note, err := RevokeProfileCredential(context.Background(), store, ref)
	if err != nil {
		t.Fatalf("RevokeProfileCredential() error = %v", err)
	}
	if !removed {
		t.Fatal("RevokeProfileCredential() reported removed=false after clearing the credential field")
	}
	if note == "" {
		t.Fatal("RevokeProfileCredential() of a 1Password credential returned no informational note")
	}
	if _, ok := client.items["item-1"]; !ok {
		t.Fatal("RevokeProfileCredential() destroyed a 1Password item jira-cli created")
	}
}
