package adf_test

// ADF validation tests.
//
// These tests exercise ValidateDoc, which must be wired into the pipeline
// stage-2 ADF validation for every mutation path.
//
// Failing conditions covered:
//   - Wrong-shape root (type != "doc", version != 1) → always error
//   - Unknown node in strict mode → error naming the node
//   - Unknown node in best-effort mode → warning, no error (opaque preserve)
//   - Illegal mark on block node in strict mode → error naming path
//   - Illegal mark on block node in best-effort mode → warning, no error
//   - Unknown mark in strict mode → error naming the mark
//   - Unknown mark in best-effort mode → warning, no error, mark preserved
//   - 100-deep valid structure → no panic, no error (depth is legal)

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// --- root shape ---

func TestValidateDoc_WrongType_AlwaysErrors(t *testing.T) {
	doc := adf.Document{Type: "foo", Version: 1}
	_, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("expected error for doc.type != 'doc'")
	}
	if !strings.Contains(err.Error(), "doc") {
		t.Errorf("error should mention 'doc': %v", err)
	}
}

func TestValidateDoc_WrongType_BestEffortAlsoErrors(t *testing.T) {
	// Root shape is always fatal regardless of mode.
	doc := adf.Document{Type: "", Version: 1}
	_, err := adf.ValidateDoc(doc, adfmode.ModeBestEffort)
	if err == nil {
		t.Fatal("expected error for doc.type='' even in best-effort mode")
	}
}

func TestValidateDoc_WrongVersion_AlwaysErrors(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 0}
	_, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("expected error for doc.version != 1")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error should mention 'version': %v", err)
	}
}

// --- unknown node ---

func TestValidateDoc_UnknownNode_StrictMode_Errors(t *testing.T) {
	raw := []byte(`{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"prefix "},
			{"type":"unknown_magic_node","attrs":{"x":1}}
		]}
	]}`)
	doc, _, err := adf.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, valErr := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if valErr == nil {
		t.Fatal("strict mode: expected error for unknown node 'unknown_magic_node'")
	}
	if !strings.Contains(valErr.Error(), "unknown_magic_node") {
		t.Errorf("error should name the unknown node; got: %v", valErr)
	}
}

func TestValidateDoc_UnknownNode_BestEffortMode_WarnNotError(t *testing.T) {
	raw := []byte(`{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"prefix "},
			{"type":"unknown_magic_node","attrs":{"x":1}}
		]}
	]}`)
	doc, _, err := adf.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	warnings, valErr := adf.ValidateDoc(doc, adfmode.ModeBestEffort)
	if valErr != nil {
		t.Fatalf("best-effort: unexpected error for unknown node: %v", valErr)
	}
	if len(warnings) == 0 {
		t.Fatal("best-effort: expected at least one warning for unknown node")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w.Message, "unknown_magic_node") || strings.Contains(w.NodeType, "unknown_magic_node") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warning should name unknown_magic_node; warnings: %+v", warnings)
	}
}

// --- illegal mark on block node ---

func TestValidateDoc_IllegalMarkOnBlock_StrictMode_Errors(t *testing.T) {
	raw := []byte(`{"type":"doc","version":1,"content":[
		{"type":"paragraph","marks":[{"type":"strong"}],"content":[
			{"type":"text","text":"x"}
		]}
	]}`)
	doc, _, err := adf.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, valErr := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if valErr == nil {
		t.Fatal("strict mode: expected error for marks on block node 'paragraph'")
	}
	if !strings.Contains(valErr.Error(), "paragraph") && !strings.Contains(valErr.Error(), "marks") {
		t.Errorf("error should reference 'paragraph' or 'marks'; got: %v", valErr)
	}
}

func TestValidateDoc_IllegalMarkOnBlock_BestEffortMode_WarnNotError(t *testing.T) {
	raw := []byte(`{"type":"doc","version":1,"content":[
		{"type":"paragraph","marks":[{"type":"strong"}],"content":[
			{"type":"text","text":"x"}
		]}
	]}`)
	doc, _, err := adf.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	warnings, valErr := adf.ValidateDoc(doc, adfmode.ModeBestEffort)
	if valErr != nil {
		t.Fatalf("best-effort: unexpected error for marks on block: %v", valErr)
	}
	if len(warnings) == 0 {
		t.Fatal("best-effort: expected at least one warning for illegal marks on block node")
	}
}

// --- unknown mark ---

func TestValidateDoc_UnknownMark_Strict_Rejects(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{{
		Type: "paragraph", Content: []adf.Node{{
			Type: "text", Text: "hi",
			Marks: []adf.Mark{{Type: "unknown_mark"}},
		}},
	}}}
	_, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("expected strict mode to reject unknown mark, got nil")
	}
	if !strings.Contains(err.Error(), "unknown_mark") {
		t.Fatalf("error must name the mark; got %v", err)
	}
}

func TestValidateDoc_UnknownMark_BestEffort_WarnsAndPreserves(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{{
		Type: "paragraph", Content: []adf.Node{{
			Type: "text", Text: "hi",
			Marks: []adf.Mark{{Type: "unknown_mark", Attrs: map[string]any{"x": 1}}},
		}},
	}}}
	warnings, err := adf.ValidateDoc(doc, adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("best-effort: unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("best-effort: expected at least one warning for unknown mark")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w.Message, "unknown_mark") || strings.Contains(w.MarkType, "unknown_mark") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warning must name unknown_mark; warnings: %+v", warnings)
	}
	// Doc structure must be preserved (mark still present on output node).
	if len(doc.Content[0].Content[0].Marks) != 1 {
		t.Error("best-effort: mark must be preserved on the node")
	}
}

// --- valid deep nesting ---

func TestValidateDoc_DeepBlockquote_NoError(t *testing.T) {
	// 100-deep is enough to confirm no stack overflow. The validator is
	// recursive but bounded by encoding/json's depth cap (10000) — verified
	// safe up to 5000-deep input.
	doc := adf.Document{Type: "doc", Version: 1}
	// Build 100-deep blockquote chain.
	leaf := adf.Node{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "deep"}}}
	var buildChain func(depth int) adf.Node
	buildChain = func(depth int) adf.Node {
		if depth == 0 {
			return leaf
		}
		return adf.Node{Type: "blockquote", Content: []adf.Node{buildChain(depth - 1)}}
	}
	doc.Content = []adf.Node{buildChain(100)}

	warnings, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err != nil {
		t.Fatalf("100-deep blockquote: unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("100-deep blockquote: unexpected warnings: %+v", warnings)
	}
}

// --- valid document no-op ---

func TestValidateDoc_ValidDocument_NoErrorNoWarning(t *testing.T) {
	raw := []byte(`{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"hello","marks":[{"type":"strong"}]}
		]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[
				{"type":"paragraph","content":[{"type":"text","text":"item"}]}
			]}
		]}
	]}`)
	doc, _, err := adf.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	warnings, valErr := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if valErr != nil {
		t.Fatalf("valid document: unexpected error: %v", valErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("valid document: unexpected warnings: %+v", warnings)
	}
}
