package cache

import (
	"context"
	"encoding/json"
	"testing"

	cachepkg "github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/jira"
)

func TestRefreshResourceValidatesBeforeWritingCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	resource := registry.Resource{
		Name:       "invalid",
		TTLMinutes: 60,
		Fetch: func(context.Context, *jira.Client) (json.RawMessage, error) {
			return json.RawMessage(`{"not":"a list"}`), nil
		},
	}
	registry.Registry = append(registry.Registry, resource)
	t.Cleanup(func() {
		registry.Registry = registry.Registry[:len(registry.Registry)-1]
	})

	_, err := refreshResource(
		context.Background(),
		"review-test",
		nil,
		true,
		resource.Name,
		true,
		false,
		0,
		false,
	)
	if err == nil {
		t.Fatal("refreshResource() error = nil, want invalid fetched data")
	}
	_, present, _, readErr := cachepkg.Read("review-test", resource.Name, 0)
	if readErr != nil {
		t.Fatalf("Read() error = %v", readErr)
	}
	if present {
		t.Fatal("invalid fetched data was persisted")
	}
}
