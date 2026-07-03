package adf_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// GFM task-list items author real ADF taskList/taskItem nodes: state from
// the checkbox, generated localIds, and the result passes strict schema
// validation end to end.
func TestFromMarkdownAuthorsTaskList(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("- [ ] buy milk\n- [x] drink milk\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("a pure task list must convert without warnings, got %v", warnings)
	}
	if len(doc.Content) != 1 || doc.Content[0].Type != "taskList" {
		t.Fatalf("expected a single taskList, got %+v", doc.Content)
	}
	list := doc.Content[0]
	if id, _ := list.Attrs["localId"].(string); id == "" {
		t.Error("taskList must carry a generated localId")
	}
	if len(list.Content) != 2 {
		t.Fatalf("expected 2 task items, got %d", len(list.Content))
	}
	for i, state := range []string{"TODO", "DONE"} {
		item := list.Content[i]
		if item.Type != "taskItem" {
			t.Errorf("item %d type = %q, want taskItem", i, item.Type)
		}
		if got, _ := item.Attrs["state"].(string); got != state {
			t.Errorf("item %d state = %q, want %q", i, got, state)
		}
		if id, _ := item.Attrs["localId"].(string); id == "" {
			t.Errorf("item %d missing generated localId", i)
		}
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("authored task list must pass strict validation: %v", err)
	}
}

// A nested pure task list nests as a taskList sibling inside the parent
// taskList — ADF's shape for indented action items.
func TestFromMarkdownNestsTaskLists(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("- [ ] parent\n  - [x] child\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	list := doc.Content[0]
	if list.Type != "taskList" || len(list.Content) != 2 {
		t.Fatalf("expected taskList with taskItem + nested taskList, got %+v", list)
	}
	if list.Content[0].Type != "taskItem" || list.Content[1].Type != "taskList" {
		t.Fatalf("expected [taskItem, taskList], got [%s, %s]", list.Content[0].Type, list.Content[1].Type)
	}
	nested := list.Content[1]
	if len(nested.Content) != 1 || nested.Content[0].Type != "taskItem" {
		t.Fatalf("nested taskList should hold one taskItem, got %+v", nested.Content)
	}
	if state, _ := nested.Content[0].Attrs["state"].(string); state != "DONE" {
		t.Errorf("nested item state = %q, want DONE", state)
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("nested task list must pass strict validation: %v", err)
	}
}

// A mixed list (checkbox and plain items) stays a bulletList — ADF forbids
// taskItem outside a pure taskList — with the checkboxes degraded to
// literal text and one non-lossy downgrade warning.
func TestFromMarkdownMixedListDegradesCheckboxes(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("- [ ] boxed\n- plain\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Type != "bulletList" {
		t.Fatalf("mixed list must stay a bulletList, got %+v", doc.Content)
	}
	var downgrades int
	for _, w := range warnings {
		if w.Lossy {
			t.Errorf("mixed-list degrade must be non-lossy, got lossy warning %+v", w)
		}
		downgrades++
	}
	if downgrades != 1 {
		t.Errorf("expected exactly one downgrade warning, got %d: %v", downgrades, warnings)
	}
	if md := adf.ToMarkdown(doc); !strings.Contains(md, "[ ] boxed") {
		t.Errorf("checkbox should degrade to literal text, got %q", md)
	}
}

// Task lists round-trip: markdown → ADF → markdown preserves items and
// states, and the lossy detector reports nothing.
func TestTaskListRoundTripsThroughMarkdown(t *testing.T) {
	source := "- [ ] buy milk\n- [x] drink milk\n"
	doc, _, err := adf.FromMarkdownLossy(source)
	if err != nil {
		t.Fatal(err)
	}
	result := adf.ToMarkdownLossy(doc)
	if len(result.LossyConstructs) != 0 {
		t.Errorf("task list rendering must not be lossy, got %v", result.LossyConstructs)
	}
	if result.Markdown != source {
		t.Errorf("round-trip drifted:\n got %q\nwant %q", result.Markdown, source)
	}
}

// ToPlain shows checkbox state instead of dropping it.
func TestToPlainRendersTaskState(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("- [ ] open item\n- [x] closed item\n")
	if err != nil {
		t.Fatal(err)
	}
	text := adf.ToPlain(doc)
	if !strings.Contains(text, "[ ] open item") || !strings.Contains(text, "[x] closed item") {
		t.Errorf("plain rendering should carry checkbox state, got %q", text)
	}
}

// A Jira-authored decision list renders with the editor's own "<>"
// decision marker instead of being reported lossy.
func TestDecisionListRendersWithMarker(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{{
		Type:  "decisionList",
		Attrs: map[string]any{"localId": "dl-1"},
		Content: []adf.Node{{
			Type:    "decisionItem",
			Attrs:   map[string]any{"localId": "di-1", "state": "DECIDED"},
			Content: []adf.Node{{Type: "text", Text: "ship it"}},
		}},
	}}}
	result := adf.ToMarkdownLossy(doc)
	if len(result.LossyConstructs) != 0 {
		t.Errorf("decision list rendering must not be lossy, got %v", result.LossyConstructs)
	}
	if want := "- <> ship it"; !strings.Contains(result.Markdown, want) {
		t.Errorf("markdown = %q, want it to contain %q", result.Markdown, want)
	}
	if plain := adf.ToPlain(doc); !strings.Contains(plain, "<> ship it") {
		t.Errorf("plain = %q, want the decision marker", plain)
	}
}

// A Jira-authored blockTaskItem (block content inside a task) flattens
// onto its checkbox line.
func TestBlockTaskItemRendersFlattened(t *testing.T) {
	doc := adf.Document{Type: "doc", Version: 1, Content: []adf.Node{{
		Type:  "taskList",
		Attrs: map[string]any{"localId": "tl-1"},
		Content: []adf.Node{{
			Type:  "blockTaskItem",
			Attrs: map[string]any{"localId": "bt-1", "state": "DONE"},
			Content: []adf.Node{{
				Type:    "paragraph",
				Content: []adf.Node{{Type: "text", Text: "reviewed the plan"}},
			}},
		}},
	}}}
	result := adf.ToMarkdownLossy(doc)
	if len(result.LossyConstructs) != 0 {
		t.Errorf("blockTaskItem rendering must not be lossy, got %v", result.LossyConstructs)
	}
	if want := "- [x] reviewed the plan"; !strings.Contains(result.Markdown, want) {
		t.Errorf("markdown = %q, want it to contain %q", result.Markdown, want)
	}
}

// Block content Markdown puts inside a task item that ADF cannot nest
// there (a code block) hoists after the list with a non-lossy downgrade —
// content moves, never vanishes.
func TestTaskItemBlockContentHoists(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("- [ ] check output\n\n  ```\n  go test\n  ```\n")
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, n := range doc.Content {
		types = append(types, n.Type)
	}
	if len(doc.Content) != 2 || doc.Content[0].Type != "taskList" || doc.Content[1].Type != "codeBlock" {
		t.Fatalf("expected [taskList, codeBlock (hoisted)], got %v", types)
	}
	lossy := false
	for _, w := range warnings {
		lossy = lossy || w.Lossy
	}
	if lossy {
		t.Errorf("hoisting is a downgrade, not a loss: %v", warnings)
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Errorf("hoisted result must pass strict validation: %v", err)
	}
}
