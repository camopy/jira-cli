package unit

import (
	"os"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/editor"
)

func TestEditorRoundTripTempFileFrontmatterAndResolution(t *testing.T) {
	t.Setenv("EDITOR", "true")
	t.Setenv("VISUAL", "false")
	if got := editor.ResolveEditor(); got != "true" {
		t.Fatalf("ResolveEditor() = %q", got)
	}
	path, err := editor.WriteTemp("PROJ-1", "description", "hello")
	if err != nil {
		t.Fatalf("WriteTemp() error = %v", err)
	}
	defer func() { _ = os.Remove(path) }()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	// Per the CLI contract: temp file YAML frontmatter MUST use
	// issue_key and field_name (not the abbreviated issue/field).
	if !strings.Contains(string(b), "issue_key: PROJ-1") || !strings.Contains(string(b), "field_name: description") {
		t.Fatalf("frontmatter must use issue_key/field_name keys per spec, got:\n%s", b)
	}
}
