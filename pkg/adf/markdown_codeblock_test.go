package adf_test

import (
	"testing"

	"github.com/matcra587/jira-cli/pkg/adf"
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

// Bare (non-fenced) code blocks have no language hint — attrs.language
// MUST be absent (not present-but-empty) so downstream renderers can
// distinguish "no language" from "language=”".
func TestFromMarkdownIndentedCodeHasNoLanguage(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("    indented code\n    second line\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	for _, n := range doc.Content {
		if n.Type == "codeBlock" {
			if _, has := n.Attrs["language"]; has {
				t.Fatalf("indented code block must NOT carry attrs.language; got %v", n.Attrs)
			}
		}
	}
}
