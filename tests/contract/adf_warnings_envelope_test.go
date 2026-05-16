package contract

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

// JSON envelope MUST carry a top-level `warnings: []` array parallel
// to `errors`. Both arrays MUST serialize as `[]` (not `null`) when
// empty so consumers can `len(warnings)` without nil checks.
func TestEnvelopeAlwaysCarriesEmptyWarningsArray(t *testing.T) {
	env := cli.Envelope{
		Meta: cli.Meta{
			Command:   "test.cmd",
			Timestamp: "2026-05-04T00:00:00Z",
		},
		Data:     map[string]any{"k": "v"},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	buf := &bytes.Buffer{}
	if err := cli.WriteEnvelope(buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	w, ok := parsed["warnings"]
	if !ok {
		t.Fatalf("envelope missing warnings key.\noutput: %s", buf.String())
	}
	if w == nil {
		t.Fatalf("warnings serialized as null; should be empty array")
	}
	wa, ok := w.([]any)
	if !ok {
		t.Fatalf("warnings not an array, got %T", w)
	}
	if len(wa) != 0 {
		t.Fatalf("expected empty warnings array, got %v", wa)
	}
}

// Each warning entry MUST include type, message, and lossy. Optional
// keys: field, path, node_type, mark_type. The shape MUST survive
// round-trip JSON marshal.
func TestWarningShape(t *testing.T) {
	w := cli.Warning{
		Type:     "adf_compatibility",
		Message:  "inlineCard not supported on description; degraded to link",
		Field:    "description",
		Path:     "/content/0/content/2",
		NodeType: "inlineCard",
		Lossy:    true,
	}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"type", "message", "field", "path", "node_type", "lossy"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("warning missing required key %q. got: %s", key, b)
		}
	}
	if got["lossy"] != true {
		t.Fatalf("lossy not preserved as bool: %v", got["lossy"])
	}
	if _, ok := got["mark_type"]; ok {
		t.Fatalf("empty mark_type should be omitted, got: %v", got["mark_type"])
	}
}

// lossy MUST always be present (even when false), because consumers
// branch on it; omitempty would silently coerce false → missing → ambiguous.
func TestWarningLossyAlwaysSerialised(t *testing.T) {
	w := cli.Warning{Type: "informational", Message: "nothing was lost"}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := got["lossy"]
	if !ok {
		t.Fatalf("lossy must be serialized even when false. got: %s", b)
	}
	if v != false {
		t.Fatalf("lossy=%v want false", v)
	}
}

// Optional fields (field, path, node_type, mark_type) MUST be omitted
// when empty so the JSON stays tight for cheap-to-parse agent
// consumption.
func TestWarningOmitsEmptyOptionalFields(t *testing.T) {
	w := cli.Warning{Type: "x", Message: "y", Lossy: true}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, omitted := range []string{"field", "path", "node_type", "mark_type"} {
		if _, has := got[omitted]; has {
			t.Fatalf("optional %q should be omitted when empty. got: %s", omitted, b)
		}
	}
}
