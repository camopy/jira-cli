package cli

import (
	"bytes"
	"strings"
	"testing"

	termansi "github.com/gechr/x/ansi"
)

// `jql reference` human output is one "value — displayName" line per field, so
// the queryable field set (custom fields included) is legible and greppable.
func TestReferencePlainListsFields(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"fields": []map[string]any{
			{"value": "summary", "display_name": "Summary"},
			{"value": "cf[10010]", "display_name": "Story Points", "custom_field_id": "cf[10010]"},
		},
		"functions":      []map[string]any{{"value": "currentUser()", "display_name": "currentUser()"}},
		"reserved_words": []string{"and", "or"},
	}
	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "jql.reference", data, WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteCommandPlain error = %v", err)
	}
	got := termansi.Strip(buf.String())
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 field lines, got %d:\n%q", len(lines), got)
	}
	if !strings.Contains(lines[0], "summary") || !strings.Contains(lines[0], "Summary") {
		t.Fatalf("line 0 should pair value and displayName, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "Story Points") {
		t.Fatalf("line 1 should show the custom field's display name, got %q", lines[1])
	}
}
