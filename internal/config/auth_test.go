package config

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// failingStore wraps a MemoryCredentialStore and forces Put or Delete to fail
// so transactional migration behavior can be exercised.
type failingStore struct {
	*MemoryCredentialStore
	failPut    bool
	failDelete bool
}

func (s *failingStore) Put(ctx context.Context, ref SecretRef, secret string) error {
	if s.failPut {
		return errors.New("backend put failed")
	}
	return s.MemoryCredentialStore.Put(ctx, ref, secret)
}

func (s *failingStore) Delete(ctx context.Context, ref SecretRef) error {
	if s.failDelete {
		return errors.New("backend delete failed")
	}
	return s.MemoryCredentialStore.Delete(ctx, ref)
}

// rollbackFailingStore is a MemoryCredentialStore whose Delete always fails,
// so a transactional rollback that must undo a fresh write can be exercised.
type rollbackDeleteFailingStore struct {
	*MemoryCredentialStore
}

func (s *rollbackDeleteFailingStore) Delete(_ context.Context, _ SecretRef) error {
	return errors.New("backend delete failed")
}

// StoreCredentialTransactionally writes the credential, runs the metadata
// save, and on a save failure rolls the credential write back. When no prior
// credential existed, rollback removes the just-written one so a save failure
// never orphans a secret in the backend.
func TestStoreCredentialTransactionallySaveFailureRemovesFreshWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryCredentialStore()
	ref := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}

	err := StoreCredentialTransactionally(ctx, store, ref, "new-token", func() error {
		// The credential must already be staged when the save runs.
		if v, getErr := store.Get(ctx, ref); getErr != nil || v != "new-token" {
			t.Fatalf("credential not staged before save: v=%q err=%v", v, getErr)
		}
		return errors.New("config save failed")
	})
	if err == nil {
		t.Fatal("StoreCredentialTransactionally() error = nil, want the save failure")
	}
	// No orphaned credential: the fresh write was rolled back.
	if _, getErr := store.Get(ctx, ref); !errors.Is(getErr, ErrCredentialNotFound) {
		t.Fatalf("save failure left an orphaned credential in the backend: err=%v", getErr)
	}
}

// When a prior credential existed, a save failure restores that prior value
// rather than deleting it: the rollback returns the backend to its prior state.
func TestStoreCredentialTransactionallySaveFailureRestoresPriorValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryCredentialStore()
	ref := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	if err := store.Put(ctx, ref, "prior-token"); err != nil {
		t.Fatalf("seed prior credential error = %v", err)
	}

	err := StoreCredentialTransactionally(ctx, store, ref, "new-token", func() error {
		return errors.New("config save failed")
	})
	if err == nil {
		t.Fatal("StoreCredentialTransactionally() error = nil, want the save failure")
	}
	got, getErr := store.Get(ctx, ref)
	if getErr != nil {
		t.Fatalf("Get() after rollback error = %v", getErr)
	}
	if got != "prior-token" {
		t.Fatalf("save failure did not restore the prior credential: got %q, want prior-token", got)
	}
}

// A successful save commits the credential write and runs no rollback.
func TestStoreCredentialTransactionallySuccessCommitsWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryCredentialStore()
	ref := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}

	if err := StoreCredentialTransactionally(ctx, store, ref, "new-token", func() error { return nil }); err != nil {
		t.Fatalf("StoreCredentialTransactionally() error = %v", err)
	}
	got, getErr := store.Get(ctx, ref)
	if getErr != nil {
		t.Fatalf("Get() error = %v", getErr)
	}
	if got != "new-token" {
		t.Fatalf("credential = %q, want new-token", got)
	}
}

