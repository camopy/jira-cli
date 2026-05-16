package adf_test

// Strict-validation completeness tests covering the rule-table gaps the
// Phase 07 review found: table-cell content whitelists, content-minItems
// enforcement, blockTaskItem registration, media/card required attrs,
// and the remaining attribute-bearing marks.
//
// Each "invalid" case is a document that PASSED strict validation before
// the rule was added — the validator silently shipped it to Jira. Each
// "valid" case is schema-legal ADF the validator must accept.

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// --- B: tableCell / tableHeader content whitelist ---

func TestValidateStrict_TableCellBareText_Errors(t *testing.T) {
	// A tableCell's content is restricted to the table_cell_content
	// whitelist — a bare text node is not block content and is invalid.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"text","text":"bare"}]}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for bare text node inside a tableCell")
	}
}

func TestValidateStrict_TableHeaderNestedTable_Errors(t *testing.T) {
	// A nested table is not in the table_cell_content whitelist.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"tableHeader","content":[
					{"type":"table","content":[
						{"type":"tableRow","content":[
							{"type":"tableCell","content":[
								{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}]}]}]}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for a nested table inside a tableHeader")
	}
}

// --- C: content minItems / required-content enforcement ---

func TestValidateStrict_EmptyBulletList_Errors(t *testing.T) {
	// bulletList.content has minItems 1 — an empty list is rejected by
	// Jira and must fail strict validation.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"bulletList","content":[]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for an empty bulletList (content minItems 1)")
	}
}

func TestValidateStrict_EmptyPanel_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"panel","attrs":{"panelType":"info"},"content":[]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for an empty panel (content minItems 1)")
	}
}

func TestValidateStrict_EmptyTable_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"table","content":[]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for an empty table (content minItems 1)")
	}
}

func TestValidateStrict_EmptyListItem_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"bulletList","content":[{"type":"listItem","content":[]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for an empty listItem (content minItems 1)")
	}
}

func TestValidateStrict_NonEmptyContainersStillPass(t *testing.T) {
	// minItems enforcement must not regress legitimately-populated nodes.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"panel","attrs":{"panelType":"info"},"content":[
			{"type":"bulletList","content":[
				{"type":"listItem","content":[
					{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("populated panel/list/item should still pass: %v", err)
	}
}

// --- D: blockTaskItem is a known node type ---

func TestValidateStrict_ValidBlockTaskItem_Accepted(t *testing.T) {
	// taskList permits a blockTaskItem child; a schema-valid one (with
	// localId + state) must pass strict validation.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"taskList","attrs":{"localId":"l1"},"content":[
			{"type":"blockTaskItem","attrs":{"localId":"b1","state":"TODO"},"content":[
				{"type":"paragraph","content":[{"type":"text","text":"do it"}]}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("a valid blockTaskItem should pass strict validation: %v", err)
	}
}

func TestValidateStrict_BlockTaskItemMissingState_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"taskList","attrs":{"localId":"l1"},"content":[
			{"type":"blockTaskItem","attrs":{"localId":"b1"},"content":[
				{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for blockTaskItem missing attrs.state")
	}
}

// --- E: media / card required attrs ---

func TestValidateStrict_MediaMissingRequiredAttrs_Errors(t *testing.T) {
	// A file-variant media node needs type+id+collection; missing them
	// must fail strict validation rather than reaching the wire.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"mediaGroup","content":[
			{"type":"media","attrs":{"type":"file"}}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for a file media node missing id/collection")
	}
}

func TestValidateStrict_ValidFileMedia_Accepted(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"mediaGroup","content":[
			{"type":"media","attrs":{"type":"file","id":"abc","collection":"col"}}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("a valid file media node should pass: %v", err)
	}
}

func TestValidateStrict_ValidExternalMedia_Accepted(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"mediaGroup","content":[
			{"type":"media","attrs":{"type":"external","url":"https://example.com/x.png"}}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("a valid external media node should pass: %v", err)
	}
}

func TestValidateStrict_InlineCardMissingURLandData_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[{"type":"inlineCard","attrs":{}}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for inlineCard missing both url and data")
	}
}

func TestValidateStrict_ValidInlineCard_Accepted(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"inlineCard","attrs":{"url":"https://example.com/KAN-1"}}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("a valid inlineCard should pass: %v", err)
	}
}

func TestValidateStrict_EmbedCardMissingLayout_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"embedCard","attrs":{"url":"https://example.com"}}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for embedCard missing attrs.layout")
	}
}

func TestValidateStrict_BlockCardMissingAllBranches_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"blockCard","attrs":{}}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for blockCard with no datasource/url/data")
	}
}

func TestValidateStrict_MediaSingleMissingLayout_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"mediaSingle","attrs":{},"content":[
			{"type":"media","attrs":{"type":"file","id":"a","collection":"c"}}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for mediaSingle missing attrs.layout")
	}
}

// --- F: alignment / indentation / border marks ---

func TestValidateStrict_AlignmentMarkMissingAlign_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","marks":[{"type":"alignment","attrs":{}}],
			"content":[{"type":"text","text":"x"}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for alignment mark missing attrs.align")
	}
}

func TestValidateStrict_AlignmentMarkBadAlign_Errors(t *testing.T) {
	// align enum is center/end only — "left" is not in the schema.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","marks":[{"type":"alignment","attrs":{"align":"left"}}],
			"content":[{"type":"text","text":"x"}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for alignment mark with align 'left'")
	}
}

func TestValidateStrict_ValidAlignmentMark_Accepted(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","marks":[{"type":"alignment","attrs":{"align":"center"}}],
			"content":[{"type":"text","text":"x"}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("a valid alignment mark should pass: %v", err)
	}
}

func TestValidateStrict_IndentationMarkOutOfRange_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","marks":[{"type":"indentation","attrs":{"level":9}}],
			"content":[{"type":"text","text":"x"}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for indentation mark level out of 1-6 range")
	}
}

func TestValidateStrict_BorderMarkMissingAttrs_Errors(t *testing.T) {
	// The border mark is schema-legal on a media node; missing its
	// required color attr must fail strict validation.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"mediaGroup","content":[
			{"type":"media","attrs":{"type":"file","id":"a","collection":"c"},
				"marks":[{"type":"border","attrs":{"size":2}}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for border mark missing attrs.color")
	}
}

func TestValidateStrict_ValidBorderMark_Accepted(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"mediaGroup","content":[
			{"type":"media","attrs":{"type":"file","id":"a","collection":"c"},
				"marks":[{"type":"border","attrs":{"size":2,"color":"#aabbccdd"}}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("a valid border mark should pass: %v", err)
	}
}

// --- G: link mark href must be non-empty ---

func TestValidateStrict_LinkMarkEmptyHref_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":""}}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for link mark with an empty href")
	}
}

// --- H: code mark is text-node-only ---

func TestValidateStrict_CodeMarkOnMention_Errors(t *testing.T) {
	// The code mark is schema-legal only on a text node.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"mention","attrs":{"id":"u1"},"marks":[{"type":"code"}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for a code mark on a mention node")
	}
}
