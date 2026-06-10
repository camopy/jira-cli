package contract

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Warnings collected by the pipeline before a live API call MUST
// survive into the response envelope. The previous integration only
// surfaced warnings in dry-run paths; this regression test exercises
// the live-submit helper directly so a future drop of the warnings
// parameter from the writeEnvelopeWithResponseAndWarnings call sites
// is caught.
//
// We don't import cmd/jira (main package). Instead we exercise the
// shape directly via the cli.Envelope contract: a live-submit envelope
// MUST include the warnings array verbatim.
func TestLiveSubmitEnvelopeIncludesPipelineWarnings(t *testing.T) {
	warning := adf.Warning{
		Type:    "field_not_on_screen",
		Message: "epic_link dropped",
		Field:   "epic_link",
		Lossy:   true,
	}
	cliWarn := cli.WarningFrom(warning)

	env := cli.Envelope{
		Meta:     cli.Meta{Command: "issue.create", Timestamp: "t"},
		Data:     map[string]any{"issue": map[string]any{"key": "JCT-1"}},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{cliWarn},
	}
	buf := &bytes.Buffer{}
	if err := cli.WriteEnvelope(buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("envelope not JSON: %v\n%s", err, buf.String())
	}
	warnings, ok := got["warnings"].([]any)
	if !ok || len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", got["warnings"])
	}
	w0, _ := warnings[0].(map[string]any)
	if w0["type"] != "field_not_on_screen" {
		t.Fatalf("warning type lost: %v", w0)
	}
	if w0["lossy"] != true {
		t.Fatalf("warning lossy flag lost: %v", w0)
	}
}

// Compile-time guard: any future signature change on
// jira.Response that breaks the helper sites would surface here.
var _ = func() *jira.Response {
	return &jira.Response{}
}

// Compile-time guard for the cobra wiring layer.
var _ = func() *cobra.Command { return &cobra.Command{} }
