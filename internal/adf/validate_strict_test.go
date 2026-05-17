package adf_test

// Strict registry-backed validation tests.
//
// These exercise the validation rules derived from the pinned ADF JSON
// schema (@atlaskit/adf-schema 52.11.3): required attrs, attr types,
// content/nesting rules, and per-node mark rules. Strict mode rejects;
// best-effort mode warns.

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

func mustParse(t *testing.T, raw string) adf.Document {
	t.Helper()
	doc, _, err := adf.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc
}

// --- required attrs ---

func TestValidateStrict_HeadingMissingLevel_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"heading","content":[{"type":"text","text":"hi"}]}
	]}`)
	_, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("expected error for heading missing attrs.level")
	}
	if !strings.Contains(err.Error(), "level") {
		t.Errorf("error should name 'level'; got: %v", err)
	}
}

func TestValidateStrict_HeadingLevelOutOfRange_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"hi"}]}
	]}`)
	_, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("expected error for heading level out of 1-6 range")
	}
}

func TestValidateStrict_PanelMissingPanelType_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"panel","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}
	]}`)
	_, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("expected error for panel missing attrs.panelType")
	}
	if !strings.Contains(err.Error(), "panelType") {
		t.Errorf("error should name 'panelType'; got: %v", err)
	}
}

// panelType "tip" and "custom" are valid per the JSON schema (7-value
// enum) even though Atlassian prose docs only list 5.
func TestValidateStrict_PanelTipAndCustom_Accepted(t *testing.T) {
	for _, pt := range []string{"tip", "custom", "info", "note", "warning", "error", "success"} {
		doc := mustParse(t, `{"type":"doc","version":1,"content":[
			{"type":"panel","attrs":{"panelType":"`+pt+`"},"content":[
				{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}
		]}`)
		if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
			t.Errorf("panelType %q should be valid: %v", pt, err)
		}
	}
}

func TestValidateStrict_PanelBadPanelType_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"panel","attrs":{"panelType":"banana"},"content":[
			{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for invalid panelType 'banana'")
	}
}

func TestValidateStrict_StatusMissingAttrs_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[{"type":"status","attrs":{"text":"DONE"}}]}
	]}`)
	_, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("expected error for status missing attrs.color")
	}
}

func TestValidateStrict_MentionMissingID_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[{"type":"mention","attrs":{"text":"@x"}}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for mention missing attrs.id")
	}
}

func TestValidateStrict_TaskItemStateEnum_Rejected(t *testing.T) {
	// taskItem.state is enum TODO/DONE.
	bad := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"taskList","attrs":{"localId":"l1"},"content":[
			{"type":"taskItem","attrs":{"localId":"t1","state":"MAYBE"}}]}
	]}`)
	if _, err := adf.ValidateDoc(bad, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for taskItem state 'MAYBE'")
	}
	good := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"taskList","attrs":{"localId":"l1"},"content":[
			{"type":"taskItem","attrs":{"localId":"t1","state":"DONE"}}]}
	]}`)
	if _, err := adf.ValidateDoc(good, adfmode.ModeStrict); err != nil {
		t.Errorf("taskItem state 'DONE' should be valid: %v", err)
	}
}

func TestValidateStrict_DecisionItemStateNotEnum_Accepted(t *testing.T) {
	// decisionItem.state is a plain string (no enum) per the schema.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"decisionList","attrs":{"localId":"l1"},"content":[
			{"type":"decisionItem","attrs":{"localId":"d1","state":"ANYTHING"},
				"content":[{"type":"text","text":"decided"}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("decisionItem with arbitrary state string should be valid: %v", err)
	}
}

func TestValidateStrict_WrongAttrType_Errors(t *testing.T) {
	// heading.level must be a number, not a string.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"heading","attrs":{"level":"2"},"content":[{"type":"text","text":"x"}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for heading.level being a string")
	}
}

// --- content / nesting rules ---

func TestValidateStrict_RootLevelText_Errors(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{
		{Type: "text", Text: "loose text at root"},
	}}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for bare text node at document root")
	}
}

func TestValidateStrict_BlockInsideParagraph_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x"},
			{"type":"panel","attrs":{"panelType":"info"},"content":[
				{"type":"paragraph","content":[{"type":"text","text":"nested"}]}]}
		]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for block node inside a paragraph")
	}
}

func TestValidateStrict_TableRowHoldsOnlyCells_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"paragraph","content":[{"type":"text","text":"not a cell"}]}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for tableRow holding a non-cell child")
	}
}

func TestValidateStrict_ValidTable_Accepted(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"tableHeader","content":[
					{"type":"paragraph","content":[{"type":"text","text":"H"}]}]},
				{"type":"tableCell","content":[
					{"type":"paragraph","content":[{"type":"text","text":"C"}]}]}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("valid table should pass: %v", err)
	}
}

func TestValidateStrict_HeadingInsideListItem_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"bulletList","content":[
			{"type":"listItem","content":[
				{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"x"}]}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for heading directly inside a listItem")
	}
}

func TestValidateStrict_BulletListChildMustBeListItem_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"bulletList","content":[
			{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for non-listItem child of bulletList")
	}
}

// --- mark rules ---

func TestValidateStrict_LinkMarkMissingHref_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x","marks":[{"type":"link","attrs":{"title":"t"}}]}]}
	]}`)
	_, err := adf.ValidateDoc(doc, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("expected error for link mark missing href")
	}
	if !strings.Contains(err.Error(), "href") {
		t.Errorf("error should name 'href'; got: %v", err)
	}
}

func TestValidateStrict_CodeMarkMutuallyExclusiveWithStrong_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x","marks":[{"type":"code"},{"type":"strong"}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for code mark combined with strong")
	}
}

func TestValidateStrict_CodePlusLink_Accepted(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x","marks":[
				{"type":"code"},{"type":"link","attrs":{"href":"https://x"}}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("code + link should be a valid combination: %v", err)
	}
}

func TestValidateStrict_SubsupBadType_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x","marks":[{"type":"subsup","attrs":{"type":"middle"}}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for subsup mark with type 'middle'")
	}
}

func TestValidateStrict_TextColorBadHex_Errors(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"x","marks":[{"type":"textColor","attrs":{"color":"red"}}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for textColor mark with non-hex color")
	}
}

func TestValidateStrict_CodeBlockTextWithMarks_Errors(t *testing.T) {
	// codeBlock content text nodes carry zero marks.
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"codeBlock","content":[
			{"type":"text","text":"x","marks":[{"type":"strong"}]}]}
	]}`)
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err == nil {
		t.Fatal("expected error for marks on codeBlock text content")
	}
}

// --- best-effort downgrade of strict-only failures ---

func TestValidateStrict_BestEffortMissingAttr_Warns(t *testing.T) {
	doc := mustParse(t, `{"type":"doc","version":1,"content":[
		{"type":"heading","content":[{"type":"text","text":"hi"}]}
	]}`)
	warnings, err := adf.ValidateDoc(doc, adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("best-effort: unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("best-effort: expected a warning for heading missing level")
	}
}
