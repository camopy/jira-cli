package cli

import (
	"bytes"
	"strings"
	"testing"
)

// A dry-run preview must speak in the conditional mood: the past-tense
// completion verb ("Transitioned issue") would claim a mutation that never
// left the machine. The dry_run field in the payload selects the form, on
// both the single-payload and shared multi-key render paths.
func TestDryRunPreviewsUseConditionalMood(t *testing.T) {
	t.Run("single payload", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteCommandPlain(&buf, "issue.transition", map[string]any{
			"issue":   "JCT-1",
			"dry_run": true,
		})
		if err != nil {
			t.Fatalf("WriteCommandPlain: %v", err)
		}
		got := buf.String()
		if !strings.Contains(got, "Would transition issue") {
			t.Fatalf("dry-run preview should use the conditional mood, got:\n%s", got)
		}
		if strings.Contains(got, "Transitioned issue") {
			t.Fatalf("dry-run preview must not claim past-tense completion:\n%s", got)
		}
	})

	t.Run("multi-key envelope", func(t *testing.T) {
		// Mirrors cmdutil.KeyedResultsData: the production multi-key
		// envelope is a typed struct (json tags below match), not a
		// map[string]any — a hand-built map would silently miss the
		// mapFromAny conversion boundary dataDryRun must cross.
		type keyedResult struct {
			Key  string `json:"key"`
			OK   bool   `json:"ok"`
			Data any    `json:"data,omitempty"`
		}
		envelope := struct {
			Results []keyedResult `json:"results"`
			Total   int           `json:"total"`
		}{
			Results: []keyedResult{
				{Key: "JCT-1", OK: true, Data: map[string]any{"issue": "JCT-1", "dry_run": true}},
				{Key: "JCT-2", OK: true, Data: map[string]any{"issue": "JCT-2", "dry_run": true}},
			},
			Total: 2,
		}
		var buf bytes.Buffer
		err := WriteCommandPlain(&buf, "issue.transition", envelope)
		if err != nil {
			t.Fatalf("WriteCommandPlain: %v", err)
		}
		got := buf.String()
		if !strings.Contains(got, "Would transition issue") {
			t.Fatalf("multi-key dry-run preview should use the conditional mood, got:\n%s", got)
		}
		if strings.Contains(got, "Transitioned issue") {
			t.Fatalf("multi-key dry-run preview must not claim past-tense completion:\n%s", got)
		}
	})

	t.Run("live run keeps past tense", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteCommandPlain(&buf, "issue.transition", map[string]any{
			"issue":   "JCT-1",
			"dry_run": false,
		})
		if err != nil {
			t.Fatalf("WriteCommandPlain: %v", err)
		}
		if got := buf.String(); !strings.Contains(got, "Transitioned issue") {
			t.Fatalf("live run should keep the past-tense confirmation, got:\n%s", got)
		}
	})
}
