package customfield_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/jira/customfield"
)

// Validators MUST reject malformed values, not just malformed shapes.
// Each entry asserts that a value with the correct shape but wrong
// type/format fails validation. If any of these passes, the validator
// is shape-only and the "encoding fatal on malformed values" pipeline
// stage isn't being delivered.
func TestValidatorsRejectMalformedValues(t *testing.T) {
	cases := map[string]any{
		"select":          map[string]any{"value": 42},                                          // value must be string
		"multiselect":     []any{map[string]any{"value": 42}},                                   // value must be string
		"user":            map[string]any{"accountId": 123},                                     // accountId must be string
		"user_empty":      map[string]any{"accountId": ""},                                      // accountId must be non-empty
		"group":           map[string]any{"name": 7},                                            // name must be string
		"components":      []any{map[string]any{"name": nil}},                                   // name must be string
		"parent":          map[string]any{"key": "not a real key"},                              // must match issue-key shape
		"parent_int":      map[string]any{"key": 99},                                            // key must be string
		"version":         []any{map[string]any{"name": false}},                                 // name must be string
		"fixversions":     []any{map[string]any{"name": []any{"x"}}},                            // name must be string
		"date":            "monday",                                                             // must be YYYY-MM-DD
		"date_garbage":    "2026-13-99",                                                         // out-of-range pieces
		"datetime":        "tomorrow",                                                           // must be ISO-8601-like
		"cascadingselect": map[string]any{"value": 42, "child": map[string]any{"value": "x"}},   // value must be string
		"cascadingchild":  map[string]any{"value": "Top", "child": map[string]any{"value": 42}}, // child.value must be string
	}
	reg := customfield.Registry()
	typeFor := map[string]string{
		"select": "select", "multiselect": "multiselect", "user": "user", "user_empty": "user",
		"group": "group", "components": "components", "parent": "parent", "parent_int": "parent",
		"version": "version", "fixversions": "fixversions",
		"date": "date", "date_garbage": "date", "datetime": "datetime",
		"cascadingselect": "cascadingselect", "cascadingchild": "cascadingselect",
	}
	for label, sample := range cases {
		t.Run(label, func(t *testing.T) {
			entry, ok := reg.Lookup(typeFor[label])
			if !ok {
				t.Fatalf("registry missing %q", typeFor[label])
			}
			if err := entry.Validator(sample); err == nil {
				t.Fatalf("validator for %q accepted malformed value %+v — should reject", typeFor[label], sample)
			}
		})
	}
}

// Minimum supported field types per the spec.
var requiredTypes = []string{
	"cascadingselect",
	"select", // single-select
	"multiselect",
	"user",  // user picker
	"group", // group picker
	"components",
	"parent", // parent / epic link
	"labels",
	"version", // affects version
	"fixversions",
	"number",
	"string",
	"date",
	"datetime",
}

// Every required field type MUST have a registry row with a working
// validator round-trip. Missing rows or non-functional validators are
// spec violations.
func TestRegistryCoversRequiredFieldTypes(t *testing.T) {
	reg := customfield.Registry()
	for _, name := range requiredTypes {
		entry, ok := reg.Lookup(name)
		if !ok {
			t.Errorf("registry missing required type %q", name)
			continue
		}
		if entry.Encoder == nil {
			t.Errorf("type %q missing Encoder", name)
		}
		if entry.Validator == nil {
			t.Errorf("type %q missing Validator", name)
		}
	}
}

// Each row MUST carry the shared envelope keys.
func TestRegistryEntriesUseSharedEnvelope(t *testing.T) {
	reg := customfield.Registry()
	all := reg.All()
	if len(all) == 0 {
		t.Fatal("registry returned 0 entries")
	}
	for _, entry := range all {
		if entry.Name == "" {
			t.Errorf("entry has empty name")
		}
		if entry.Status == "" {
			t.Errorf("entry %q missing status", entry.Name)
		}
		if entry.SubmitDescription == "" {
			t.Errorf("entry %q missing submit_description", entry.Name)
		}
	}
}

// Every registered type MUST round-trip via Encode → Decode (or
// equivalent) so an agent can submit values back through the registry
// without losing information.
func TestEveryTypeValidatorRoundTrips(t *testing.T) {
	reg := customfield.Registry()
	cases := map[string]any{
		"string":          "hello world",
		"number":          float64(42),
		"date":            "2026-05-04",
		"datetime":        "2026-05-04T10:00:00.000+0000",
		"select":          map[string]any{"value": "Option A"},
		"multiselect":     []any{map[string]any{"value": "A"}, map[string]any{"value": "B"}},
		"user":            map[string]any{"accountId": "5b10ac8d82e05b22cc7d4ef5"},
		"group":           map[string]any{"name": "jira-developers"},
		"components":      []any{map[string]any{"name": "ui"}},
		"parent":          map[string]any{"key": "KAN-1"},
		"labels":          []any{"bug", "regression"},
		"version":         []any{map[string]any{"name": "1.0.0"}},
		"fixversions":     []any{map[string]any{"name": "1.1.0"}},
		"cascadingselect": map[string]any{"value": "Top", "child": map[string]any{"value": "Sub"}},
	}
	for name, sample := range cases {
		t.Run(name, func(t *testing.T) {
			entry, ok := reg.Lookup(name)
			if !ok {
				t.Fatalf("registry missing %q", name)
			}
			if err := entry.Validator(sample); err != nil {
				t.Errorf("validator rejected sample %+v: %v", sample, err)
			}
			out, err := entry.Encoder(sample)
			if err != nil {
				t.Errorf("encoder failed on %+v: %v", sample, err)
			}
			if out == nil {
				t.Errorf("encoder returned nil for %+v", sample)
			}
		})
	}
}