// A rollback that itself fails is surfaced, not swallowed: the returned error
// names both the save failure and the rollback failure.
func TestStoreCredentialTransactionallySurfacesRollbackFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := &rollbackDeleteFailingStore{MemoryCredentialStore: NewMemoryCredentialStore()}
	ref := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}

	err := StoreCredentialTransactionally(ctx, store, ref, "new-token", func() error {
		return errors.New("config save failed")
	})
	if err == nil {
		t.Fatal("StoreCredentialTransactionally() error = nil, want a combined failure")
	}
	msg := err.Error()
	if !strings.Contains(msg, "config save failed") {
		t.Fatalf("error does not name the save failure: %q", msg)
	}
	if !strings.Contains(msg, "rollback") || !strings.Contains(msg, "backend delete failed") {
		t.Fatalf("error does not surface the rollback failure: %q", msg)
	}
}

// A successful migration writes the destination before the metadata save and
// deletes the source only after the save succeeds.
func TestMigrateCredentialsStagesWriteBeforeSave(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := NewMemoryCredentialStore()
	dest := NewMemoryCredentialStore()
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "token-1"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}

	saved := false
	report, err := MigrateCredentials(ctx, []CredentialMigration{{
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error {
		// Destination must already hold the secret when the save runs.
		if v, getErr := dest.Get(ctx, dstRef); getErr != nil || v != "token-1" {
			t.Fatalf("destination not staged before save: v=%q err=%v", v, getErr)
		}
		saved = true
		return nil
	})
	if err != nil {
		t.Fatalf("MigrateCredentials error = %v", err)
	}
	if !saved {
		t.Fatal("save callback was not invoked")
	}
	if report.HasCleanupFailures() {
		t.Fatalf("unexpected cleanup failures: %+v", report)
	}
	if _, getErr := source.Get(ctx, srcRef); !errors.Is(getErr, ErrCredentialNotFound) {
		t.Fatalf("source secret not deleted after successful save: err=%v", getErr)
	}
}

// When the metadata save fails, the freshly written destination secret must
// be rolled back and the source secret left intact.
func TestMigrateCredentialsRollsBackOnSaveFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := NewMemoryCredentialStore()
	dest := NewMemoryCredentialStore()
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "token-1"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}

	_, err := MigrateCredentials(ctx, []CredentialMigration{{
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error {
		return errors.New("config save failed")
	})
	if err == nil {
		t.Fatal("MigrateCredentials error = nil, want save failure")
	}
	if _, getErr := dest.Get(ctx, dstRef); !errors.Is(getErr, ErrCredentialNotFound) {
		t.Fatalf("destination secret not rolled back after save failure: err=%v", getErr)
	}
	if v, getErr := source.Get(ctx, srcRef); getErr != nil || v != "token-1" {
		t.Fatalf("source secret was disturbed by a failed migration: v=%q err=%v", v, getErr)
	}
}

// A failure cleaning up an old secret after a durable save is reported, not
// hidden: the migration itself succeeds but the report names the failure.
func TestMigrateCredentialsReportsCleanupFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := &failingStore{MemoryCredentialStore: NewMemoryCredentialStore(), failDelete: true}
	dest := NewMemoryCredentialStore()
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example"}
	if err := source.MemoryCredentialStore.Put(ctx, srcRef, "token-1"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}

	report, err := MigrateCredentials(ctx, []CredentialMigration{{
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error { return nil })
	if err != nil {
		t.Fatalf("MigrateCredentials error = %v (cleanup failure must not fail the migration)", err)
	}
	if !report.HasCleanupFailures() {
		t.Fatalf("cleanup failure was hidden: report = %+v", report)
	}
}

// A destination write failure aborts the migration before any save, and the
// source secret is left untouched.
func TestMigrateCredentialsAbortsOnDestinationWriteFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := NewMemoryCredentialStore()
	dest := &failingStore{MemoryCredentialStore: NewMemoryCredentialStore(), failPut: true}
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "token-1"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}

	saved := false
	_, err := MigrateCredentials(ctx, []CredentialMigration{{
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error {
		saved = true
		return nil
	})
	if err == nil {
		t.Fatal("MigrateCredentials error = nil, want destination write failure")
	}
	if saved {
		t.Fatal("save callback ran despite a destination write failure")
	}
	if v, getErr := source.Get(ctx, srcRef); getErr != nil || v != "token-1" {
		t.Fatalf("source secret disturbed by an aborted migration: v=%q err=%v", v, getErr)
	}
}

// When the destination already holds a credential and the metadata save
// fails, rollback must RESTORE the destination's prior value, not delete it.
// Staging overwrites the destination, so a blind Delete on rollback would
// destroy a real pre-existing secret.
func TestMigrateCredentialsRollbackRestoresPriorDestinationSecret(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := NewMemoryCredentialStore()
	dest := NewMemoryCredentialStore()
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "new-token"); err != nil {
		t.Fatalf("seed source Put error = %v", err)
	}
	// The destination already has a real, pre-existing credential.
	if err := dest.Put(ctx, dstRef, "prior-dest-secret"); err != nil {
		t.Fatalf("seed dest Put error = %v", err)
	}

	_, err := MigrateCredentials(ctx, []CredentialMigration{{
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error {
		return errors.New("config save failed")
	})
	if err == nil {
		t.Fatal("MigrateCredentials error = nil, want save failure")
	}
	if v, getErr := dest.Get(ctx, dstRef); getErr != nil || v != "prior-dest-secret" {
		t.Fatalf("rollback destroyed the pre-existing destination secret: v=%q err=%v", v, getErr)
	}
	if v, getErr := source.Get(ctx, srcRef); getErr != nil || v != "new-token" {
		t.Fatalf("source secret was disturbed: v=%q err=%v", v, getErr)
	}
}

// When the destination had NO prior credential, rollback must delete the
// staged write and leave nothing behind.
func TestMigrateCredentialsRollbackDeletesWhenNoPriorDestination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := NewMemoryCredentialStore()
	dest := NewMemoryCredentialStore()
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "new-token"); err != nil {
		t.Fatalf("seed source Put error = %v", err)
	}

	_, err := MigrateCredentials(ctx, []CredentialMigration{{
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error {
		return errors.New("config save failed")
	})
	if err == nil {
		t.Fatal("MigrateCredentials error = nil, want save failure")
	}
	if _, getErr := dest.Get(ctx, dstRef); !errors.Is(getErr, ErrCredentialNotFound) {
		t.Fatalf("rollback left a staged secret behind: err=%v", getErr)
	}
}

// rollbackFailingStore stages a Put normally but fails every subsequent
// rollback Put or Delete, so MigrateCredentials' rollback-error reporting can
// be exercised.
type rollbackFailingStore struct {
	*MemoryCredentialStore
	staged bool
}

func (s *rollbackFailingStore) Put(ctx context.Context, ref SecretRef, secret string) error {
	if s.staged {
		// This is a rollback restore — fail it.
		return errors.New("rollback put failed")
	}
	s.staged = true
	return s.MemoryCredentialStore.Put(ctx, ref, secret)
}

func (s *rollbackFailingStore) Delete(_ context.Context, _ SecretRef) error {
	// Rollback delete of a staged write — fail it.
	return errors.New("rollback delete failed")
}

// A rollback step that itself fails must not be swallowed: MigrateCredentials
// must return a combined error naming the affected profile, so the caller is
// not told the migration was cleanly rolled back when a destination write
// could not be undone.
func TestMigrateCredentialsSurfacesRollbackFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := NewMemoryCredentialStore()
	dest := &rollbackFailingStore{MemoryCredentialStore: NewMemoryCredentialStore()}
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "token-1"); err != nil {
		t.Fatalf("seed Put error = %v", err)
	}

	_, err := MigrateCredentials(ctx, []CredentialMigration{{
		Profile:     "work",
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error {
		return errors.New("config save failed")
	})
	if err == nil {
		t.Fatal("MigrateCredentials error = nil, want a combined save+rollback failure")
	}
	if !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("error does not surface the rollback failure: %v", err)
	}
	if !strings.Contains(err.Error(), "work") {
		t.Fatalf("error does not name the affected profile: %v", err)
	}
}

// CredentialIdentity keys a credential by site host + profile. Distinct
// profile names on the same site must produce distinct keyring keys.
func TestCredentialIdentityKeyringKeysDoNotCollide(t *testing.T) {
	t.Parallel()
	seen := map[string]string{}
	for _, name := range []string{"work", "work-staging", "work_staging", "workstaging"} {
		ref, err := CredentialIdentity(Profile{Name: name, BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendKeyring})
		if err != nil {
			t.Fatalf("CredentialIdentity(%q) error = %v", name, err)
		}
		key := ref.KeyringName()
		if other, dup := seen[key]; dup {
			t.Fatalf("profile %q and %q produced the same keyring key %q", name, other, key)
		}
		seen[key] = name
	}
}

// The keyring key is the readable site host + profile, with no digest.
func TestCredentialIdentityKeyringKeyIsReadable(t *testing.T) {
	t.Parallel()
	ref, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity() error = %v", err)
	}
	if got := ref.KeyringName(); got != "company.atlassian.net/work" {
		t.Fatalf("KeyringName() = %q, want company.atlassian.net/work", got)
	}
}

// A schemeless / shorthand base URL is normalized to the same host as its
// canonical form, so the credential key is stable however the URL is spelled.
func TestCredentialIdentityNormalizesHost(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"company", "company.atlassian.net", "https://company.atlassian.net", "https://company.atlassian.net/"} {
		ref, err := CredentialIdentity(Profile{Name: "work", BaseURL: base, SecretBackend: SecretBackendKeyring})
		if err != nil {
			t.Fatalf("CredentialIdentity(%q) error = %v", base, err)
		}
		if got := ref.KeyringName(); got != "company.atlassian.net/work" {
			t.Fatalf("base %q produced keyring key %q, want company.atlassian.net/work", base, got)
		}
	}
}

