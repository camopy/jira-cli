package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

func TestNamedQueryLoaderFrontmatterAndRawJQL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mine.jql"), []byte("---\nname: Mine\nproject: PROJ\n---\nassignee = currentUser()"), 0o600); err != nil {
		t.Fatal(err)
	}
	queries, err := config.LoadQueries(dir)
	if err != nil {
		t.Fatalf("LoadQueries() error = %v", err)
	}
	if queries["mine"].Name != "Mine" || queries["mine"].JQL != "assignee = currentUser()" {
		t.Fatalf("query = %+v", queries["mine"])
	}
}
