package config

import (
	"context"
	"errors"
	"testing"

	onepassword "github.com/1password/onepassword-sdk-go"
)

func TestOnePasswordSDKStoreResolvesCredentialReference(t *testing.T) {
	client := newFakeOnePasswordClient()
	client.secret = "sdk-secret"
	store := OnePasswordSDKStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira work"}

	got, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "sdk-secret" {
		t.Fatalf("Get() = %q", got)
	}
	if client.resolvedRef != "op://Engineering/jira%20work/credential" {
		t.Fatalf("resolved reference = %q", client.resolvedRef)
	}
}

func TestOnePasswordSDKStoreUpsertsCredentialItem(t *testing.T) {
	client := newFakeOnePasswordClient()
	store := OnePasswordSDKStore{Client: client}
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
	if len(created.Fields) != 1 || created.Fields[0].ID != "credential" || created.Fields[0].FieldType != onepassword.ItemFieldTypeConcealed || created.Fields[0].Value != "sdk-secret" {
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

func TestOnePasswordSDKStoreArchivesCredentialItem(t *testing.T) {
	client := newFakeOnePasswordClient()
	store := OnePasswordSDKStore{Client: client}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Account: "Team", Vault: "Engineering", Item: "jira-cli-work"}

	if err := store.Put(context.Background(), ref, "sdk-secret"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if client.archivedVault != "vault-1" || client.archivedItem != "item-1" {
		t.Fatalf("archived vault/item = %q/%q", client.archivedVault, client.archivedItem)
	}
}

func TestOnePasswordSDKStoreRequiresSDKAuthSource(t *testing.T) {
	store := OnePasswordSDKStore{}
	ref := SecretRef{Profile: "work", Backend: SecretBackendOnePassword, Vault: "Engineering", Item: "jira-cli-work"}

	err := store.Put(context.Background(), ref, "sdk-secret")
	if err == nil {
		t.Fatal("Put() error = nil")
	}
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Put() error = %v, want ErrCredentialNotFound", err)
	}
}

type fakeOnePasswordClient struct {
	secret        string
	resolvedRef   string
	created       []onepassword.ItemCreateParams
	updated       []onepassword.Item
	items         map[string]onepassword.Item
	archivedVault string
	archivedItem  string
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
	}
	c.items[item.ID] = item
	return item, nil
}

func (c *fakeOnePasswordClient) PutItem(_ context.Context, item onepassword.Item) (onepassword.Item, error) {
	c.updated = append(c.updated, item)
	c.items[item.ID] = item
	return item, nil
}

func (c *fakeOnePasswordClient) ArchiveItem(_ context.Context, vaultID, itemID string) error {
	c.archivedVault = vaultID
	c.archivedItem = itemID
	return nil
}