// Two sites with an identically named profile must not share a keyring key:
// the key includes the site host.
func TestCredentialIdentitySeparatesSites(t *testing.T) {
	t.Parallel()
	a, err := CredentialIdentity(Profile{Name: "default", BaseURL: "https://one.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(a) error = %v", err)
	}
	b, err := CredentialIdentity(Profile{Name: "default", BaseURL: "https://two.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(b) error = %v", err)
	}
	if a.KeyringName() == b.KeyringName() {
		t.Fatalf("identically named profiles for different sites share keyring key %q", a.KeyringName())
	}
}

// CredentialIdentitiesDiffer reports whether two SecretRefs address different
// credential storage. A base_url change re-derives the site host, which is
// part of the keyring key, so the credential under the old identity is
// orphaned — auth login warns the user instead of silently stranding it.
func TestCredentialIdentitiesDifferOnHostChange(t *testing.T) {
	t.Parallel()
	oldRef, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://one.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(old) error = %v", err)
	}
	newRef, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://two.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(new) error = %v", err)
	}
	if !CredentialIdentitiesDiffer(oldRef, newRef) {
		t.Fatal("CredentialIdentitiesDiffer() = false for a base_url host change, want true")
	}
}

// Re-running auth login without changing the site must not be reported as an
// identity change: the credential stays reachable, nothing is orphaned.
func TestCredentialIdentitiesDoNotDifferWhenHostUnchanged(t *testing.T) {
	t.Parallel()
	oldRef, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(old) error = %v", err)
	}
	// Same site, spelled in shorthand — still the same host.
	newRef, err := CredentialIdentity(Profile{Name: "work", BaseURL: "company", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(new) error = %v", err)
	}
	if CredentialIdentitiesDiffer(oldRef, newRef) {
		t.Fatal("CredentialIdentitiesDiffer() = true for an unchanged site, want false")
	}
}

