package pipeline_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// A lossy Markdown→ADF conversion (warnings carried in
// MutationInput.MarkdownWarnings) must abort the mutation in strict
// mode before submission — user content was dropped during conversion
// and submitting silently would lose it.
func TestRunMutation_MarkdownLossy_StrictAborts(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "kept"}}},
	}}
	res := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		ADFDoc: &doc,
		MarkdownWarnings: []adf.Warning{{
			Type:     "markdown_lossy_conversion",
			Message:  "Markdown table is not supported and was dropped during ADF conversion",
			NodeType: "table",
			Lossy:    true,
		}},
		DryRun: true,
	})
	if !res.Aborted {
		t.Fatal("strict mode must abort when Markdown conversion was lossy")
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "table") {
		t.Fatalf("abort error should name the dropped construct; got %v", res.Err)
	}
}

// In best-effort mode the same lossy conversion warns but does not
// abort — the partial document is submitted.
func TestRunMutation_MarkdownLossy_BestEffortWarns(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "kept"}}},
	}}
	res := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeBestEffort,
		ADFDoc: &doc,
		MarkdownWarnings: []adf.Warning{{
			Type:     "markdown_lossy_conversion",
			Message:  "Markdown table is not supported and was dropped during ADF conversion",
			NodeType: "table",
			Lossy:    true,
		}},
		DryRun: true,
	})
	if res.Aborted {
		t.Fatalf("best-effort must not abort on lossy Markdown; err=%v", res.Err)
	}
	found := false
	for _, w := range res.Warnings {
		if w.Type == "markdown_lossy_conversion" {
			found = true
		}
	}
	if !found {
		t.Fatalf("best-effort must surface the Markdown lossy warning; got %+v", res.Warnings)
	}
}

// Non-lossy Markdown warnings (none) leave the mutation untouched.
func TestRunMutation_NoMarkdownWarnings_StrictProceeds(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{{Type: "text", Text: "kept"}}},
	}}
	res := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		ADFDoc: &doc,
		DryRun: true,
	})
	if res.Aborted {
		t.Fatalf("clean conversion must not abort; err=%v", res.Err)
	}
}
