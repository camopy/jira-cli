package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	onepassword "github.com/1password/onepassword-sdk-go"
	"github.com/matcra587/jira-cli/internal/version"
)

type OnePasswordStore struct {
	Bin string
	SDK OnePasswordSDKStore
}

func (s OnePasswordStore) Get(ctx context.Context, ref SecretRef) (string, error) {
	if s.useSDK(ref) {
		return s.sdkStore().Get(ctx, ref)
	}
	out, err := s.run(ctx, "item", "get", ref.Item, "--vault", ref.Vault, "--fields", "credential")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s OnePasswordStore) Put(ctx context.Context, ref SecretRef, secret string) error {
	if s.useSDK(ref) {
		return s.sdkStore().Put(ctx, ref, secret)
	}
	payload, err := json.Marshal(map[string]any{
		"title":    ref.Item,
		"category": "password",
		"fields": []map[string]any{{
			"id":      "credential",
			"type":    "CONCEALED",
			"label":   "credential",
			"value":   secret,
			"purpose": "PASSWORD",
		}},
	})
	if err != nil {
		return err
	}
	_, err = s.runWithInput(ctx, string(payload), "item", "create", "--vault", ref.Vault, "--template", "-")
	return err
}

func (s OnePasswordStore) Delete(ctx context.Context, ref SecretRef) error {
	if s.useSDK(ref) {
		return s.sdkStore().Delete(ctx, ref)
	}
	_, err := s.run(ctx, "item", "delete", ref.Item, "--vault", ref.Vault, "--archive")
	return err
}

func (s OnePasswordStore) useSDK(ref SecretRef) bool {
	return s.Bin == "" && (s.SDK.Client != nil || s.SDK.NewClient != nil || ref.Account != "" || os.Getenv(onePasswordServiceAccountTokenEnv) != "")
}

func (s OnePasswordStore) sdkStore() OnePasswordSDKStore {
	if s.SDK.Client != nil || s.SDK.NewClient != nil {
		return s.SDK
	}
	return OnePasswordSDKStore{}
}

const onePasswordServiceAccountTokenEnv = "OP_SERVICE_ACCOUNT_TOKEN"

type onePasswordClient interface {
	Resolve(context.Context, string) (string, error)
	ListVaults(context.Context) ([]onepassword.VaultOverview, error)
	ListItems(context.Context, string) ([]onepassword.ItemOverview, error)
	GetItem(context.Context, string, string) (onepassword.Item, error)
	CreateItem(context.Context, onepassword.ItemCreateParams) (onepassword.Item, error)
	PutItem(context.Context, onepassword.Item) (onepassword.Item, error)
	ArchiveItem(context.Context, string, string) error
}

type onePasswordClientFactory func(context.Context, SecretRef) (onePasswordClient, error)

type OnePasswordSDKStore struct {
	Client    onePasswordClient
	NewClient onePasswordClientFactory
}

func (s OnePasswordSDKStore) Get(ctx context.Context, ref SecretRef) (string, error) {
	client, err := s.client(ctx, ref)
	if err != nil {
		return "", err
	}
	return client.Resolve(ctx, onePasswordCredentialReference(ref))
}

func (s OnePasswordSDKStore) Put(ctx context.Context, ref SecretRef, secret string) error {
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
		Tags:     []string{"jira-cli"},
	})
	return err
}

func (s OnePasswordSDKStore) Delete(ctx context.Context, ref SecretRef) error {
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
	return client.ArchiveItem(ctx, vaultID, item.ID)
}

func (s OnePasswordSDKStore) client(ctx context.Context, ref SecretRef) (onePasswordClient, error) {
	if s.Client != nil {
		return s.Client, nil
	}
	newClient := s.NewClient
	if newClient == nil {
		newClient = newOnePasswordSDKClient
	}
	return newClient(ctx, ref)
}

func newOnePasswordSDKClient(ctx context.Context, ref SecretRef) (onePasswordClient, error) {
	opts := []onepassword.ClientOption{
		onepassword.WithIntegrationInfo("jira-cli", version.Version),
	}
	if token := os.Getenv(onePasswordServiceAccountTokenEnv); token != "" {
		opts = append(opts, onepassword.WithServiceAccountToken(token))
	} else if ref.Account != "" {
		opts = append(opts, onepassword.WithDesktopAppIntegration(ref.Account))
	} else {
		return nil, fmt.Errorf("%w: 1Password SDK requires %s or onepassword_account", ErrCredentialNotFound, onePasswordServiceAccountTokenEnv)
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

func (c onePasswordSDKClient) ArchiveItem(ctx context.Context, vaultID, itemID string) error {
	return c.client.Items().Archive(ctx, vaultID, itemID)
}

func onePasswordCredentialReference(ref SecretRef) string {
	return "op://" + url.PathEscape(ref.Vault) + "/" + url.PathEscape(ref.Item) + "/credential"
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

func onePasswordFindItem(ctx context.Context, client onePasswordClient, vaultID, itemTitle string) (onepassword.Item, bool, error) {
	if itemTitle == "" {
		return onepassword.Item{}, false, fmt.Errorf("1Password item is required: %w", ErrCredentialNotFound)
	}
	items, err := client.ListItems(ctx, vaultID)
	if err != nil {
		return onepassword.Item{}, false, err
	}
	for _, item := range items {
		if item.ID == itemTitle || item.Title == itemTitle {
			full, err := client.GetItem(ctx, vaultID, item.ID)
			if err != nil {
				return onepassword.Item{}, false, err
			}
			return full, true, nil
		}
	}
	return onepassword.Item{}, false, nil
}

func upsertOnePasswordCredentialField(fields []onepassword.ItemField, secret string) []onepassword.ItemField {
	for i := range fields {
		if fields[i].ID == "credential" || strings.EqualFold(fields[i].Title, "credential") {
			fields[i] = onePasswordCredentialField(secret)
			return fields
		}
	}
	return append(fields, onePasswordCredentialField(secret))
}

func onePasswordCredentialField(secret string) onepassword.ItemField {
	return onepassword.ItemField{
		ID:        "credential",
		Title:     "credential",
		FieldType: onepassword.ItemFieldTypeConcealed,
		Value:     secret,
	}
}

func (s OnePasswordStore) run(ctx context.Context, args ...string) (string, error) {
	return s.runWithInput(ctx, "", args...)
}

func (s OnePasswordStore) runWithInput(ctx context.Context, input string, args ...string) (string, error) {
	bin := s.Bin
	if bin == "" {
		bin = "op"
	}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // 1Password command and args are constructed from trusted CLI backend configuration.
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if expiredOnePasswordSession(msg) {
			return "", fmt.Errorf("1Password CLI session expired: run `op signin` and retry: %w", err)
		}
		return "", fmt.Errorf("1Password CLI command failed: %w", err)
	}
	return string(out), nil
}

func expiredOnePasswordSession(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "not signed in") ||
		strings.Contains(msg, "signin") ||
		strings.Contains(msg, "sign in") ||
		strings.Contains(msg, "session expired")
}
