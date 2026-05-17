//go:build cgo || windows

package config

import (
	"context"
	"fmt"
	"net/url"
	"os"

	onepassword "github.com/1password/onepassword-sdk-go"
	"github.com/matcra587/jira-cli/internal/version"
)

const onePasswordServiceAccountTokenEnv = "OP_SERVICE_ACCOUNT_TOKEN"

// onePasswordCredentialFieldID is the stable, jira-cli-owned identifier of the
// managed credential field. Ownership of a field is decided by this ID, never
// by display title: a user's own field merely titled "credential" carries a
// different ID and is never read, overwritten, or removed by jira-cli. The ID
// doubles as the field's title so it is also a unique secret-reference path
// segment.
//
//nolint:gosec // G101: a 1Password field identifier, not a credential value.
const onePasswordCredentialFieldID = "jira-cli-credential"

// onePasswordClient is the subset of the 1Password SDK jira-cli depends on.
// It is an interface so tests can substitute an in-memory fake. There is no
// item-delete method: jira-cli never destroys a 1Password item, only ever
// strips its own managed credential field via PutItem.
type onePasswordClient interface {
	Resolve(context.Context, string) (string, error)
	ListVaults(context.Context) ([]onepassword.VaultOverview, error)
	ListItems(context.Context, string) ([]onepassword.ItemOverview, error)
	GetItem(context.Context, string, string) (onepassword.Item, error)
	CreateItem(context.Context, onepassword.ItemCreateParams) (onepassword.Item, error)
	PutItem(context.Context, onepassword.Item) (onepassword.Item, error)
}

type onePasswordClientFactory func(context.Context, SecretRef) (onePasswordClient, error)

// OnePasswordStore is the credential store for the 1password backend. It is
// SDK-only: it talks to 1Password through the official Go SDK, never the `op`
// CLI binary. Authentication comes from a service-account token in the
// environment or a desktop-app account pinned on the profile.
type OnePasswordStore struct {
	// Client, when set, is used directly — tests inject an in-memory fake here.
	Client onePasswordClient
	// NewClient, when set, builds a client per request; it defaults to the
	// real SDK client when neither Client nor NewClient is supplied.
	NewClient onePasswordClientFactory
}

// Get resolves the credential field of the profile's 1Password item. It first
// confirms the item exists structurally — the SDK exposes no typed not-found
// error, so the existence check (the same ListItems path Put and Delete use)
// is the deterministic way to tell a missing item from a real backend failure.
// A missing item or vault normalizes to ErrCredentialNotFound.
func (s OnePasswordStore) Get(ctx context.Context, ref SecretRef) (string, error) {
	client, err := s.client(ctx, ref)
	if err != nil {
		return "", err
	}
	vaultID, err := onePasswordVaultID(ctx, client, ref)
	if err != nil {
		return "", err
	}
	if _, found, findErr := onePasswordFindItem(ctx, client, vaultID, ref.Item); findErr != nil {
		return "", findErr
	} else if !found {
		return "", fmt.Errorf("1Password item %q: %w", ref.Item, ErrCredentialNotFound)
	}
	return client.Resolve(ctx, onePasswordCredentialReference(ref))
}

// Put writes the credential to 1Password as a true upsert: an existing item is
// edited in place, otherwise the item is created. The in-place edit means a
// staged Put genuinely overwrites a prior value, so the migration rollback
// contract — "restore the prior value" — holds.
//
// The credential always goes into the field with the stable jira-cli-owned ID;
// a user's own field merely titled "credential" with a different ID is left
// untouched. Editing a pre-existing item only writes that one field, so Put
// never disturbs anything else on an item jira-cli did not create.
func (s OnePasswordStore) Put(ctx context.Context, ref SecretRef, secret string) error {
	client, err := s.client(ctx, ref)
	if err != nil {
		return err
	}
	vaultID, err := onePasswordVaultID(ctx, client, ref)
	if err != nil {
		return err
	}
	item, found, err := onePasswordFindItem(ctx, client, vaultID, ref.Item)
	if err != nil {
		return err
	}
	if found {
		item.Fields = upsertOnePasswordCredentialField(item.Fields, secret)
		if item.Category == "" {
			item.Category = onepassword.ItemCategoryPassword
		}
		_, err = client.PutItem(ctx, item)
		return err
	}
	_, err = client.CreateItem(ctx, onepassword.ItemCreateParams{
		Category: onepassword.ItemCategoryPassword,
		VaultID:  vaultID,
		Title:    ref.Item,
		Fields:   []onepassword.ItemField{onePasswordCredentialField(secret)},
	})
	return err
}

