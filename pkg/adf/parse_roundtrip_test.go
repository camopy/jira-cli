package adf_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/matcra587/jira-cli/pkg/adf"
)

// Parse → Marshal MUST preserve a known-node document byte-
// equivalently when re-emitted. Equivalence is asserted on the parsed
// JSON shape, not on byte-for-byte ordering, because map ordering is
// undefined.
func TestParseMarshalRoundTripPreservesKnownNodes(t *testing.T) {
	original := []byte(`{
		"type": "doc",
		"version": 1,
		"content": [
			{"type": "heading", "attrs": {"level": 1}, "content": [{"type": "text", "text": "Title"}]},
			{"type": "paragraph", "content": [
				{"type": "text", "text": "hello "},
				{"type": "text", "text": "world", "marks": [{"type": "strong"}]}
			]}
		]
	}`)

	doc, _, err := adf.Parse(original)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got, err := adf.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	requireJSONEqual(t, original, got)
}

// Opaque-passthrough: unknown node types MUST round-trip byte-
// equivalently — including their attrs, children, and marks. No silent
// loss of rich-text semantics.
func TestParseMarshalPreservesUnknownNodeType(t *testing.T) {
	original := []byte(`{
		"type": "doc",
		"version": 1,
		"content": [
			{"type": "futureBlock", "attrs": {"version": 99, "flag": true},
			 "content": [{"type": "text", "text": "inside opaque"}]}
		]
	}`)

	doc, warnings, err := adf.Parse(original)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("Parse on opaque should not warn in best-effort default; got %d warnings", len(warnings))
	}

	got, err := adf.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	requireJSONEqual(t, original, got)
}

// Opaque marks: unknown mark types on known text nodes MUST round-trip
// without loss of the mark or its attrs.
func TestParseMarshalPreservesUnknownMark(t *testing.T) {
	original := []byte(`{
		"type": "doc",
		"version": 1,
		"content": [
			{"type": "paragraph", "content": [
				{"type": "text", "text": "decorated",
				 "marks": [
					{"type": "futureMark", "attrs": {"shade": "neon"}}
				 ]}
			]}
		]
	}`)

	doc, warnings, err := adf.Parse(original)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("Parse on opaque mark should not warn in best-effort default; got %d warnings", len(warnings))
	}

	got, err := adf.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	requireJSONEqual(t, original, got)
}

// Nested opaques: an unknown node nested inside a known block MUST
// survive the round-trip with its attrs and children intact.
func TestParseMarshalPreservesNestedOpaque(t *testing.T) {
	original := []byte(`{
		"type": "doc",
		"version": 1,
		"content": [
			{"type": "panel", "attrs": {"panelType": "info"}, "content": [
				{"type": "paragraph", "content": [{"type": "text", "text": "before"}]},
				{"type": "futureWidget", "attrs": {"id": "w-1"}, "content": [
					{"type": "text", "text": "embedded opaque"}
				]},
				{"type": "paragraph", "content": [{"type": "text", "text": "after"}]}
			]}
		]
	}`)

	doc, _, err := adf.Parse(original)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got, err := adf.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	requireJSONEqual(t, original, got)
}

func requireJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var w, g any
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("unmarshal got: %v\ngot bytes: %s", err, got)
	}
	if !reflect.DeepEqual(w, g) {
		wb, _ := json.MarshalIndent(w, "", "  ")
		gb, _ := json.MarshalIndent(g, "", "  ")
		t.Fatalf("JSON not equivalent\n--- want ---\n%s\n--- got ---\n%s", wb, gb)
	}
}
