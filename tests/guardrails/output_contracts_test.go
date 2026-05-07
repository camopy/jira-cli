// MOTIVATION: output-contract regressions are a recurring upstream
// class — empty lists exiting non-zero, --plain colliding with --raw,
// --debug leaking into stdout. Comments in this file are PROVENANCE
// ONLY and MUST NOT be a source of implementation, fixtures, wording,
// or test logic.
package guardrails

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

// Empty lists MUST be a successful exit-0 result with an empty issues
// array, not a failure or null body. We exercise the envelope layer
// directly — the CLI integration test at tests/contract/issue_list_*.go
// covers the end-to-end path.
func TestEmptyListEnvelopeIsExitZeroWithEmptyArray(t *testing.T) {
	env := cli.Envelope{
		Meta:     cli.Meta{Command: "issue.list", Profile: "p", Timestamp: "t"},
		Data:     map[string]any{"issues": []any{}, "count": 0},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	buf := &bytes.Buffer{}
	if err := cli.WriteEnvelope(buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("envelope not valid JSON: %v\n%s", err, buf.String())
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing or wrong type: %v", got["data"])
	}
	issues, _ := data["issues"].([]any)
	if issues == nil {
		t.Fatalf("issues serialized as null; should be empty array")
	}
	if len(issues) != 0 {
		t.Fatalf("issues should be empty, got %d", len(issues))
	}
	// Errors and warnings MUST also be empty arrays, never null.
	if got["errors"] == nil {
		t.Fatalf("errors serialized as null; should be empty array")
	}
	if got["warnings"] == nil {
		t.Fatalf("warnings serialized as null; should be empty array")
	}
}

// The JSON envelope shape MUST stay consistent across all command
// names. The required keys are {meta, data, errors, warnings};
// nothing else is implied.
func TestEnvelopeShapeIsStable(t *testing.T) {
	env := cli.Envelope{
		Meta:     cli.Meta{Command: "x", Profile: "p", Timestamp: "t"},
		Data:     map[string]any{"k": "v"},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	buf := &bytes.Buffer{}
	if err := cli.WriteEnvelope(buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	for _, key := range []string{"meta", "data", "errors", "warnings"} {
		if _, has := got[key]; !has {
			t.Fatalf("envelope missing required key %q", key)
		}
	}
}
