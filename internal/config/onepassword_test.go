//go:build cgo || windows

package config

import (
	"context"
	"errors"
	"strings"
	"testing"

	onepassword "github.com/1password/onepassword-sdk-go"
)

// Deleting a 1Password item that does not exist must be idempotent: Delete
// returns ErrCredentialNotFound so callers normalize it to success.
func TestOnePasswordStoreDeleteMissingItemIsIdempotent(t *testing.T) {
	client := newFakeOnePasswordClient()
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira-cli-work"}

	err := store.Delete(context.Background(), ref)
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Delete() of a missing item error = %v, want ErrCredentialNotFound", err)
	}
}

func TestOnePasswordStoreResolvesCredentialReference(t *testing.T) {
	client := newFakeOnePasswordClient()
	client.secret = "sdk-secret"
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira work"}
	// Get confirms the item exists before resolving its value, so seed it.
	if err := store.Put(context.Background(), ref, "sdk-secret"); err != nil {
		t.Fatalf("seed Put() error = %v", err)
	}

	got, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "sdk-secret" {
		t.Fatalf("Get() = %q", got)
	}
	if client.resolvedRef != "op://Engineering/jira%20work/jira-cli-credential" {
		t.Fatalf("resolved reference = %q", client.resolvedRef)
	}
}

func TestOnePasswordStoreUpsertsCredentialItem(t *testing.T) {
	client := newFakeOnePasswordClient()
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira-cli-work"}

	if err := store.Put(context.Background(), ref, "sdk-secret"); err != nil {
		t.Fatalf("Put() create error = %v", err)
	}
	if len(client.created) != 1 {
		t.Fatalf("created items = %d", len(client.created))
	}
	created := client.created[0]
	if created.VaultID != "vault-1" || created.Title != "jira-cli-work" || created.Category != onepassword.ItemCategoryPassword {
		t.Fatalf("created item = %+v", created)
	}
	// The managed credential field is identified by a stable jira-cli-owned
	// field ID, not by its display title.
	if len(created.Fields) != 1 || created.Fields[0].ID != onePasswordCredentialFieldID || created.Fields[0].FieldType != onepassword.ItemFieldTypeConcealed || created.Fields[0].Value != "sdk-secret" {
		t.Fatalf("created credential field = %+v", created.Fields)
	}

	if err := store.Put(context.Background(), ref, "rotated-secret"); err != nil {
		t.Fatalf("Put() update error = %v", err)
	}
	if len(client.updated) != 1 {
		t.Fatalf("updated items = %d", len(client.updated))
	}
	updated := client.updated[0]
	if updated.ID != "item-1" || updated.Fields[0].Value != "rotated-secret" {
		t.Fatalf("updated item = %+v", updated)
	}
}

