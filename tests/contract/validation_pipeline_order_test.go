package contract

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// The validation pipeline MUST run in this exact order:
//  1. parse / shape
//  2. ADF + compatibility
//  3. field schema / screen
//  4. customfield registry
//  5. dry-run preview / live submit
//
// We exercise the order by injecting failures at each stage and
// asserting the result tells us which stage aborted.
func TestPipelineRunsStagesInDeterministicOrder(t *testing.T) {
	cases := []struct {
		name      string
		input     pipeline.Input
		wantStage pipeline.Stage
	}{
		{
			name:      "stage 1 fatal — malformed flag",
			input:     pipeline.Input{ParseError: "malformed flag --foo"},
			wantStage: pipeline.StageParse,
		},
		{
			name: "stage 2 fatal — invalid ADF in strict",
			input: pipeline.Input{
				Mode:           adfmode.ModeStrict,
				ADFInvalid:     true,
				FieldOnScreen:  true,
				CustomEncodeOK: true,
			},
			wantStage: pipeline.StageADF,
		},
		{
			name: "stage 3 fatal — field not on screen in strict",
			input: pipeline.Input{
				Mode:           adfmode.ModeStrict,
				FieldOnScreen:  false,
				CustomEncodeOK: true,
			},
			wantStage: pipeline.StageFieldSchema,
		},
		{
			name: "stage 4 fatal — customfield encode failure",
			input: pipeline.Input{
				Mode:           adfmode.ModeStrict,
				FieldOnScreen:  true,
				CustomEncodeOK: false,
			},
			wantStage: pipeline.StageCustomField,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := pipeline.Run(tc.input)
			if !out.Aborted {
				t.Fatalf("pipeline should have aborted; got result=%+v", out)
			}
			if out.AbortedAt != tc.wantStage {
				t.Fatalf("aborted at stage %v, want %v", out.AbortedAt, tc.wantStage)
			}
		})
	}
}

// Warnings collected at earlier stages MUST survive a later-stage
// fatal. The full envelope tells the user about every observation.
func TestPipelineWarningsSurviveLaterStageFatal(t *testing.T) {
	in := pipeline.Input{
		Mode:           adfmode.ModeStrict,
		ADFWarning:     "inlineCard degraded",
		FieldOnScreen:  false, // stage 3 will fatal
		CustomEncodeOK: true,
	}
	out := pipeline.Run(in)
	if !out.Aborted || out.AbortedAt != pipeline.StageFieldSchema {
		t.Fatalf("expected stage-3 fatal, got %+v", out)
	}
	if len(out.Warnings) != 1 || out.Warnings[0].Message != "inlineCard degraded" {
		t.Fatalf("stage-2 warning was not preserved through stage-3 fatal: %v", out.Warnings)
	}
}
