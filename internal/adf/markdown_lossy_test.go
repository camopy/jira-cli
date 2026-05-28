package adf_test

// Honest Markdown→ADF conversion tests.
//
// FromMarkdownLossy must emit a structured warning for every Markdown
// construct it cannot represent faithfully in the supported ADF node
// set. Silent drops hide content loss from callers and from strict-mode
// abort gates.

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// A GFM table has no supported ADF authoring path here — conversion
// must warn rather than drop the table silently.
func TestFromMarkdownLossy_TableWarns(t *testing.T) {
	md := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	_, warnings, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning for an unsupported Markdown table")
	}
}

// An image is not a supported authoring node — must warn.
func TestFromMarkdownLossy_ImageWarns(t *testing.T) {
	md := "![alt](https://example.com/p.png)\n"
	_, warnings, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning for an unsupported Markdown image")
	}
}

// Plain supported Markdown converts with no warnings.
func TestFromMarkdownLossy_PlainParagraphNoWarnings(t *testing.T) {
	doc, warnings, err := adf.FromMarkdownLossy("hello **world**\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("plain Markdown must not warn; got %+v", warnings)
	}
	if len(doc.Content) == 0 {
		t.Fatal("expected non-empty document")
	}
}

// Reference link definitions are parser metadata once goldmark resolves
// them onto the link node; dropping the definition line must not become
// a lossy conversion warning.
func TestFromMarkdownLossy_ReferenceLinkDefinitionDoesNotWarn(t *testing.T) {
	md := "[example][ref]\n\n[ref]: https://example.com \"title\"\n"
	doc, warnings, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("reference link definition must not warn; got %+v", warnings)
	}
	if len(doc.Content) != 1 || len(doc.Content[0].Content) != 1 {
		t.Fatalf("expected one paragraph with one text node; got %+v", doc.Content)
	}
	node := doc.Content[0].Content[0]
	if node.Text != "example" {
		t.Fatalf("text = %q, want %q", node.Text, "example")
	}
	if len(node.Marks) != 1 || node.Marks[0].Type != "link" {
		t.Fatalf("expected one link mark; got %+v", node.Marks)
	}
	if got := node.Marks[0].Attrs["href"]; got != "https://example.com" {
		t.Fatalf("href = %v, want %q", got, "https://example.com")
	}
}

// Headings, lists, code fences, and emphasis are all supported — no
// warnings for a document built only from those.
func TestFromMarkdownLossy_SupportedConstructsNoWarnings(t *testing.T) {
	md := "# Title\n\n" +
		"Some *emphasis* and **strong** and a [link](https://x).\n\n" +
		"- one\n- two\n\n" +
		"1. first\n2. second\n\n" +
		"```go\nfunc main() {}\n```\n"
	_, warnings, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("supported constructs must not warn; got %+v", warnings)
	}
}

// A blockquote currently has no authoring path in FromMarkdown — it
// must warn rather than silently drop the quoted content.
func TestFromMarkdownLossy_BlockquoteWarns(t *testing.T) {
	_, warnings, err := adf.FromMarkdownLossy("> quoted line\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning for an unsupported Markdown blockquote")
	}
}

// The warning message must name the unsupported construct so callers
// and agents can act on it.
func TestFromMarkdownLossy_WarningNamesConstruct(t *testing.T) {
	_, warnings, err := adf.FromMarkdownLossy("| a |\n|---|\n| 1 |\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning")
	}
	named := false
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w.Message), "table") {
			named = true
		}
	}
	if !named {
		t.Errorf("warning should name the 'table' construct; got %+v", warnings)
	}
}

// Hard line breaks inside a paragraph are representable as a hardBreak
// node — conversion must preserve them, not collapse the break.
func TestFromMarkdownLossy_HardBreakPreserved(t *testing.T) {
	md := "line one  \nline two\n"
	doc, _, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	found := false
	var walk func(n adf.Node)
	walk = func(n adf.Node) {
		if n.Type == "hardBreak" {
			found = true
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	for _, n := range doc.Content {
		walk(n)
	}
	if !found {
		t.Errorf("hard break should convert to a hardBreak node; doc=%+v", doc.Content)
	}
}
