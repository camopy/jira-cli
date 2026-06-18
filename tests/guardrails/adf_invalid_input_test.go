// MOTIVATION: ADF that is structurally invalid per Atlassian's schema — an
// empty text node (text ""), a mention with an empty id, and similar
// contentless-but-required constructs — is rejected by Jira with an opaque
// 400 INVALID_INPUT that names only the top-level field, not the offending
// node. Document generators (and humans hand-editing JSON) emit these
// routinely, e.g. blank cells in a table. The mutation pipeline is the single
// chokepoint every write funnels through, so it MUST repair the losslessly
// fixable cases and reject the rest BEFORE the document reaches the wire —
// otherwise the failure only surfaces as an unactionable server error. These
// guardrails fail the build if that protection is removed or bypassed.
// Comments in this file are PROVENANCE ONLY and MUST NOT be a source of
// implementation, fixtures, wording, or test logic.
package guardrails

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// countEmptyText reports how many empty text nodes remain anywhere in the tree.
func countEmptyText(nodes []adf.Node) int {
	n := 0
	for _, node := range nodes {
		if node.Type == "text" && node.Text == "" {
			n++
		}
		n += countEmptyText(node.Content)
	}
	return n
}

// The mutation pipeline must strip empty text nodes from the document it
// submits, in strict mode, so an INVALID_INPUT-class document never reaches
// Jira. Exercised through RunMutation — the shared chokepoint for comment,
// issue create/edit, and worklog — so relocating or dropping the normalization
// step fails here. The shape mirrors the real trigger (a blank table cell)
// without reproducing any specific document.
func TestMutationPipelineStripsEmptyTextBeforeSubmit(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "table", Content: []adf.Node{
			{Type: "tableRow", Content: []adf.Node{
				{Type: "tableCell", Content: []adf.Node{
					{Type: "paragraph", Content: []adf.Node{
						{Type: "text", Text: "kept"},
						{Type: "text", Text: ""},
					}},
				}},
				{Type: "tableCell", Content: []adf.Node{
					{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: ""}}},
				}},
			}},
		}},
	}}

	res := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		ADFDoc: &doc,
		DryRun: true,
	})

	if res.Aborted {
		t.Fatalf("strict mutation aborted on a repairable empty text node: %v", res.Err)
	}
	if res.SubmitADF == nil {
		t.Fatal("pipeline produced no SubmitADF")
	}
	if n := countEmptyText(res.SubmitADF.Content); n != 0 {
		t.Fatalf("INVALID_INPUT-class regression: %d empty text node(s) would reach Jira", n)
	}
}

// A document whose only inline content is an empty text node must still leave a
// valid parent behind (an empty paragraph), so normalization can never turn a
// blank cell into a schema violation of its own (tableCell requires >=1 block).
func TestMutationPipelineLeavesValidParentForBlankCell(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "table", Content: []adf.Node{
			{Type: "tableRow", Content: []adf.Node{
				{Type: "tableCell", Content: []adf.Node{
					{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: ""}}},
				}},
			}},
		}},
	}}

	res := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		ADFDoc: &doc,
		DryRun: true,
	})
	if res.Aborted {
		t.Fatalf("normalization produced an invalid document for a blank cell: %v", res.Err)
	}
	cell := res.SubmitADF.Content[0].Content[0].Content[0]
	if cell.Type != "tableCell" || len(cell.Content) != 1 || cell.Content[0].Type != "paragraph" {
		t.Fatalf("blank cell lost its block child: %+v", cell)
	}
}

// A required reference attr that is present but empty (mention id) is NOT
// losslessly repairable, so strict validation must reject it locally with the
// path — never forward it to an opaque server-side INVALID_INPUT.
func TestStrictValidationRejectsEmptyMentionID(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{
			{Type: "mention", Attrs: map[string]any{"id": ""}},
		}},
	}}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("strict validation forwarded a mention with an empty id (INVALID_INPUT-class)")
	}
}
