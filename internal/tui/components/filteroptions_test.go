package components

import (
	"strings"
	"testing"
)

// The filter overlay marks the active row with the ❯ glyph, not a plain ">".
func TestFilterOptionsCursorGlyph(t *testing.T) {
	f := NewFilterOptions().ShowWithRows("issues", []FilterRow{
		{Label: "Status", Choices: []string{"All", "Open"}, Current: 0},
		{Label: "Assignee", Choices: []string{"Anyone", "Me"}, Current: 0},
	})
	content := f.View().Content
	if !strings.Contains(content, "❯ ") {
		t.Fatalf("filter overlay missing ❯ cursor glyph:\n%s", content)
	}
	if strings.Contains(content, "> ") {
		t.Fatalf("filter overlay still uses the plain \"> \" marker:\n%s", content)
	}
}
