package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/pkg/adf"
)

func TestADFValidation(t *testing.T) {
	doc := adf.Document{
		Type:    "doc",
		Version: 1,
		Content: []adf.Node{{
			Type: "paragraph",
			Content: []adf.Node{{
				Type: "text",
				Text: "hello",
				Marks: []adf.Mark{{
					Type: "strong",
				}},
			}},
		}},
	}
	if _, err := adf.ValidateDoc(doc, adfmode.ModeStrict); err != nil {
		t.Fatalf("ValidateDoc() error = %v", err)
	}

	bad := adf.Document{Type: "paragraph", Version: 1}
	if _, err := adf.ValidateDoc(bad, adfmode.ModeStrict); err == nil {
		t.Fatal("ValidateDoc() error = nil for invalid root")
	}
}
