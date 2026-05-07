package contract

import (
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// Create/edit field validation MUST behave per mode:
//
//	strict      — abort whole submission with field/id/project/issue_type/
//	               operation/reason on the first invalid field
//	best-effort — drop invalid fields with one warning per drop
//	               (type=field_not_on_screen, lossy=true) and submit the
//	               remainder
func TestFieldValidationStrictAbortsOnInvalidField(t *testing.T) {
	schema := pipeline.ScreenSchema{
		Project:     "KAN",
		IssueType:   "Story",
		ValidFields: map[string]bool{"summary": true, "description": true, "labels": true},
	}
	in := map[string]any{
		"summary":     "valid",
		"description": "valid",
		"epic_link":   "EPIC-1", // not on screen
	}

	_, _, err := pipeline.ValidateFields(in, schema, adfmode.ModeStrict)
	if err == nil {
		t.Fatal("strict mode must abort on invalid field; got nil")
	}
	pe := &pipeline.FieldValidationError{}
	ok := errors.As(err, &pe)
	if !ok {
		t.Fatalf("expected *pipeline.FieldValidationError, got %T: %v", err, err)
	}
	if pe.Field != "epic_link" {
		t.Fatalf("error.field = %q, want epic_link", pe.Field)
	}
	if pe.Project != "KAN" || pe.IssueType != "Story" {
		t.Fatalf("error missing project/issue_type context: %+v", pe)
	}
	if pe.Reason == "" {
		t.Fatal("error.reason missing")
	}
}

func TestFieldValidationBestEffortDropsInvalidFields(t *testing.T) {
	schema := pipeline.ScreenSchema{
		Project:     "KAN",
		IssueType:   "Story",
		ValidFields: map[string]bool{"summary": true, "description": true},
	}
	in := map[string]any{
		"summary":     "valid",
		"description": "valid",
		"epic_link":   "EPIC-1",
		"sprint_id":   42,
	}

	out, warnings, err := pipeline.ValidateFields(in, schema, adfmode.ModeBestEffort)
	if err != nil {
		t.Fatalf("best-effort must not abort, got %v", err)
	}
	if _, present := out["epic_link"]; present {
		t.Fatalf("epic_link should have been dropped, got %v", out["epic_link"])
	}
	if _, present := out["sprint_id"]; present {
		t.Fatalf("sprint_id should have been dropped, got %v", out["sprint_id"])
	}
	if out["summary"] != "valid" {
		t.Fatalf("valid field summary missing or mutated: %v", out["summary"])
	}
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings (epic_link + sprint_id), got %d: %v", len(warnings), warnings)
	}
	for _, w := range warnings {
		if w.Type != "field_not_on_screen" {
			t.Errorf("warning type = %q, want field_not_on_screen", w.Type)
		}
		if !w.Lossy {
			t.Errorf("warning must be lossy=true")
		}
	}
}