// Delete must NEVER destroy a 1Password item — not even one jira-cli itself
// created. A 1Password item is a user-owned object; revoking a credential
// removes only jira-cli's own managed credential field and leaves the item,
// with every other field and all its content, in place.
func TestOnePasswordStoreDeleteNeverDestroysCreatedItem(t *testing.T) {
	client := newFakeOnePasswordClient()
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira-cli-work", ItemIsDefault: true}

	// Put creates the item.
	if err := store.Put(context.Background(), ref, "sdk-secret"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	// The item itself must survive — Delete never hard-deletes.
	survivor, ok := client.items["item-1"]
	if !ok {
		t.Fatal("Delete() destroyed a 1Password item; it must only strip the managed field")
	}
	// Only the managed credential field is gone.
	for _, f := range survivor.Fields {
		if f.ID == onePasswordCredentialFieldID {
			t.Fatalf("Delete() left the managed credential field behind: %+v", survivor.Fields)
		}
	}
}

// Delete of a pre-existing item jira-cli did not create removes only the
// managed credential field and leaves the item, and every other field, intact.
func TestOnePasswordStoreDeleteStripsOnlyManagedField(t *testing.T) {
	client := newFakeOnePasswordClient()
	// A pre-existing item the user created themselves, which happens to use a
	// title matching jira-cli's default scheme.
	client.items["item-1"] = onepassword.Item{
		ID: "item-1", VaultID: "vault-1", Title: "jira-cli-work",
		Fields: []onepassword.ItemField{
			{ID: onePasswordCredentialFieldID, Title: "credential", FieldType: onepassword.ItemFieldTypeConcealed, Value: "secret"},
			{ID: "user-note", Title: "note", Value: "keep me"},
		},
	}
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira-cli-work", ItemIsDefault: true}

	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	survivor, ok := client.items["item-1"]
	if !ok {
		t.Fatal("Delete() removed a 1Password item, want it preserved")
	}
	var sawCredential, sawUserNote bool
	for _, f := range survivor.Fields {
		if f.ID == onePasswordCredentialFieldID {
			sawCredential = true
		}
		if f.ID == "user-note" {
			sawUserNote = true
			if f.Value != "keep me" {
				t.Fatalf("Delete() disturbed an unrelated user field: %+v", survivor.Fields)
			}
		}
	}
	if sawCredential {
		t.Fatalf("Delete() left the managed credential field behind: %+v", survivor.Fields)
	}
	if !sawUserNote {
		t.Fatalf("Delete() dropped an unrelated user field: %+v", survivor.Fields)
	}
}

// A user's own field merely TITLED "credential" but carrying a non-jira-cli
// field ID is not jira-cli's: login must not overwrite it and logout must not
// remove it. Only the field with the stable jira-cli-owned ID is managed.
func TestOnePasswordStoreFieldOwnershipIsByIDNotTitle(t *testing.T) {
	client := newFakeOnePasswordClient()
	// A user-named item that already holds a user field titled "credential"
	// with the user's own field ID — not jira-cli's owned ID.
	client.items["item-1"] = onepassword.Item{
		ID: "item-1", VaultID: "vault-1", Title: "my-jira-login",
		Fields: []onepassword.ItemField{
			{ID: "user-credential-field", Title: "credential", FieldType: onepassword.ItemFieldTypeConcealed, Value: "user-owned-value"},
		},
	}
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "my-jira-login"}

	// Login writes jira-cli's credential — into its OWN field, not the user's.
	if err := store.Put(context.Background(), ref, "jira-cli-secret"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	afterPut := client.items["item-1"]
	var userField, jiraField *onepassword.ItemField
	for i := range afterPut.Fields {
		switch afterPut.Fields[i].ID {
		case "user-credential-field":
			userField = &afterPut.Fields[i]
		case onePasswordCredentialFieldID:
			jiraField = &afterPut.Fields[i]
		}
	}
	if userField == nil || userField.Value != "user-owned-value" {
		t.Fatalf("Put() overwrote a user field titled \"credential\": %+v", afterPut.Fields)
	}
	if jiraField == nil || jiraField.Value != "jira-cli-secret" {
		t.Fatalf("Put() did not write the credential to the jira-cli-owned field ID: %+v", afterPut.Fields)
	}

	// Logout removes only jira-cli's owned field; the user's field survives.
	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	survivor := client.items["item-1"]
	var sawUser, sawJira bool
	for _, f := range survivor.Fields {
		if f.ID == "user-credential-field" {
			sawUser = true
			if f.Value != "user-owned-value" {
				t.Fatalf("Delete() disturbed the user field titled \"credential\": %+v", survivor.Fields)
			}
		}
		if f.ID == onePasswordCredentialFieldID {
			sawJira = true
		}
	}
	if !sawUser {
		t.Fatalf("Delete() removed a user field merely titled \"credential\": %+v", survivor.Fields)
	}
	if sawJira {
		t.Fatalf("Delete() left the jira-cli-owned credential field behind: %+v", survivor.Fields)
	}
}

// The managed credential field round-trips by its stable jira-cli-owned ID:
// the field jira-cli writes on Put is the field whose value Get reads back,
// identified by ID rather than by display title.
func TestOnePasswordStoreOwnedCredentialFieldRoundTripsByID(t *testing.T) {
	client := newFakeOnePasswordClient()
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira-cli-work", ItemIsDefault: true}

	if err := store.Put(context.Background(), ref, "round-trip-secret"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	// The credential lives in the field with the jira-cli-owned ID.
	stored := client.items["item-1"]
	var byID string
	found := false
	for _, f := range stored.Fields {
		if f.ID == onePasswordCredentialFieldID {
			byID, found = f.Value, true
		}
	}
	if !found || byID != "round-trip-secret" {
		t.Fatalf("credential was not written to the owned field ID %q: %+v", onePasswordCredentialFieldID, stored.Fields)
	}
	client.secret = "round-trip-secret"
	got, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "round-trip-secret" {
		t.Fatalf("Get() = %q, want round-trip-secret", got)
	}
}

// Delete of a user-named item that has no jira-cli credential field is
// idempotent: there is nothing to strip, the item is left untouched.
func TestOnePasswordStoreDeleteOfUserNamedItemWithoutCredentialField(t *testing.T) {
	client := newFakeOnePasswordClient()
	client.items["item-1"] = onepassword.Item{
		ID: "item-1", VaultID: "vault-1", Title: "my-jira-login",
		Fields: []onepassword.ItemField{{ID: "notes", Title: "notes", Value: "keep me"}},
	}
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "my-jira-login"}

	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := client.items["item-1"]; !ok {
		t.Fatal("Delete() removed a user-named item that had no credential field")
	}
}

// With no service-account token in the env and no desktop-app account on the
// ref, the SDK store has no auth source: operations surface a typed error.
func TestOnePasswordStoreRequiresAuthSource(t *testing.T) {
	t.Setenv(onePasswordServiceAccountTokenEnv, "")
	store := OnePasswordStore{}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Vault: "Engineering", Item: "jira-cli-work"}

	err := store.Put(context.Background(), ref, "sdk-secret")
	if err == nil {
		t.Fatal("Put() error = nil")
	}
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Put() error = %v, want ErrCredentialNotFound", err)
	}
}

