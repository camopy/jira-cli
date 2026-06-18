package adf_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// A mention with an empty id is a broken reference Jira rejects; strict
// validation must catch it locally (with the path) rather than letting Jira
// return an opaque INVALID_INPUT. Best-effort surfaces it as a warning.
func TestValidateDoc_EmptyMentionID(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{
			{Type: "mention", Attrs: map[string]any{"id": ""}},
		}},
	}}

	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("strict validation accepted a mention with an empty id")
	}

	warnings, err := adf.ValidateDoc(doc, adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("best-effort must not error on an empty mention id: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("best-effort should warn about the empty mention id")
	}
}

// A non-empty mention id is valid and must pass.
func TestValidateDoc_NonEmptyMentionIDPasses(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{
			{Type: "mention", Attrs: map[string]any{"id": "557058:abc"}},
		}},
	}}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Fatalf("valid mention rejected: %v", err)
	}
}

// An emoji with an empty shortName is likewise rejected by strict validation.
func TestValidateDoc_EmptyEmojiShortName(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "paragraph", Content: []adf.Node{
			{Type: "emoji", Attrs: map[string]any{"shortName": ""}},
		}},
	}}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("strict validation accepted an emoji with an empty shortName")
	}
}