// Switching the secret backend re-points the credential to entirely different
// storage, so the identity differs and the old backend's credential must be
// revoked.
func TestCredentialIdentitiesDifferOnBackendChange(t *testing.T) {
	t.Parallel()
	keyringRef, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(keyring) error = %v", err)
	}
	onePasswordRef, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendOnePassword, Vault: "Engineering"})
	if err != nil {
		t.Fatalf("CredentialIdentity(1password) error = %v", err)
	}
	if !CredentialIdentitiesDiffer(keyringRef, onePasswordRef) {
		t.Fatal("CredentialIdentitiesDiffer() = false for a secret-backend change, want true")
	}
}

// Changing only the 1Password account re-points the credential at a different
// 1Password integration, so the identity differs even though host, vault, and
// item are unchanged.
func TestCredentialIdentitiesDifferOnOnePasswordAccountChange(t *testing.T) {
	t.Parallel()
	oldRef, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendOnePassword, Vault: "Engineering", OnePasswordAccount: "old.1password.com"})
	if err != nil {
		t.Fatalf("CredentialIdentity(old) error = %v", err)
	}
	newRef, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendOnePassword, Vault: "Engineering", OnePasswordAccount: "new.1password.com"})
	if err != nil {
		t.Fatalf("CredentialIdentity(new) error = %v", err)
	}
	if !CredentialIdentitiesDiffer(oldRef, newRef) {
		t.Fatal("CredentialIdentitiesDiffer() = false for a 1Password account change, want true")
	}
}

