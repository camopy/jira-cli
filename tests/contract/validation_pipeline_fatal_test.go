package contract

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// Per-stage fatal semantics for strict + best-effort.
//
// Stage 1 (parse): fatal in BOTH modes when required missing, malformed
//
//	JSON, mutex flags, stdin discipline violation.
//
// Stage 2 (ADF):   fatal in strict for invalid ADF; in best-effort,
//
//	fatal only if no valid ADF can be produced at all.
//
// Stage 3 (field): fatal in strict for any invalid field; in best-effort,
//
//	only when required fields cannot be satisfied.
//
// Stage 4 (custom): fatal when value cannot be encoded safely; best-
//
//	effort drops unsupported optional fields with
//	warnings; malformed values fatal in both modes.
//
// Stage 5 (submit): dry-run never submits; live submit failures are
//
//	per the existing error contract.
func TestPipelineStageFatalSemantics(t *testing.T) {
	t.Run("stage 1 fatal in both modes", func(t *testing.T) {
		for _, mode := range []adfmode.Mode{adfmode.ModeStrict, adfmode.ModeBestEffort} {
			out := pipeline.Run(pipeline.Input{Mode: mode, ParseError: "required field missing"})
			if !out.Aborted || out.AbortedAt != pipeline.StageParse {
				t.Fatalf("mode=%v: stage 1 must always be fatal; got %+v", mode, out)
			}
		}
	})

	t.Run("stage 2 fatal only in strict on invalid ADF", func(t *testing.T) {
		strict := pipeline.Run(pipeline.Input{Mode: adfmode.ModeStrict, ADFInvalid: true, FieldOnScreen: true, CustomEncodeOK: true})
		if !strict.Aborted || strict.AbortedAt != pipeline.StageADF {
			t.Fatalf("strict invalid-ADF must abort at stage 2; got %+v", strict)
		}
		best := pipeline.Run(pipeline.Input{Mode: adfmode.ModeBestEffort, ADFInvalid: true, ADFWarning: "degraded", FieldOnScreen: true, CustomEncodeOK: true})
		if best.Aborted {
			t.Fatalf("best-effort with degradable ADF must NOT abort; got %+v", best)
		}
		if len(best.Warnings) == 0 {
			t.Fatal("best-effort ADF degradation must produce at least one warning")
		}
	})

	t.Run("stage 3 fatal only in strict on invalid field", func(t *testing.T) {
		strict := pipeline.Run(pipeline.Input{Mode: adfmode.ModeStrict, FieldOnScreen: false, CustomEncodeOK: true})
		if !strict.Aborted || strict.AbortedAt != pipeline.StageFieldSchema {
			t.Fatalf("strict invalid-field must abort at stage 3; got %+v", strict)
		}
		best := pipeline.Run(pipeline.Input{Mode: adfmode.ModeBestEffort, FieldOnScreen: false, CustomEncodeOK: true})
		if best.Aborted {
			t.Fatalf("best-effort invalid-field must NOT abort, drops field with warning; got %+v", best)
		}
		if len(best.Warnings) == 0 {
			t.Fatal("best-effort field drop must produce at least one warning")
		}
	})

	t.Run("stage 4 fatal in both modes on encoding failure", func(t *testing.T) {
		for _, mode := range []adfmode.Mode{adfmode.ModeStrict, adfmode.ModeBestEffort} {
			out := pipeline.Run(pipeline.Input{Mode: mode, CustomEncodeOK: false, FieldOnScreen: true})
			if !out.Aborted || out.AbortedAt != pipeline.StageCustomField {
				t.Fatalf("mode=%v: stage 4 encoding failure must always be fatal; got %+v", mode, out)
			}
		}
	})

	t.Run("stage 5 dry-run never submits but runs stages 1-4", func(t *testing.T) {
		out := pipeline.Run(pipeline.Input{Mode: adfmode.ModeStrict, FieldOnScreen: true, CustomEncodeOK: true, DryRun: true})
		if out.Aborted {
			t.Fatalf("clean dry-run must succeed; got %+v", out)
		}
		if out.Submitted {
			t.Fatal("dry-run MUST NOT submit (stage 5)")
		}
		if !out.PreviewReady {
			t.Fatal("dry-run MUST report preview-ready")
		}
	})
}
