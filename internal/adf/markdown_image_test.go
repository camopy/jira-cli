package adf_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// A Markdown image converts to its alt text carrying a link mark to the
// image URL — ADF cannot embed external images by URL, so the clickable
// reference is the faithful fallback.
func TestFromMarkdownImageBecomesAltTextLink(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("![screenshot](https://example.com/p.png)\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	node := doc.Content[0].Content[0]
	if node.Type != "text" || node.Text != "screenshot" {
		t.Fatalf("node = %+v, want text node carrying the alt text", node)
	}
	if len(node.Marks) != 1 || node.Marks[0].Type != "link" {
		t.Fatalf("marks = %+v, want a single link mark", node.Marks)
	}
	if href, _ := node.Marks[0].Attrs["href"].(string); href != "https://example.com/p.png" {
		t.Fatalf("link href = %q, want the image URL", href)
	}
}

// An image with an empty alt uses the URL as the display text.
func TestFromMarkdownImageEmptyAltFallsBackToURL(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("![](https://example.com/p.png)\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	node := doc.Content[0].Content[0]
	if node.Text != "https://example.com/p.png" {
		t.Fatalf("text = %q, want the URL fallback", node.Text)
	}
}

// Surrounding inline marks thread onto the degraded link, and the result
// passes strict validation.
func TestFromMarkdownImageInheritsMarksAndValidates(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("**![alt](https://example.com/p.png)**\n")
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	node := doc.Content[0].Content[0]
	types := make(map[string]bool, len(node.Marks))
	for _, m := range node.Marks {
		types[m.Type] = true
	}
	if !types["strong"] || !types["link"] {
		t.Fatalf("marks = %+v, want strong + link", node.Marks)
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Fatalf("strict validation rejected degraded image: %v", err)
	}
}