// The default 1Password item title is site-scoped, so two sites that share a
// profile name and one vault no longer collide on the same item.
func TestCredentialIdentityOnePasswordItemIsSiteScoped(t *testing.T) {
	t.Parallel()
	a, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://one.atlassian.net", SecretBackend: SecretBackendOnePassword})
	if err != nil {
		t.Fatalf("CredentialIdentity(a) error = %v", err)
	}
	b, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://two.atlassian.net", SecretBackend: SecretBackendOnePassword})
	if err != nil {
		t.Fatalf("CredentialIdentity(b) error = %v", err)
	}
	if !a.ItemIsDefault || !b.ItemIsDefault {
		t.Fatal("a default-named 1Password item must be marked ItemIsDefault")
	}
	if a.Item == b.Item {
		t.Fatalf("default 1Password item title is not site-scoped: both = %q", a.Item)
	}
	if !strings.Contains(a.Item, "work") {
		t.Fatalf("default 1Password item title does not include the profile: %q", a.Item)
	}
}

// A user-supplied 1Password item name is used verbatim and is NOT marked as a
// jira-cli-owned default.
func TestCredentialIdentityUserItemIsNotDefault(t *testing.T) {
	t.Parallel()
	ref, err := CredentialIdentity(Profile{Name: "work", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendOnePassword, Item: "my-login"})
	if err != nil {
		t.Fatalf("CredentialIdentity() error = %v", err)
	}
	if ref.Item != "my-login" {
		t.Fatalf("Item = %q, want my-login", ref.Item)
	}
	if ref.ItemIsDefault {
		t.Fatal("a user-named 1Password item must not be marked ItemIsDefault")
	}
}

// EnvTokenKey must be bijective: profile names that differ only by case,
// hyphen, or underscore must map to distinct env var keys, or
// CredentialIdentity must reject the name.
func TestEnvTokenKeyIsBijectiveOrRejected(t *testing.T) {
	t.Parallel()
	keys := map[string][]string{}
	for _, name := range []string{"work", "Work", "work-staging", "work_staging"} {
		ref, err := CredentialIdentity(Profile{Name: name, BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendKeyring})
		if err != nil {
			// A rejected name is acceptable; it just must not collide.
			continue
		}
		key := ref.EnvTokenKey()
		keys[key] = append(keys[key], name)
	}
	for key, names := range keys {
		if len(names) > 1 {
			t.Fatalf("env token key %q is shared by non-bijective profile names %v", key, names)
		}
	}
}