// Delete revokes the credential from 1Password. It NEVER destroys a 1Password
// item — not even one jira-cli itself created. A 1Password item is a
// user-owned object, and an item's tags are user-editable metadata that can
// never prove who created it; so the only safe revocation is to remove
// jira-cli's own managed credential field — identified by its stable
// jira-cli-owned ID — and leave the item, and every other field and all its
// content, in place. An item jira-cli created solely to hold a credential
// simply ends up without that field; that is fine and expected.
//
// A missing item normalizes to ErrCredentialNotFound so revocation is
// idempotent.
func (s OnePasswordStore) Delete(ctx context.Context, ref SecretRef) error {
	client, err := s.client(ctx, ref)
	if err != nil {
		return err
	}
	vaultID, err := onePasswordVaultID(ctx, client, ref)
	if err != nil {
		return err
	}
	item, found, err := onePasswordFindItem(ctx, client, vaultID, ref.Item)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("1Password item %q: %w", ref.Item, ErrCredentialNotFound)
	}
	stripped, hadCredential := removeOnePasswordCredentialField(item.Fields)
	if !hadCredential {
		// Nothing of jira-cli's to remove; leave the item untouched.
		return nil
	}
	item.Fields = stripped
	_, err = client.PutItem(ctx, item)
	return err
}

func (s OnePasswordStore) client(ctx context.Context, ref SecretRef) (onePasswordClient, error) {
	if s.Client != nil {
		return s.Client, nil
	}
	newClient := s.NewClient
	if newClient == nil {
		newClient = newOnePasswordSDKClient
	}
	return newClient(ctx, ref)
}

// newOnePasswordSDKClient builds the real 1Password SDK client. It requires an
// auth source: a service-account token in the environment, or a desktop-app
// account name pinned on the profile. With neither, it returns a typed
// credential error rather than a generic SDK failure.
func newOnePasswordSDKClient(ctx context.Context, ref SecretRef) (onePasswordClient, error) {
	opts := []onepassword.ClientOption{
		onepassword.WithIntegrationInfo("jira-cli", version.Version),
	}
	switch {
	case os.Getenv(onePasswordServiceAccountTokenEnv) != "":
		opts = append(opts, onepassword.WithServiceAccountToken(os.Getenv(onePasswordServiceAccountTokenEnv)))
	case ref.Account != "":
		opts = append(opts, onepassword.WithDesktopAppIntegration(ref.Account))
	default:
		return nil, fmt.Errorf("%w: 1Password requires %s or onepassword_account", ErrCredentialNotFound, onePasswordServiceAccountTokenEnv)
	}
	client, err := onepassword.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return onePasswordSDKClient{client: client}, nil
}

type onePasswordSDKClient struct {
	client *onepassword.Client
}

func (c onePasswordSDKClient) Resolve(ctx context.Context, secretReference string) (string, error) {
	return c.client.Secrets().Resolve(ctx, secretReference)
}

func (c onePasswordSDKClient) ListVaults(ctx context.Context) ([]onepassword.VaultOverview, error) {
	return c.client.Vaults().List(ctx)
}

func (c onePasswordSDKClient) ListItems(ctx context.Context, vaultID string) ([]onepassword.ItemOverview, error) {
	return c.client.Items().List(ctx, vaultID)
}

func (c onePasswordSDKClient) GetItem(ctx context.Context, vaultID, itemID string) (onepassword.Item, error) {
	return c.client.Items().Get(ctx, vaultID, itemID)
}

func (c onePasswordSDKClient) CreateItem(ctx context.Context, params onepassword.ItemCreateParams) (onepassword.Item, error) {
	return c.client.Items().Create(ctx, params)
}

func (c onePasswordSDKClient) PutItem(ctx context.Context, item onepassword.Item) (onepassword.Item, error) {
	return c.client.Items().Put(ctx, item)
}

func onePasswordCredentialReference(ref SecretRef) string {
	return "op://" + url.PathEscape(ref.Vault) + "/" + url.PathEscape(ref.Item) + "/" + onePasswordCredentialFieldID
}

