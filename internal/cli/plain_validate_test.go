package cli

import (
	"bytes"
	"strings"
	"testing"

	termansi "github.com/gechr/x/ansi"
)

func validateData() map[string]any {
	return map[string]any{
		"queries": []map[string]any{
			{"query": "project = ENG", "valid": true},
			{"query": "bad =", "valid": false, "errors": []string{"Error in the JQL Query: expecting a value"}},
			{"query": "project = ENG ORDER BY x", "valid": true, "warnings": []string{"The value 'x' does not exist for the field 'order by'"}},
		},
	}
}

// `jql validate` prints one line per query: OK / OK (warnings) / INVALID, with
// the parse messages appended and the query text for legibility.
func TestValidatePlainPrintsPerQueryStatus(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "jql.validate", validateData(), WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteCommandPlain error = %v", err)
	}
	got := termansi.Strip(buf.String())
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d:\n%q", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "OK  ") || !strings.Contains(lines[0], "project = ENG") {
		t.Fatalf("line 0 should be OK with the query, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "INVALID") || !strings.Contains(lines[1], "expecting a value") {
		t.Fatalf("line 1 should be INVALID with the error, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "warnings") || !strings.Contains(lines[2], "does not exist") {
		t.Fatalf("line 2 should surface the warning, got %q", lines[2])
	}
}
