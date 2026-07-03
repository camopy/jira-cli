package cli_test

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli"
)

// The strict-mode Markdown→ADF abort must map to a validation error with a
// stable code and a populated hint — the original defect shipped the raw
// warning message with an empty hint.
func TestMapErrorLossyConversionCarriesHint(t *testing.T) {
	err := adf.LossyConversionError{Warning: adf.Warning{
		Type:    "markdown_lossy_conversion",
		Message: `Markdown blockquote dropped (line 3, col 1: "> quoted")`,
		Lossy:   true,
	}}
	mapped := cli.MapError(err)
	if mapped.Type != "validation" {
		t.Errorf("type = %q, want validation", mapped.Type)
	}
	if mapped.Code != "markdown_lossy_conversion" {
		t.Errorf("code = %q, want markdown_lossy_conversion", mapped.Code)
	}
	if mapped.Hint == "" {
		t.Error("hint is empty — every markdown conversion diagnostic must carry remediation")
	}
	if !strings.Contains(mapped.Message, "line 3") {
		t.Errorf("message = %q, want the source-mapped warning text", mapped.Message)
	}
}