func onePasswordVaultID(ctx context.Context, client onePasswordClient, ref SecretRef) (string, error) {
	if ref.Vault == "" {
		return "", fmt.Errorf("1Password vault is required: %w", ErrCredentialNotFound)
	}
	vaults, err := client.ListVaults(ctx)
	if err != nil {
		return "", err
	}
	for _, vault := range vaults {
		if vault.ID == ref.Vault || vault.Title == ref.Vault {
			return vault.ID, nil
		}
	}
	return "", fmt.Errorf("1Password vault %q: %w", ref.Vault, ErrCredentialNotFound)
}

// onePasswordFindItem locates the profile's item in the vault. An ID match is
// unambiguous and wins outright; title matches are collected so duplicate
// titles are rejected with a typed error rather than silently resolving to
// whichever item happened to be listed first.
func onePasswordFindItem(ctx context.Context, client onePasswordClient, vaultID, itemTitle string) (onepassword.Item, bool, error) {
	if itemTitle == "" {
		return onepassword.Item{}, false, fmt.Errorf("1Password item is required: %w", ErrCredentialNotFound)
	}
	items, err := client.ListItems(ctx, vaultID)
	if err != nil {
		return onepassword.Item{}, false, err
	}
	var titleMatches []onepassword.ItemOverview
	for _, item := range items {
		if item.ID == itemTitle {
			full, getErr := client.GetItem(ctx, vaultID, item.ID)
			if getErr != nil {
				return onepassword.Item{}, false, getErr
			}
			return full, true, nil
		}
		if item.Title == itemTitle {
			titleMatches = append(titleMatches, item)
		}
	}
	if len(titleMatches) > 1 {
		return onepassword.Item{}, false, &CredentialError{
			Type:        ErrorTypeValidation,
			ErrCode:     ErrorCodeOnePasswordItemAmbiguous,
			Message:     fmt.Sprintf("the 1Password vault has %d items titled %q", len(titleMatches), itemTitle),
			HintMsg:     "give the profile a unique 1Password item title or set the item to a specific item ID",
			IsRetryable: false,
			Context:     ErrorContext{Backend: string(SecretBackendOnePassword), ConfigKey: "profile.item"},
		}
	}
	if len(titleMatches) == 1 {
		full, getErr := client.GetItem(ctx, vaultID, titleMatches[0].ID)
		if getErr != nil {
			return onepassword.Item{}, false, getErr
		}
		return full, true, nil
	}
	return onepassword.Item{}, false, nil
}

// upsertOnePasswordCredentialField writes the credential into the field with
// the stable jira-cli-owned ID, replacing it if present and appending it
// otherwise. Matching is by ID only: a user's own field merely titled
// "credential" carries a different ID and is left untouched.
func upsertOnePasswordCredentialField(fields []onepassword.ItemField, secret string) []onepassword.ItemField {
	for i := range fields {
		if isOnePasswordCredentialField(fields[i]) {
			fields[i] = onePasswordCredentialField(secret)
			return fields
		}
	}
	return append(fields, onePasswordCredentialField(secret))
}

// removeOnePasswordCredentialField returns the item's fields with the managed
// jira-cli credential field dropped, and reports whether one was present.
// Used to revoke a credential from an item jira-cli did not create without
// deleting the item or disturbing any other field on it. Matching is by the
// jira-cli-owned field ID, so a user field titled "credential" is preserved.
func removeOnePasswordCredentialField(fields []onepassword.ItemField) ([]onepassword.ItemField, bool) {
	kept := make([]onepassword.ItemField, 0, len(fields))
	removed := false
	for _, f := range fields {
		if isOnePasswordCredentialField(f) {
			removed = true
			continue
		}
		kept = append(kept, f)
	}
	return kept, removed
}

// isOnePasswordCredentialField reports whether a field is jira-cli's managed
// credential field. Identity is the stable jira-cli-owned field ID — never
// the display title, which a user could coincidentally also use.
func isOnePasswordCredentialField(f onepassword.ItemField) bool {
	return f.ID == onePasswordCredentialFieldID
}

func onePasswordCredentialField(secret string) onepassword.ItemField {
	return onepassword.ItemField{
		ID:        onePasswordCredentialFieldID,
		Title:     onePasswordCredentialFieldID,
		FieldType: onepassword.ItemFieldTypeConcealed,
		Value:     secret,
	}
}
