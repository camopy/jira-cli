package adf_test

import (
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// inlineCard on a target field MUST be evaluated against the
// project/field schema. Unknown compatibility = unsupported.
//
// Best-effort: degrade to text node carrying a link mark wrapping the same
// URL. URL preservation is mandatory.
//
// Strict: client-side abort with a typed error. No degradation, no submit.
func TestApplyCompatibility_BestEffortDegradesUnsupportedInlineCard(t *testing.T) {
	doc, _, err := adf.Parse([]byte(`{
		"type": "doc", "version": 1, "content": [
			{"type": "paragraph", "content": [
				{"type": "inlineCard", "attrs": {"url": "https://example.com/issue/KAN-1"}}
			]}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// description on PROJ-X has no inlineCard support per the schema.
	schema := adf.FieldCompatibility{Field: "description", InlineCardSupported: false}

	out, warnings, err := adf.ApplyCompatibility(doc, schema, adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("ApplyCompatibility (best-effort): %v", err)
	}

	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d", len(warnings))
	}
	w := warnings[0]
	if w.Type != "adf_compatibility" {
		t.Fatalf("warning.type = %q, want adf_compatibility", w.Type)
	}
	if w.NodeType != "inlineCard" {
		t.Fatalf("warning.node_type = %q, want inlineCard", w.NodeType)
	}
	if w.Field != "description" {
		t.Fatalf("warning.field = %q, want description", w.Field)
	}
	if !w.Lossy {
		t.Fatalf("warning.lossy = false, want true")
	}
	if w.Path == "" {
		t.Fatalf("warning.path must point to the degraded node")
	}

	// The inlineCard MUST be replaced by a text node carrying a link mark
	// wrapping the same URL. URL never lost.
	para := out.Content[0]
	if len(para.Content) != 1 {
		t.Fatalf("paragraph should have 1 child after degrade, got %d", len(para.Content))
	}
	child := para.Content[0]
	if child.Type != "text" {
		t.Fatalf("degraded child type = %q, want text", child.Type)
	}
	if child.Text == "" {
		t.Fatalf("degraded child must carry the URL as text")
	}
	if len(child.Marks) != 1 || child.Marks[0].Type != "link" {
		t.Fatalf("degraded child must have a link mark; got marks=%v", child.Marks)
	}
	href, _ := child.Marks[0].Attrs["href"].(string)
	if href != "https://example.com/issue/KAN-1" {
		t.Fatalf("link mark href = %q, want the original inlineCard URL", href)
	}
}

// Strict path.
func TestApplyCompatibility_StrictRefusesUnsupportedInlineCard(t *testing.T) {
	doc, _, err := adf.Parse([]byte(`{
		"type": "doc", "version": 1, "content": [
			{"type": "paragraph", "content": [
				{"type": "inlineCard", "attrs": {"url": "https://example.com/x"}}
			]}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	schema := adf.FieldCompatibility{Field: "description", InlineCardSupported: false}

	_, _, err = adf.ApplyCompatibility(doc, schema, adfmode.ModeStrict)
	if err == nil {
		t.Fatalf("strict mode must abort, got nil error")
	}
	var compatErr *adf.CompatibilityError
	if !errors.As(err, &compatErr) {
		t.Fatalf("expected *adf.CompatibilityError, got %T: %v", err, err)
	}
	if compatErr.NodeType != "inlineCard" || compatErr.Field != "description" {
		t.Fatalf("CompatibilityError fields wrong: %+v", compatErr)
	}
}

// When the target field SUPPORTS inlineCard, no degradation and no
// warning. The doc MUST pass through unchanged.
func TestApplyCompatibility_SupportedFieldKeepsInlineCard(t *testing.T) {
	doc, _, err := adf.Parse([]byte(`{
		"type": "doc", "version": 1, "content": [
			{"type": "paragraph", "content": [
				{"type": "inlineCard", "attrs": {"url": "https://example.com/x"}}
			]}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	schema := adf.FieldCompatibility{Field: "description", InlineCardSupported: true}

	out, warnings, err := adf.ApplyCompatibility(doc, schema, adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("ApplyCompatibility: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings when supported, got %v", warnings)
	}
	got := out.Content[0].Content[0]
	if got.Type != "inlineCard" {
		t.Fatalf("inlineCard removed despite schema support; got %q", got.Type)
	}
}

// Unknown-schema path: when InlineCardSupported is the zero value
// (no information), treat as unsupported. Best-effort emits an
// "unknown compatibility" warning AND degrades.
func TestApplyCompatibility_UnknownSchemaTreatedAsUnsupported(t *testing.T) {
	doc, _, err := adf.Parse([]byte(`{
		"type": "doc", "version": 1, "content": [
			{"type": "paragraph", "content": [
				{"type": "inlineCard", "attrs": {"url": "https://example.com/y"}}
			]}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// FieldCompatibility with no support flag set should be treated as
	// unknown (unsupported).
	schema := adf.FieldCompatibility{Field: "description"}

	_, warnings, err := adf.ApplyCompatibility(doc, schema, adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("ApplyCompatibility: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("unknown schema must surface a warning in best-effort")
	}
	if !warnings[0].Lossy {
		t.Fatalf("degradation warning must be lossy=true")
	}
}

// Compile-time assertion that adf.Warning satisfies the cli.WarningSource
// interface used by cli.WarningFrom (shared envelope spirit).
var _ cli.WarningSource = adf.Warning{}