// A profile name that cannot be encoded into a safe keyring/env key is
// rejected with a typed CredentialError carrying the namespace-collision code.
func TestCredentialIdentityRejectsUnsafeProfileName(t *testing.T) {
	t.Parallel()
	_, err := CredentialIdentity(Profile{Name: "bad name/with:colon", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err == nil {
		t.Fatal("CredentialIdentity() error = nil for an unsafe profile name")
	}
	var ce *CredentialError
	if !errors.As(err, &ce) || ce.ErrCode != ErrorCodeCredentialNamespaceCollision {
		t.Fatalf("CredentialIdentity() error = %v, want a namespace-collision CredentialError", err)
	}
}

// envToken resolution must use the bijective key so a JIRA_TOKEN_* override
// for one profile cannot bleed into a sibling whose name differs only by a
// hyphen/underscore swap.
func TestResolveCredentialEnvLookupIsProfileSpecific(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryCredentialStore()
	hyphen, err := CredentialIdentity(Profile{Name: "work-staging", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(work-staging) error = %v", err)
	}
	underscore, err := CredentialIdentity(Profile{Name: "work_staging", BaseURL: "https://company.atlassian.net", SecretBackend: SecretBackendKeyring})
	if err != nil {
		t.Fatalf("CredentialIdentity(work_staging) error = %v", err)
	}
	t.Setenv(hyphen.EnvTokenKey(), "hyphen-token")
	// The underscore profile has no env override; resolution must fall through
	// to the store (and miss), not pick up the hyphen profile's env token.
	if _, err := ResolveCredential(ctx, store, underscore); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("ResolveCredential(work_staging) error = %v, want ErrCredentialNotFound", err)
	}
	got, err := ResolveCredential(ctx, store, hyphen)
	if err != nil {
		t.Fatalf("ResolveCredential(work-staging) error = %v", err)
	}
	if got != "hyphen-token" {
		t.Fatalf("ResolveCredential(work-staging) = %q, want hyphen-token", got)
	}
}

// ReadSecret is the canonical secret reader. It must trim only the CLI record
// delimiter (a trailing CR/LF), never interior or otherwise meaningful bytes
// of the token.
func TestReadSecretTrimsOnlyRecordDelimiter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trailing newline", "tok-en\n", "tok-en"},
		{"trailing crlf", "tok-en\r\n", "tok-en"},
		{"interior spaces preserved", "tok en\n", "tok en"},
		{"leading space preserved", " tok-en\n", " tok-en"},
		{"trailing tab preserved", "tok-en\t\n", "tok-en\t"},
		{"no delimiter", "tok-en", "tok-en"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadSecret(tc.in)
			if err != nil {
				t.Fatalf("ReadSecret(%q) error = %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ReadSecret(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ReadSecret must reject an explicitly empty credential rather than treating
// it as a stored value.
func TestReadSecretRejectsExplicitEmpty(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "\n", "\r\n", "   \n"} {
		if _, err := ReadSecret(in); err == nil {
			t.Fatalf("ReadSecret(%q) error = nil, want empty-credential rejection", in)
		}
	}
}

// Deleting a credential that is not present must be idempotent.
func TestMemoryStoreDeleteMissingIsHarmless(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryCredentialStore()
	ref := SecretRef{Profile: "ghost", Backend: SecretBackendKeyring, Host: "x.example"}
	if err := store.Delete(ctx, ref); err != nil {
		t.Fatalf("Delete() of a missing credential error = %v", err)
	}
}

// Config.Validate must reject an unsupported secret_backend rather than
// silently coercing an unknown value to keyring.
func TestValidateRejectsUnknownSecretBackend(t *testing.T) {
	t.Parallel()
	cfg := Config{
		DefaultProfile: "default",
		Profiles: []Profile{{
			Name:          "default",
			BaseURL:       "https://company.atlassian.net",
			AuthType:      AuthTypeToken,
			SecretBackend: SecretBackend("vault"),
		}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil for unsupported secret_backend")
	}
	if !strings.Contains(err.Error(), "vault") {
		t.Fatalf("Validate() error %q does not name the bad backend", err)
	}
}

// An empty secret_backend is still allowed and defaults to keyring; only an
// explicitly unknown value is rejected.
func TestValidateDefaultsEmptySecretBackend(t *testing.T) {
	t.Parallel()
	cfg := Config{
		DefaultProfile: "default",
		Profiles: []Profile{{
			Name:     "default",
			BaseURL:  "https://company.atlassian.net",
			AuthType: AuthTypeToken,
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v for an empty secret_backend", err)
	}
	if cfg.Profiles[0].SecretBackend != SecretBackendKeyring {
		t.Fatalf("empty secret_backend = %q, want keyring", cfg.Profiles[0].SecretBackend)
	}
}
