package adf_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// The pinned @atlaskit/adf-schema artifact (testdata/adf-schema-v52.11.3.json)
// is the ground truth the validation rules were derived from. This test
// walks every node and mark definition in it and demands each one is (a) a
// type the validator recognizes and (b) a row in the agent support matrix.
// It exists so the schema, the validator, and the matrix can never drift
// apart silently again: bumping the schema artifact fails this test with a
// named list of everything the new version added.

// schemaDefSkips are schema definitions that are not node or mark types:
// content-group unions referenced by other definitions.
var schemaDefSkips = map[string]bool{
	"block_content":              true,
	"inline_node":                true,
	"nestedExpand_content":       true,
	"non_nestable_block_content": true,
	"table_cell_content":         true,
}

// variantSuffixes are the schema's per-context node variants; each maps to
// the same base node type.
var variantSuffixes = []string{
	"_with_marks",
	"_with_alignment",
	"_with_indentation",
	"_with_no_marks",
	"_with_font_size",
	"_full",
	"_root_only",
	"_caption",
}

// snakeNames maps the schema's snake_case table definitions to their
// camelCase node types.
var snakeNames = map[string]string{
	"table_cell":   "tableCell",
	"table_row":    "tableRow",
	"table_header": "tableHeader",
}

// textVariants are text-node shapes with dedicated definitions.
var textVariants = map[string]bool{
	"code_inline":           true,
	"formatted_text_inline": true,
}

// normalizeSchemaDef resolves a schema definition name to its node or mark
// type. ok=false means the definition is not a node/mark type.
func normalizeSchemaDef(def string) (name string, kind adf.Kind, ok bool) {
	if schemaDefSkips[def] {
		return "", 0, false
	}
	switch {
	case strings.HasSuffix(def, "_node"):
		kind = adf.KindNode
		name = strings.TrimSuffix(def, "_node")
	case strings.HasSuffix(def, "_mark"):
		kind = adf.KindMark
		name = strings.TrimSuffix(def, "_mark")
	default:
		return "", 0, false
	}
	for _, suffix := range variantSuffixes {
		name = strings.TrimSuffix(name, suffix)
	}
	if textVariants[name] {
		name = "text"
	}
	if camel, isSnake := snakeNames[name]; isSnake {
		name = camel
	}
	return name, kind, true
}

func schemaUniverse(t *testing.T) map[adf.Kind]map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("testdata/adf-schema-v52.11.3.json")
	if err != nil {
		t.Fatalf("read pinned schema artifact: %v", err)
	}
	var schema struct {
		Definitions map[string]json.RawMessage `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("parse pinned schema artifact: %v", err)
	}
	if len(schema.Definitions) == 0 {
		t.Fatal("pinned schema artifact has no definitions — wrong file?")
	}
	universe := map[adf.Kind]map[string]bool{
		adf.KindNode: {},
		adf.KindMark: {},
	}
	for def := range schema.Definitions {
		if name, kind, ok := normalizeSchemaDef(def); ok {
			universe[kind][name] = true
		}
	}
	return universe
}

// TestEverySchemaTypeIsKnownAndInMatrix is the drift gate: every node and
// mark in the pinned ADF schema must have a matrix row (which also implies
// the validator recognizes it — the matrix synthesizes rows from the
// validation rules).
func TestEverySchemaTypeIsKnownAndInMatrix(t *testing.T) {
	reg := adf.Registry()
	universe := schemaUniverse(t)
	if len(universe[adf.KindNode]) < 30 || len(universe[adf.KindMark]) < 10 {
		t.Fatalf("schema universe implausibly small (%d nodes, %d marks) — normalization broke",
			len(universe[adf.KindNode]), len(universe[adf.KindMark]))
	}
	for kind, names := range universe {
		for name := range names {
			if _, ok := reg.Lookup(kind, name); !ok {
				t.Errorf("schema %s %q has no support-matrix row: add a validation rule "+
					"(schema_rules.go) so Registry() synthesizes one, or a curated row (registry.go)",
					kind, name)
			}
		}
	}
}

// TestMatrixClaimsNothingOutsideTheSchema is the reverse gate: a matrix
// row naming a type absent from the pinned schema is a typo or a stale
// leftover from a schema bump.
func TestMatrixClaimsNothingOutsideTheSchema(t *testing.T) {
	universe := schemaUniverse(t)
	// doc has no definition of its own — it is the schema's root object.
	universe[adf.KindNode]["doc"] = true
	for _, entry := range adf.Registry().All() {
		if !universe[entry.Kind][entry.Name] {
			t.Errorf("matrix row %s %q does not exist in the pinned ADF schema", entry.Kind, entry.Name)
		}
	}
}
