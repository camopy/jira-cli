package cli

import (
	"bytes"
	"reflect"
	"testing"
)

// TestAliasListPlainRendersAliases pins the human rendering of `alias list`:
// one aligned name → expansion line per alias, natural-ordered by name
// (sprint2 before sprint10), never the generic value={...} collapse.
func TestAliasListPlainRendersAliases(t *testing.T) {
	var buf bytes.Buffer
	err := WriteAliasListPlain(&buf, "alias.list", map[string]any{"aliases": map[string]string{
		"sprint10": "search jql 'sprint = 10'",
		"mine":     "issue list --assignee me",
		"sprint2":  "search jql 'sprint = 2'",
	}, "count": 3})
	if err != nil {
		t.Fatalf("WriteAliasListPlain() error = %v", err)
	}
	want := "Aliases\n" +
		"mine      →  issue list --assignee me\n" +
		"sprint2   →  search jql 'sprint = 2'\n" +
		"sprint10  →  search jql 'sprint = 10'\n"
	if got := buf.String(); got != want {
		t.Errorf("alias list plain output = %q, want %q", got, want)
	}
}

// TestAliasListPlainEmpty pins the no-aliases state: it says so explicitly
// instead of printing nothing.
func TestAliasListPlainEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteAliasListPlain(&buf, "alias.list", map[string]any{"aliases": map[string]string{}, "count": 0}); err != nil {
		t.Fatalf("WriteAliasListPlain() error = %v", err)
	}
	want := "Aliases\n  (no aliases configured)\n"
	if got := buf.String(); got != want {
		t.Errorf("empty alias list plain output = %q, want %q", got, want)
	}
}

// TestAliasListPlainDispatch verifies WriteCommandPlain routes alias.list to
// the dedicated renderer rather than the generic fallback.
func TestAliasListPlainDispatch(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCommandPlain(&buf, "alias.list", map[string]any{"aliases": map[string]string{
		"mine": "issue list --assignee me",
	}, "count": 1})
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	want := "Aliases\nmine  →  issue list --assignee me\n"
	if got := buf.String(); got != want {
		t.Errorf("alias.list dispatch output = %q, want %q", got, want)
	}
}

// TestPlainFieldsRendersStringKeyedMaps pins the hardened generic fallback:
// any string-keyed map renders one field per key (lexically ordered, matching
// the map[string]any path), not a single collapsed value={...} field. Nested
// string-keyed maps flatten with dotted keys the same way map[string]any does.
func TestPlainFieldsRendersStringKeyedMaps(t *testing.T) {
	got := plainFields(map[string]string{"beta": "2", "alpha": "1"})
	want := []plainField{
		{key: "alpha", value: "1"},
		{key: "beta", value: "2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("plainFields(map[string]string) = %+v, want %+v", got, want)
	}

	gotInts := plainFields(map[string]int{"count": 3})
	// Non-string scalars render through plainFieldValue's fmt.Sprint tail.
	wantInts := []plainField{{key: "count", value: "3"}}
	if !reflect.DeepEqual(gotInts, wantInts) {
		t.Errorf("plainFields(map[string]int) = %+v, want %+v", gotInts, wantInts)
	}

	gotNested := plainFields(map[string]any{"outer": map[string]string{"inner": "v"}})
	wantNested := []plainField{{key: "outer.inner", value: "v"}}
	if !reflect.DeepEqual(gotNested, wantNested) {
		t.Errorf("plainFields(nested string map) = %+v, want %+v", gotNested, wantNested)
	}
}
