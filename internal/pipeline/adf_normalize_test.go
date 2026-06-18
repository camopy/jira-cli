package pipeline_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// An empty text node ({"type":"text","text":""}) is invalid ADF — Jira rejects
// the whole document with an opaque INVALID_INPUT — yet it carries no content.
// The mutation pipeline must normalize it away before validation/submission,
// in strict mode, so the document Jira receives (SubmitADF) is clean and a
// non-lossy warning records that the body was repaired. The synthetic shape
// here mirrors the real trigger: a blank cell in a table row.
func TestRunMutation_StripsEmptyTextNodeBeforeSubmit(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "table", Content: []adf.Node{
			{Type: "tableRow", Content: []adf.Node{
				{Type: "tableCell", Content: []adf.Node{
					{Type: "paragraph", Content: []adf.Node{
						{Type: "text", Text: "value"},
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
		t.Fatal("SubmitADF is nil; expected the normalized document")
	}

	// No empty text node may survive anywhere in the submitted tree.
	if n := countEmptyText(res.SubmitADF.Content); n != 0 {
		t.Fatalf("submitted document still carries %d empty text node(s)", n)
	}
	// The real content must survive.
	row := res.SubmitADF.Content[0].Content[0]
	if got := row.Content[0].Content[0].Content[0].Text; got != "value" {
		t.Fatalf("real cell text was lost: got %q, want value", got)
	}
	// The blank cell keeps its (now-empty) paragraph, so the cell stays valid.
	blank := row.Content[1].Content[0]
	if blank.Type != "paragraph" || len(blank.Content) != 0 {
		t.Fatalf("blank cell should hold one empty paragraph, got %+v", blank)
	}

	// The repair is surfaced as a non-lossy normalization warning.
	found := false
	for _, w := range res.Warnings {
		if w.Type == "adf_normalized" {
			found = true
			if w.Lossy {
				t.Fatalf("normalization warning must be non-lossy: %+v", w)
			}
		}
	}
	if !found {
		t.Fatalf("expected an adf_normalized warning; got %+v", res.Warnings)
	}
}

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
