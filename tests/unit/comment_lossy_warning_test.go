package unit

import (
	"slices"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Comment list emits a structured warning per lossy comment with
// sorted-unique lossy_constructs. CollectLossyCommentWarnings is the helper
// used by `cmd/jira/issue_comment.go` to build the envelope's warnings[].
//
// Test contract:
//   - non-lossy comment → no warning entry
//   - lossy comment with multiple lossy node types → one entry, sorted unique
//   - comments with nil/missing body skipped without panicking
//   - returns nil on empty input

func TestCollectLossyCommentWarningsEmpty(t *testing.T) {
	got := jira.CollectLossyCommentWarnings(nil)
	if got != nil {
		t.Fatalf("got %v; want nil", got)
	}
}

func TestCollectLossyCommentWarningsSkipsNonLossy(t *testing.T) {
	doc := adfDocFromMarkdown(t, "plain paragraph")
	id := "100"
	comments := []*jira.Comment{
		{ID: &id, Body: &doc},
	}
	got := jira.CollectLossyCommentWarnings(comments)
	if len(got) != 0 {
		t.Fatalf("got %d warnings; want 0 (no lossy content)", len(got))
	}
}

func TestCollectLossyCommentWarningsEmitsOnePerLossyComment(t *testing.T) {
	plainDoc := adfDocFromMarkdown(t, "plain")
	plainID := "100"

	// Build an ADF doc by hand with two distinct lossy node types so we can
	// assert the sorted-unique behavior. inlineCard/panel/table/mention are
	// renderable now, so reach for constructs the renderer still drops.
	lossyDoc := adf.Document{
		Type:    "doc",
		Version: 1,
		Content: []adf.Node{
			{
				Type: "paragraph",
				Content: []adf.Node{
					{Type: "decisionItem"},
					{Type: "text", Text: "see"},
				},
			},
			{Type: "extension", Attrs: map[string]any{"extensionKey": "x"}},
		},
	}
	lossyID := "201"
	comments := []*jira.Comment{
		{ID: &plainID, Body: &plainDoc},
		{ID: &lossyID, Body: &lossyDoc},
	}

	got := jira.CollectLossyCommentWarnings(comments)
	if len(got) != 1 {
		t.Fatalf("warnings = %d; want 1 (only comment 201 is lossy)", len(got))
	}
	if got[0].CommentID != "201" {
		t.Errorf("warning.comment_id = %q; want 201", got[0].CommentID)
	}
	if got[0].Type != "adf-lossy-comment" {
		t.Errorf("warning.type = %q; want adf-lossy-comment", got[0].Type)
	}

	// LossyConstructs must be sorted unique. decisionItem < extension.
	want := []string{"decisionItem", "extension"}
	if !slices.Equal(got[0].LossyConstructs, want) {
		t.Errorf("lossy_constructs = %v; want %v (sorted unique)", got[0].LossyConstructs, want)
	}
}

func TestCollectLossyCommentWarningsTolerantOfNilBody(t *testing.T) {
	id := "100"
	comments := []*jira.Comment{
		nil,
		{ID: &id, Body: nil},
	}
	got := jira.CollectLossyCommentWarnings(comments)
	if len(got) != 0 {
		t.Fatalf("got %d warnings; want 0 (nil tolerant)", len(got))
	}
}

// adfDocFromMarkdown is a tiny helper local to this file. It discards
// conversion warnings — callers here use plain Markdown that converts
// without loss.
func adfDocFromMarkdown(t *testing.T, md string) adf.Document {
	t.Helper()
	doc, _, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("adf.FromMarkdownLossy(%q): %v", md, err)
	}
	return doc
}