// A 1Password Get of an item absent from the vault must normalize to
// ErrCredentialNotFound — the SDK exposes no typed not-found error, so the
// structural existence check is the deterministic way to report absence.
func TestOnePasswordStoreGetMissingItemIsNotFound(t *testing.T) {
	client := newFakeOnePasswordClient() // empty vault: no items
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira-cli-work"}

	_, err := store.Get(context.Background(), ref)
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get() of a missing item = %v, want ErrCredentialNotFound", err)
	}
}

// A 1Password vault with two items sharing the requested title is ambiguous:
// the store rejects it with a typed CredentialError rather than silently
// resolving to whichever item was listed first.
func TestOnePasswordStoreRejectsAmbiguousItem(t *testing.T) {
	client := newFakeOnePasswordClient()
	client.items["item-a"] = onepassword.Item{ID: "item-a", VaultID: "vault-1", Title: "jira"}
	client.items["item-b"] = onepassword.Item{ID: "item-b", VaultID: "vault-1", Title: "jira"}
	store := OnePasswordStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira"}

	_, err := store.Get(context.Background(), ref)
	if err == nil {
		t.Fatal("Get() of an ambiguous item error = nil")
	}
	var ce *CredentialError
	if !errors.As(err, &ce) {
		t.Fatalf("Get() error type = %T, want *CredentialError", err)
	}
	if ce.ErrCode != ErrorCodeOnePasswordItemAmbiguous {
		t.Fatalf("Get() error code = %q, want onepassword_item_ambiguous", ce.ErrCode)
	}
}

