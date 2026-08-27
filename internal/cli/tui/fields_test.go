package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

func TestResolveCustomFields(t *testing.T) {
	metadata := []fieldMetadata{
		{ID: "customfield_10010", Name: "Story Points"},
		{ID: "customfield_10020", Name: "Customer"},
		{ID: "customfield_10030", Name: "Customer"},
		{ID: "summary", Name: "Summary"},
	}
	entries := []config.TUICustomField{
		{Field: "story points", Column: true, Label: "Points"},
		{Field: "customfield_10020", Column: true},
		{Field: "customfield_99999"},
		{Field: "customfield_1"},
		{Field: "CUSTOMFIELD_10010"},
		{Field: "Customer"},
		{Field: "Summary"},
		{Field: "customfield_10030", Column: true, Label: "Customer"},
	}

	got, err := resolveCustomFields(entries, metadata)
	want := []core.CustomField{
		{ID: "customfield_10010", Name: "Story Points", Label: "Points", Column: true},
		{ID: "customfield_10020", Name: "Customer", Column: true},
		{ID: "customfield_99999", Name: "customfield_99999"},
		{ID: "customfield_10030", Name: "Customer", Label: "Customer", Column: false},
	}
	if len(got) != len(want) {
		t.Fatalf("resolved fields = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("resolved field %d = %#v, want %#v", i, got[i], want[i])
		}
	}
	wantErrors := map[string]string{
		"short id":        `custom field "customfield_1" was not found`,
		"noncanonical id": `custom field "CUSTOMFIELD_10010" was not found`,
		"ambiguous name":  `custom field name "Customer" is ambiguous: customfield_10020, customfield_10030`,
		"system field":    `custom field "Summary" was not found`,
		"duplicate label": `column label "Customer" is already used`,
	}
	for name, wantErr := range wantErrors {
		t.Run(name, func(t *testing.T) {
			if err == nil || !strings.Contains(err.Error(), wantErr) {
				t.Errorf("resolve error = %v, want it to contain %q", err, wantErr)
			}
		})
	}
}

func TestLoadCustomFieldsResolvesNamesFromCacheWithoutAClient(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	key := "tui-fields-test"
	data := json.RawMessage(`[{"id":"customfield_10010","name":"Story Points"}]`)
	if _, err := cache.Write(key, "fields", data); err != nil {
		t.Fatalf("cache.Write() error = %v", err)
	}

	got, err := loadCustomFields(context.Background(), []config.TUICustomField{{Field: "Story Points"}}, key, nil)
	if err != nil {
		t.Fatalf("loadCustomFields() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "customfield_10010" {
		t.Fatalf("loadCustomFields() = %#v", got)
	}
}

func TestResolveCustomFieldsReportsBlankSelector(t *testing.T) {
	got, err := resolveCustomFields([]config.TUICustomField{{Field: "  "}}, nil)
	if len(got) != 0 || err == nil || !strings.Contains(err.Error(), "field is required") {
		t.Fatalf("resolve blank = %#v, %v", got, err)
	}
}
