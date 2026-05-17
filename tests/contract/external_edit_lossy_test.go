package contract

import (
	"context"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/editor"
)

// The external --edit flow MUST round-trip through Markdown for the
// editor buffer while preserving ADF subtrees that have no clean
// Markdown representation (panel, table, inlineCard, mention, etc.)
// as protected opaque blocks. Lossy steps MUST emit warnings.
//
// We exercise the round-trip purely on the editor package — the editor
// command launcher is mocked by writing the same content back to the
// temp file (no-op edit), which is the canonical "user opened editor and
// saved without changes" path.
func TestExternalEditPreservesOpaqueSubtreesAcrossNoOpEdit(t *testing.T) {
	original := []byte(`{
		"type": "doc", "version": 1, "content": [
			{"type": "paragraph", "content": [{"type": "text", "text": "lead"}]},
			{"type": "panel", "attrs": {"panelType": "info"}, "content": [
				{"type": "paragraph", "content": [{"type": "text", "text": "ENC"}]}
			]},
			{"type": "paragraph", "content": [
				{"type": "text", "text": "see also "},
				{"type": "inlineCard", "attrs": {"url": "https://example.com/KAN-1"}}
			]},
			{"type": "paragraph", "content": [{"type": "text", "text": "trailer"}]}
		]
	}`)

	doc, _, err := adf.Parse(original)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out, warnings, err := editor.RoundTripADF(context.Background(), editor.RoundTripADFOptions{
		IssueKey:  "KAN-1",
		FieldName: "description",
		Document:  doc,
		EditFn:    func(_ context.Context, path string) error { return nil }, // no-op edit
	})
	if err != nil {
		t.Fatalf("RoundTripADF: %v", err)
	}

	// Lossy step warning expected — the panel and inlineCard cannot be
	// represented in plain Markdown, so they're round-tripped as opaque
	// placeholders. Warnings communicate the lossy step happened.
	if len(warnings) == 0 {
		t.Fatal("no warnings emitted for opaque subtree round-trip; structured warnings required")
	}

	// Re-marshal and verify the panel/inlineCard URL/text survived.
	got, err := adf.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	gotStr := string(got)
	for _, marker := range []string{`"panel"`, `"inlineCard"`, "https://example.com/KAN-1", "ENC"} {
		if !strings.Contains(gotStr, marker) {
			t.Fatalf("opaque marker %q missing after round-trip; got: %s", marker, gotStr)
		}
	}
}
