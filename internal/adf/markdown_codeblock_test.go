package adf_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// Markdown-input layer: a fenced ```lang block in Markdown MUST
// produce an ADF codeBlock with attrs.language=lang. The hint is what
// Jira (and downstream renderers) use to syntax-highlight; dropping it
// silently turns a Go snippet into anonymous text.
func TestFromMarkdownPreservesFencedCodeLanguage(t *testing.T) {
	cases := map[string]string{
		"```go\nfunc main() {}\n```":       "go",
		"```python\nprint('hi')\n```":      "python",
		"```rust\nfn main() {}\n```":       "rust",
		"```typescript\nconst x = 1;\n```": "typescript",
	}
	for md, want := range cases {
		t.Run(want, func(t *testing.T) {
			doc, _, err := adf.FromMarkdownLossy(md)
			if err != nil {
				t.Fatalf("FromMarkdownLossy: %v", err)
			}
			var got string
			for _, n := range doc.Content {
				if n.Type == "codeBlock" {
					if v, ok := n.Attrs["language"].(string); ok {
						got = v
					}
				}
			}
			if got != want {
				t.Fatalf("language for fenced ```%s = %q, want %q", want, got, want)
			}
		})
	}
}

// Unhinted code blocks (indented, or fenced without a language) MUST carry
// attrs.language as an explicit empty string: Jira renders an attr-less
// codeBlock with its default language (java), so "no attr" silently
// java-highlights arbitrary output.
func TestFromMarkdownUnhintedCodeCarriesEmptyLanguage(t *testing.T) {
	for name, md := range map[string]string{
		"indented":       "    indented code\n    second line\n",
		"fenced-no-hint": "```\nplain output\n```",
	} {
		t.Run(name, func(t *testing.T) {
			doc, _, err := adf.FromMarkdownLossy(md)
			if err != nil {
				t.Fatalf("FromMarkdownLossy: %v", err)
			}
			block := findCodeBlock(t, doc)
			lang, has := block.Attrs["language"]
			if !has {
				t.Fatalf("code block must carry attrs.language (Jira defaults attr-less blocks to java); got %v", block.Attrs)
			}
			if lang != "" {
				t.Fatalf("attrs.language = %q, want empty string for an unhinted block", lang)
			}
		})
	}
}

// An empty fenced block must produce a codeBlock with NO content array —
// the ADF schema requires text nodes to be non-empty, so an empty text
// child is a validation rejection waiting to happen.
func TestFromMarkdownEmptyCodeBlockHasNoTextChild(t *testing.T) {
	for name, md := range map[string]string{
		"empty":        "```\n```",
		"newline-only": "```go\n\n```",
	} {
		t.Run(name, func(t *testing.T) {
			doc, _, err := adf.FromMarkdownLossy(md)
			if err != nil {
				t.Fatalf("FromMarkdownLossy: %v", err)
			}
			block := findCodeBlock(t, doc)
			if len(block.Content) != 0 {
				t.Fatalf("empty code block must have no content; got %+v", block.Content)
			}
		})
	}
}

// The fence's trailing newline is Markdown syntax, not code: the text child
// must not carry it, or Jira renders a spurious blank line.
func TestFromMarkdownCodeBlockTrimsTrailingNewline(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("```go\nfunc main() {}\n```")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	block := findCodeBlock(t, doc)
	if len(block.Content) != 1 {
		t.Fatalf("code block content = %+v, want one text child", block.Content)
	}
	if got, want := block.Content[0].Text, "func main() {}"; got != want {
		t.Fatalf("code block text = %q, want %q", got, want)
	}
}

// Every shape this converter emits for code blocks must pass strict ADF
// validation — the whole point of the fixes is surviving Jira's schema.
func TestFromMarkdownCodeBlocksPassStrictValidation(t *testing.T) {
	for name, md := range map[string]string{
		"hinted":   "```go\nfunc main() {}\n```",
		"unhinted": "```\nplain\n```",
		"empty":    "```\n```",
		"indented": "    indented code\n",
	} {
		t.Run(name, func(t *testing.T) {
			doc, _, err := adf.FromMarkdownLossy(md)
			if err != nil {
				t.Fatalf("FromMarkdownLossy: %v", err)
			}
			if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
				t.Fatalf("strict validation rejected converter output: %v", err)
			}
		})
	}
}

func findCodeBlock(t *testing.T, doc adf.Document) adf.Node {
	t.Helper()
	for _, n := range doc.Content {
		if n.Type == "codeBlock" {
			return n
		}
	}
	t.Fatal("no codeBlock node in converted document")
	return adf.Node{}
}
