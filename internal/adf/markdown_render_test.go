package adf_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// docOf wraps blocks into a Document the way Jira delivers them.
func docOf(nodes ...adf.Node) adf.Document {
	return adf.Document{Type: "doc", Version: 1, Content: nodes}
}

func para(inline ...adf.Node) adf.Node {
	return adf.Node{Type: "paragraph", Content: inline}
}

func text(s string) adf.Node { return adf.Node{Type: "text", Text: s} }

func TestToMarkdownBlockquote(t *testing.T) {
	doc := docOf(adf.Node{Type: "blockquote", Content: []adf.Node{
		para(text("first line")),
		para(text("second para")),
	}})
	got := adf.ToMarkdown(doc)
	want := "> first line\n>\n> second para\n"
	if got != want {
		t.Errorf("blockquote:\ngot  %q\nwant %q", got, want)
	}
}

func TestToMarkdownRule(t *testing.T) {
	doc := docOf(para(text("above")), adf.Node{Type: "rule"}, para(text("below")))
	got := adf.ToMarkdown(doc)
	if !strings.Contains(got, "\n\n---\n\n") {
		t.Errorf("rule not rendered as ---:\n%q", got)
	}
}

func TestToMarkdownHardBreak(t *testing.T) {
	doc := docOf(para(text("line one"), adf.Node{Type: "hardBreak"}, text("line two")))
	got := adf.ToMarkdown(doc)
	if !strings.Contains(got, "line one  \nline two") {
		t.Errorf("hardBreak not a markdown line break:\n%q", got)
	}
}

func TestToMarkdownMention(t *testing.T) {
	doc := docOf(para(
		text("hello "),
		adf.Node{Type: "mention", Attrs: map[string]any{"id": "5b10a", "text": "@johndoe"}},
		text(" — please review."),
	))
	got := adf.ToMarkdown(doc)
	if !strings.Contains(got, "hello @johndoe — please review.") {
		t.Errorf("mention not rendered as @name:\n%q", got)
	}
}

func TestToMarkdownMentionWithoutAtPrefix(t *testing.T) {
	doc := docOf(para(adf.Node{Type: "mention", Attrs: map[string]any{"text": "janedoe"}}))
	if got := adf.ToMarkdown(doc); !strings.Contains(got, "@janedoe") {
		t.Errorf("bare mention text should gain the @ prefix:\n%q", got)
	}
}

func TestToMarkdownEmoji(t *testing.T) {
	doc := docOf(para(
		text("shipped "),
		adf.Node{Type: "emoji", Attrs: map[string]any{"shortName": ":rocket:", "text": "🚀"}},
	))
	if got := adf.ToMarkdown(doc); !strings.Contains(got, "shipped 🚀") {
		t.Errorf("emoji should render its text:\n%q", got)
	}
}

func TestToMarkdownStatus(t *testing.T) {
	doc := docOf(para(adf.Node{Type: "status", Attrs: map[string]any{"text": "In Progress", "color": "yellow"}}))
	if got := adf.ToMarkdown(doc); !strings.Contains(got, "`In Progress`") {
		t.Errorf("status should render as an inline code chip:\n%q", got)
	}
}

func TestToMarkdownInlineCard(t *testing.T) {
	doc := docOf(para(adf.Node{Type: "inlineCard", Attrs: map[string]any{"url": "https://example.atlassian.net/browse/JCT-1"}}))
	got := adf.ToMarkdown(doc)
	if !strings.Contains(got, "<https://example.atlassian.net/browse/JCT-1>") {
		t.Errorf("inlineCard should render as an autolink:\n%q", got)
	}
}

func TestToMarkdownPanel(t *testing.T) {
	doc := docOf(adf.Node{
		Type:  "panel",
		Attrs: map[string]any{"panelType": "warning"},
		Content: []adf.Node{
			para(text("Take note of this.")),
		},
	})
	got := adf.ToMarkdown(doc)
	want := "> **Warning**\n>\n> Take note of this.\n"
	if got != want {
		t.Errorf("panel:\ngot  %q\nwant %q", got, want)
	}
}

func TestToMarkdownTable(t *testing.T) {
	cell := func(kind, s string) adf.Node {
		return adf.Node{Type: kind, Content: []adf.Node{para(text(s))}}
	}
	doc := docOf(adf.Node{Type: "table", Content: []adf.Node{
		{Type: "tableRow", Content: []adf.Node{cell("tableHeader", "Key"), cell("tableHeader", "Value")}},
		{Type: "tableRow", Content: []adf.Node{cell("tableCell", "alpha"), cell("tableCell", "1")}},
	}})
	got := adf.ToMarkdown(doc)
	want := "| Key | Value |\n| --- | --- |\n| alpha | 1 |\n"
	if got != want {
		t.Errorf("table:\ngot  %q\nwant %q", got, want)
	}
}

func TestToMarkdownMediaPlaceholder(t *testing.T) {
	doc := docOf(adf.Node{Type: "mediaSingle", Content: []adf.Node{
		{Type: "media", Attrs: map[string]any{"id": "abc-123", "type": "file", "alt": "screenshot.png"}},
	}})
	got := adf.ToMarkdown(doc)
	if !strings.Contains(got, "[attachment: screenshot.png]") {
		t.Errorf("media should be a labeled placeholder:\n%q", got)
	}
}

