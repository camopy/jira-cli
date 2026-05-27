package pipeline_test

import (
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// FlattenCustomFields lifts a contract-level `custom_fields` sub-map into
// top-level customfield_NNNNN keys so screen validation and encoding see
// one flat namespace. Raw customfield_NNNNN keys already at the top level
// are left alone.
func TestFlattenCustomFieldsLiftsNestedMap(t *testing.T) {
	in := map[string]any{
		"summary": "ok",
		"custom_fields": map[string]any{
			"customfield_10001": float64(5),
			"customfield_10002": "Option A",
		},
		"customfield_10003": "already flat",
	}
	out, err := pipeline.FlattenCustomFields(in)
	if err != nil {
		t.Fatalf("flatten error = %v", err)
	}
	if _, has := out["custom_fields"]; has {
		t.Fatalf("custom_fields wrapper must be removed after flatten: %v", out)
	}
	if out["customfield_10001"] != float64(5) {
		t.Fatalf("customfield_10001 not lifted: %v", out["customfield_10001"])
	}
	if out["customfield_10002"] != "Option A" {
		t.Fatalf("customfield_10002 not lifted: %v", out["customfield_10002"])
	}
	if out["customfield_10003"] != "already flat" {
		t.Fatalf("raw customfield_10003 must survive flatten: %v", out["customfield_10003"])
	}
	if out["summary"] != "ok" {
		t.Fatalf("native field mutated by flatten: %v", out["summary"])
	}
}

// A custom_fields wrapper whose value is not an object is fatal — the
// user supplied input the CLI cannot route, and dropping it would lose
// data silently.
func TestFlattenCustomFieldsRejectsNonObjectWrapper(t *testing.T) {
	_, err := pipeline.FlattenCustomFields(map[string]any{
		"custom_fields": "not an object",
	})
	if err == nil {
		t.Fatal("non-object custom_fields wrapper must be fatal")
	}
}

// A custom_fields entry that collides with a top-level customfield key is
// fatal: the CLI cannot silently pick a winner.
func TestFlattenCustomFieldsRejectsCollision(t *testing.T) {
	_, err := pipeline.FlattenCustomFields(map[string]any{
		"customfield_10001": "top-level",
		"custom_fields": map[string]any{
			"customfield_10001": "nested",
		},
	})
	if err == nil {
		t.Fatal("colliding customfield key must be fatal")
	}
}

// RunMutation MUST flatten custom_fields before stage 3 so a nested
// customfield value is screen-validated like any flat one. In strict
// mode a nested customfield off the active screen aborts at stage 3.
func TestRunMutationFlattensCustomFieldsBeforeScreenValidation(t *testing.T) {
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode: adfmode.ModeStrict,
		Fields: map[string]any{
			"summary": "ok",
			"custom_fields": map[string]any{
				"customfield_77777": "off screen",
			},
		},
		Schema: pipeline.ScreenSchema{
			Project:     "KAN",
			IssueType:   "Story",
			ValidFields: map[string]bool{"summary": true},
		},
	})
	if !out.Aborted || out.AbortedAt != pipeline.StageFieldSchema {
		t.Fatalf("nested customfield off screen must abort at stage 3, got %+v", out)
	}
	var fve *pipeline.FieldValidationError
	if !errors.As(out.Err, &fve) {
		t.Fatalf("expected FieldValidationError, got %T", out.Err)
	}
	if fve.Field != "customfield_77777" {
		t.Fatalf("wrong field reported: %q", fve.Field)
	}
}

func TestRunMutationExemptsSelectedFieldsFromScreenValidation(t *testing.T) {
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode: adfmode.ModeStrict,
		Fields: map[string]any{
			"project":   map[string]any{"key": "KAN"},
			"issuetype": map[string]any{"name": "Task"},
			"summary":   "move me",
		},
		ScreenValidationExemptFields: map[string]bool{
			"project":   true,
			"issuetype": true,
		},
		Schema: pipeline.ScreenSchema{
			Project:     "JCT",
			IssueType:   "Task",
			ValidFields: map[string]bool{"summary": true},
		},
		DryRun: true,
	})
	if out.Aborted {
		t.Fatalf("exempt project/issuetype fields should bypass screen validation, got %+v", out)
	}
	if out.SubmitFields["project"] == nil || out.SubmitFields["issuetype"] == nil || out.SubmitFields["summary"] != "move me" {
		t.Fatalf("exempt and validated fields should all be submitted: %+v", out.SubmitFields)
	}
}

func TestRunMutationDoesNotExemptCustomFieldsFromScreenValidation(t *testing.T) {
	out := pipeline.RunMutation(pipeline.MutationInput{
		Mode: adfmode.ModeStrict,
		Fields: map[string]any{
			"customfield_77777": "off screen",
		},
		ScreenValidationExemptFields: map[string]bool{
			"customfield_77777": true,
		},
		Schema: pipeline.ScreenSchema{
			Project:     "JCT",
			IssueType:   "Task",
			ValidFields: map[string]bool{"summary": true},
		},
		DryRun: true,
	})
	if !out.Aborted || out.AbortedAt != pipeline.StageFieldSchema {
		t.Fatalf("customfield exemption should not bypass screen validation, got %+v", out)
	}
	var fve *pipeline.FieldValidationError
	if !errors.As(out.Err, &fve) || fve.Field != "customfield_77777" {
		t.Fatalf("expected customfield_77777 field validation error, got %T %[1]v", out.Err)
	}
}

// When the screen schema declares a customfield's type, the encoder
// validates and encodes the value per that type — not by guessing from
// the registry's type-name keys. A select customfield typed by the
// schema accepts both an explicit {"value":...} object and a bare
// label, and rejects a genuinely malformed value fatally.
func TestRunMutationEncodesCustomFieldByScreenSchemaType(t *testing.T) {
	schema := pipeline.ScreenSchema{
		Project:     "KAN",
		IssueType:   "Story",
		ValidFields: map[string]bool{"summary": true, "customfield_20001": true},
		FieldTypes:  map[string]string{"customfield_20001": "select"},
	}

	// A genuinely malformed select value (a number, neither a label
	// string nor an object) → fatal in strict mode.
	bad := pipeline.RunMutation(pipeline.MutationInput{
		Mode:   adfmode.ModeStrict,
		Fields: map[string]any{"summary": "ok", "customfield_20001": float64(42)},
		Schema: schema,
	})
	if !bad.Aborted || bad.AbortedAt != pipeline.StageCustomField {
		t.Fatalf("malformed typed customfield must abort at stage 4, got %+v", bad)
	}

	// Well-formed select value → passes.
	good := pipeline.RunMutation(pipeline.MutationInput{
		Mode: adfmode.ModeStrict,
		Fields: map[string]any{
			"summary":           "ok",
			"customfield_20001": map[string]any{"value": "Option A"},
		},
		Schema: schema,
		DryRun: true,
	})
	if good.Aborted {
		t.Fatalf("well-formed typed customfield must pass, got %+v", good)
	}
}
