package adfcmd

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// Guardrail: the standalone converter must be the same pipeline a mutation
// runs — FromMarkdownLossy → Normalize → ValidateDoc — so a clean convert
// here can never disagree with a submit.
func TestConvertMarkdownMatchesPipelineConverter(t *testing.T) {
	const md = "## Title\n\nSome **bold** text.\n\n```go\nfunc main() {}\n```\n\n| a |\n|---|\n| 1 |\n"

	got, warnings, err := convertMarkdown(md, adfmode.ModeStrict)
	if err != nil {
		t.Fatalf("convertMarkdown: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("clean input produced warnings: %+v", warnings)
	}

	want, _, err := adf.FromMarkdownLossy(md)
	if err != nil {
		t.Fatalf("FromMarkdownLossy: %v", err)
	}
	wantNormalized, _ := adf.Normalize(want)
	if !reflect.DeepEqual(got, wantNormalized) {
		t.Fatal("convertMarkdown output diverged from the pipeline conversion sequence")
	}
}

// Strict mode aborts on a lossy step with the typed source-mapped error —
// mutation parity.
func TestConvertMarkdownStrictAbortsOnLossyStep(t *testing.T) {
	_, _, err := convertMarkdown("<div>\nraw\n</div>\n", adfmode.ModeStrict)
	var lossy adf.LossyConversionError
	if !errors.As(err, &lossy) {
		t.Fatalf("err = %v, want adf.LossyConversionError", err)
	}
	if !strings.Contains(lossy.Error(), "line 1") {
		t.Errorf("error = %q, want source-mapped position", lossy.Error())
	}
}

// Best-effort emits the sanitized document plus the lossy warning.
func TestConvertMarkdownBestEffortEmitsDocumentAndWarnings(t *testing.T) {
	doc, warnings, err := convertMarkdown("intro\n\n<div>\nraw\n</div>\n", adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("convertMarkdown: %v", err)
	}
	if len(doc.Content) == 0 {
		t.Fatal("best-effort must still emit the document")
	}
	lossyFound := false
	for _, w := range warnings {
		if w.Lossy {
			lossyFound = true
		}
	}
	if !lossyFound {
		t.Fatalf("warnings = %+v, want the lossy mark-conflict entry", warnings)
	}
}
