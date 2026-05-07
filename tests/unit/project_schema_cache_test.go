package unit

import (
	"testing"
	"time"

	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestProjectSchemaCacheTTLProfileSwitchAndManualInvalidation(t *testing.T) {
	cache := jira.NewProjectSchemaCache(time.Minute)
	schema := &jira.ProjectFieldSchema{ProjectKey: "PROJ", IssueType: "Task"}
	cache.Set("default", "PROJ", "Task", schema)
	if got, ok := cache.Get("default", "PROJ", "Task"); !ok || got.ProjectKey != "PROJ" {
		t.Fatalf("cache get = %+v ok=%v", got, ok)
	}
	cache.InvalidateProfile("default")
	if _, ok := cache.Get("default", "PROJ", "Task"); ok {
		t.Fatal("cache still has profile entry after invalidation")
	}

	cache.Set("other", "PROJ", "Task", schema)
	cache.InvalidateAll()
	if _, ok := cache.Get("other", "PROJ", "Task"); ok {
		t.Fatal("cache still has entry after manual invalidate")
	}
}