func TestToMarkdownMediaWithoutAltUsesID(t *testing.T) {
	doc := docOf(adf.Node{Type: "mediaGroup", Content: []adf.Node{
		{Type: "media", Attrs: map[string]any{"id": "abc-123", "type": "file"}},
	}})
	if got := adf.ToMarkdown(doc); !strings.Contains(got, "[attachment: abc-123]") {
		t.Errorf("media without alt should fall back to its id:\n%q", got)
	}
}

func TestToMarkdownStrikeMark(t *testing.T) {
	doc := docOf(para(adf.Node{Type: "text", Text: "gone", Marks: []adf.Mark{{Type: "strike"}}}))
	if got := adf.ToMarkdown(doc); !strings.Contains(got, "~~gone~~") {
		t.Errorf("strike mark not rendered:\n%q", got)
	}
}

// The newly renderable constructs must no longer be reported as lossy.
func TestLossySetShrinksWithNewRenderables(t *testing.T) {
	doc := docOf(
		adf.Node{Type: "blockquote", Content: []adf.Node{para(text("q"))}},
		adf.Node{Type: "panel", Attrs: map[string]any{"panelType": "info"}, Content: []adf.Node{para(text("p"))}},
		adf.Node{Type: "rule"},
		para(
			adf.Node{Type: "mention", Attrs: map[string]any{"text": "@a"}},
			adf.Node{Type: "hardBreak"},
			adf.Node{Type: "text", Text: "s", Marks: []adf.Mark{{Type: "strike"}}},
		),
	)
	res := adf.ToMarkdownLossy(doc)
	if len(res.LossyConstructs) != 0 {
		t.Errorf("newly renderable constructs still lossy: %v", res.LossyConstructs)
	}
}

func TestLossyStillReportsUnknown(t *testing.T) {
	doc := docOf(
		adf.Node{Type: "extension", Attrs: map[string]any{"extensionKey": "x"}},
		para(adf.Node{Type: "text", Text: "u", Marks: []adf.Mark{{Type: "underline"}}}),
	)
	res := adf.ToMarkdownLossy(doc)
	want := []string{"extension", "underline"}
	if len(res.LossyConstructs) != 2 || res.LossyConstructs[0] != want[0] || res.LossyConstructs[1] != want[1] {
		t.Errorf("LossyConstructs = %v, want %v", res.LossyConstructs, want)
	}
}

// ToPlain must flatten the same attr-only inline nodes the markdown path
// renders, so the human/plain output never silently drops them.
func TestToPlainRendersAttrOnlyInlineNodes(t *testing.T) {
	doc := docOf(
		para(
			text("ping "),
			adf.Node{Type: "mention", Attrs: map[string]any{"text": "@johndoe"}},
			text(" about "),
			adf.Node{Type: "inlineCard", Attrs: map[string]any{"url": "https://example.com/x"}},
		),
		para(
			adf.Node{Type: "status", Attrs: map[string]any{"text": "In Progress"}},
			adf.Node{Type: "emoji", Attrs: map[string]any{"text": "🚀"}},
		),
		adf.Node{Type: "mediaSingle", Content: []adf.Node{
			{Type: "media", Attrs: map[string]any{"alt": "screenshot.png"}},
		}},
	)
	got := adf.ToPlain(doc)
	for _, want := range []string{"@johndoe", "https://example.com/x", "In Progress", "🚀", "[attachment: screenshot.png]"} {
		if !strings.Contains(got, want) {
			t.Errorf("ToPlain missing %q in:\n%q", want, got)
		}
	}
}

func TestToMarkdownTablePadsRaggedRows(t *testing.T) {
	cell := func(kind, s string) adf.Node {
		return adf.Node{Type: kind, Content: []adf.Node{para(text(s))}}
	}
	doc := docOf(adf.Node{Type: "table", Content: []adf.Node{
		{Type: "tableRow", Content: []adf.Node{cell("tableHeader", "A")}},
		{Type: "tableRow", Content: []adf.Node{cell("tableCell", "1"), cell("tableCell", "2")}},
	}})
	got := adf.ToMarkdown(doc)
	want := "| A |  |\n| --- | --- |\n| 1 | 2 |\n"
	if got != want {
		t.Errorf("ragged table:\ngot  %q\nwant %q", got, want)
	}
}

func TestToMarkdownTableCellWithTwoParagraphs(t *testing.T) {
	cell := adf.Node{Type: "tableCell", Content: []adf.Node{para(text("foo")), para(text("bar"))}}
	doc := docOf(adf.Node{Type: "table", Content: []adf.Node{
		{Type: "tableRow", Content: []adf.Node{{Type: "tableHeader", Content: []adf.Node{para(text("H"))}}}},
		{Type: "tableRow", Content: []adf.Node{cell}},
	}})
	if got := adf.ToMarkdown(doc); !strings.Contains(got, "| foo bar |") {
		t.Errorf("multi-block cell should join with a space:\n%q", got)
	}
}