// A keyring -> 1Password migration into a FRESH vault must succeed: the
// destination snapshot sees no prior item (ErrCredentialNotFound), so the
// migration proceeds and Put creates the item.
func TestMigrateKeyringToOnePasswordEmptyVault(t *testing.T) {
	ctx := context.Background()

	source := NewMemoryCredentialStore()
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "migrated-token"); err != nil {
		t.Fatalf("seed source error = %v", err)
	}

	client := newFakeOnePasswordClient() // fresh vault: no items
	dest := OnePasswordStore{Client: client}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example", Account: "Team", Vault: "Engineering", Item: "jira-cli-work", ItemIsDefault: true}

	report, err := MigrateCredentials(ctx, []CredentialMigration{{
		Profile:     "work",
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error { return nil })
	if err != nil {
		t.Fatalf("MigrateCredentials() into a fresh 1Password vault error = %v", err)
	}
	if report.HasCleanupFailures() {
		t.Fatalf("unexpected cleanup failures: %+v", report)
	}
	if len(client.created) != 1 {
		t.Fatalf("migration created %d 1Password items, want 1", len(client.created))
	}
	if client.created[0].Title != "jira-cli-work" {
		t.Fatalf("created item title = %q, want jira-cli-work", client.created[0].Title)
	}
}

// A 1Password migration whose config save fails AFTER a staged create, with NO
// pre-existing destination item, rolls back by stripping the managed
// credential field — never by destroying the staged item. A staged item left
// field-less after a rolled-back migration is acceptable; the item survives.
func TestMigrateOnePasswordRollbackStripsStagedField(t *testing.T) {
	ctx := context.Background()

	source := NewMemoryCredentialStore()
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "migrated-token"); err != nil {
		t.Fatalf("seed source error = %v", err)
	}

	client := newFakeOnePasswordClient()
	dest := OnePasswordStore{Client: client}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example", Account: "Team", Vault: "Engineering", Item: "jira-cli-work", ItemIsDefault: true}

	_, err := MigrateCredentials(ctx, []CredentialMigration{{
		Profile:     "work",
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error { return errors.New("config save failed") })
	if err == nil {
		t.Fatal("MigrateCredentials() error = nil, want save failure")
	}
	// The staged item must still exist — rollback never destroys an item.
	staged, ok := client.items["item-1"]
	if !ok {
		t.Fatal("rollback destroyed the staged 1Password item, want it preserved field-less")
	}
	// The staged credential field must be gone after rollback.
	for _, f := range staged.Fields {
		if f.ID == onePasswordCredentialFieldID {
			t.Fatalf("rollback left the staged credential field behind: %+v", staged.Fields)
		}
	}
}

// A 1Password migration whose config save fails AFTER staging, WITH a
// pre-existing destination item, must roll back by restoring the prior item
// value in place — no duplicate item, prior secret intact.
func TestMigrateOnePasswordRollbackRestoresPriorItem(t *testing.T) {
	ctx := context.Background()

	source := NewMemoryCredentialStore()
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}
	if err := source.Put(ctx, srcRef, "migrated-token"); err != nil {
		t.Fatalf("seed source error = %v", err)
	}

	client := newFakeOnePasswordClient()
	dest := OnePasswordStore{Client: client}
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example", Account: "Team", Vault: "Engineering", Item: "jira-cli-work", ItemIsDefault: true}
	// The destination item already exists with a prior value.
	if err := dest.Put(ctx, dstRef, "prior-dest-secret"); err != nil {
		t.Fatalf("seed destination item error = %v", err)
	}

	_, err := MigrateCredentials(ctx, []CredentialMigration{{
		Profile:     "work",
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error { return errors.New("config save failed") })
	if err == nil {
		t.Fatal("MigrateCredentials() error = nil, want save failure")
	}
	// Exactly one item, restored to the prior value.
	if len(client.items) != 1 {
		t.Fatalf("vault holds %d items after rollback, want 1 (no duplicate)", len(client.items))
	}
	client.secret = "prior-dest-secret"
	got, getErr := dest.Get(ctx, dstRef)
	if getErr != nil {
		t.Fatalf("Get() after rollback error = %v", getErr)
	}
	if got != "prior-dest-secret" {
		t.Fatalf("rollback did not restore the prior 1Password item value: got %q", got)
	}
}

// Migrating AWAY from a 1Password source strips the managed credential field
// from the source item but never destroys the item — even when jira-cli
// created it. The item survives, and a note naming it is emitted.
func TestMigrateAwayFromOnePasswordStripsFieldKeepsItem(t *testing.T) {
	ctx := context.Background()

	client := newFakeOnePasswordClient()
	source := OnePasswordStore{Client: client}
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example", Account: "Team", Vault: "Engineering", Item: "jira-cli-work", ItemIsDefault: true}
	if err := source.Put(ctx, srcRef, "old-token"); err != nil {
		t.Fatalf("seed source item error = %v", err)
	}

	dest := NewMemoryCredentialStore()
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}

	report, err := MigrateCredentials(ctx, []CredentialMigration{{
		Profile:     "work",
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error { return nil })
	if err != nil {
		t.Fatalf("MigrateCredentials() error = %v", err)
	}
	if report.HasCleanupFailures() {
		t.Fatalf("unexpected cleanup failures: %+v", report)
	}
	// The source item must survive — migrate-away never destroys an item.
	survivor, ok := client.items["item-1"]
	if !ok {
		t.Fatal("migrate-away destroyed the source 1Password item, want it preserved")
	}
	for _, f := range survivor.Fields {
		if f.ID == onePasswordCredentialFieldID {
			t.Fatalf("migrate-away left the managed credential field on the source item: %+v", survivor.Fields)
		}
	}
	// An informational note must name the item.
	if len(report.CleanupNotes) != 1 {
		t.Fatalf("migrate-away emitted %d cleanup notes, want 1", len(report.CleanupNotes))
	}
	if !strings.Contains(report.CleanupNotes[0].Message, "jira-cli-work") {
		t.Fatalf("cleanup note does not name the source item: %q", report.CleanupNotes[0].Message)
	}
}

// Migrating AWAY from a 1Password source item that also carries unrelated user
// fields strips only the managed credential field and preserves the rest.
func TestMigrateAwayFromOnePasswordPreservesUserFields(t *testing.T) {
	ctx := context.Background()

	client := newFakeOnePasswordClient()
	client.items["item-1"] = onepassword.Item{
		ID: "item-1", VaultID: "vault-1", Title: "my-jira-login",
		Fields: []onepassword.ItemField{{ID: "user-note", Title: "note", Value: "keep me"}},
	}
	source := OnePasswordStore{Client: client}
	srcRef := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Host: "x.example", Account: "Team", Vault: "Engineering", Item: "my-jira-login", ItemIsDefault: false}
	if err := source.Put(ctx, srcRef, "old-token"); err != nil {
		t.Fatalf("seed source item error = %v", err)
	}

	dest := NewMemoryCredentialStore()
	dstRef := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}

	report, err := MigrateCredentials(ctx, []CredentialMigration{{
		Profile:     "work",
		Source:      source,
		Destination: dest,
		SourceRef:   srcRef,
		DestRef:     dstRef,
	}}, func() error { return nil })
	if err != nil {
		t.Fatalf("MigrateCredentials() error = %v", err)
	}
	if report.HasCleanupFailures() {
		t.Fatalf("unexpected cleanup failures: %+v", report)
	}
	survivor, ok := client.items["item-1"]
	if !ok {
		t.Fatal("migrate-away destroyed the source 1Password item, want it preserved")
	}
	var sawNote, sawCredential bool
	for _, f := range survivor.Fields {
		if f.ID == "user-note" {
			sawNote = true
			if f.Value != "keep me" {
				t.Fatalf("migrate-away disturbed an unrelated user field: %+v", survivor.Fields)
			}
		}
		if f.ID == onePasswordCredentialFieldID {
			sawCredential = true
		}
	}
	if !sawNote {
		t.Fatalf("migrate-away dropped an unrelated user field: %+v", survivor.Fields)
	}
	if sawCredential {
		t.Fatalf("migrate-away left the managed credential field behind: %+v", survivor.Fields)
	}
	if len(report.CleanupNotes) != 1 {
		t.Fatalf("migrate-away emitted %d cleanup notes, want 1", len(report.CleanupNotes))
	}
	if !strings.Contains(report.CleanupNotes[0].Message, "my-jira-login") {
		t.Fatalf("cleanup note does not name the source item: %q", report.CleanupNotes[0].Message)
	}
}

type fakeOnePasswordClient struct {
	secret      string
	resolvedRef string
	created     []onepassword.ItemCreateParams
	updated     []onepassword.Item
	items       map[string]onepassword.Item
}

func newFakeOnePasswordClient() *fakeOnePasswordClient {
	return &fakeOnePasswordClient{
		items: map[string]onepassword.Item{},
	}
}

func (c *fakeOnePasswordClient) Resolve(_ context.Context, secretReference string) (string, error) {
	c.resolvedRef = secretReference
	return c.secret, nil
}

func (c *fakeOnePasswordClient) ListVaults(_ context.Context) ([]onepassword.VaultOverview, error) {
	return []onepassword.VaultOverview{{ID: "vault-1", Title: "Engineering"}}, nil
}

func (c *fakeOnePasswordClient) ListItems(_ context.Context, vaultID string) ([]onepassword.ItemOverview, error) {
	items := make([]onepassword.ItemOverview, 0, len(c.items))
	for _, item := range c.items {
		if item.VaultID == vaultID {
			items = append(items, onepassword.ItemOverview{ID: item.ID, VaultID: item.VaultID, Title: item.Title})
		}
	}
	return items, nil
}

func (c *fakeOnePasswordClient) GetItem(_ context.Context, vaultID, itemID string) (onepassword.Item, error) {
	item, ok := c.items[itemID]
	if !ok || item.VaultID != vaultID {
		return onepassword.Item{}, ErrCredentialNotFound
	}
	return item, nil
}

func (c *fakeOnePasswordClient) CreateItem(_ context.Context, params onepassword.ItemCreateParams) (onepassword.Item, error) {
	c.created = append(c.created, params)
	item := onepassword.Item{
		ID:       "item-1",
		Title:    params.Title,
		Category: params.Category,
		VaultID:  params.VaultID,
		Fields:   params.Fields,
		Sections: params.Sections,
		Tags:     params.Tags,
	}
	c.items[item.ID] = item
	return item, nil
}

func (c *fakeOnePasswordClient) PutItem(_ context.Context, item onepassword.Item) (onepassword.Item, error) {
	c.updated = append(c.updated, item)
	c.items[item.ID] = item
	return item, nil
}
