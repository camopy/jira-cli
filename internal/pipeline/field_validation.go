// Package pipeline implements the deterministic 5-stage validation
// pipeline that gates every Jira mutation submission. Stages run in
// fixed order; warnings collected by earlier stages survive a
// later-stage fatal so the envelope always carries the full context.
//
// Stages:
//
//  1. Parse / shape
//  2. ADF + compatibility (pkg/adf calls)
//  3. Field schema / screen validation (this file)
//  4. Customfield registry (pkg/jira/customfield)
//  5. Dry-run preview / live submit
//
// Strict aborts at the first FATAL stage. Best-effort drops or coerces
// where defined and continues.
package pipeline

import (
	"fmt"
	"sort"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// ScreenSchema is the per-project / per-issue-type field whitelist.
// Fields not in ValidFields are "not on screen" and trigger a
// strict abort or best-effort drop.
type ScreenSchema struct {
	Project     string
	IssueType   string
	ValidFields map[string]bool
}

// FieldValidationError is the typed error returned by ValidateFields in
// strict mode. Carries the required context: field name, project,
// issue type, operation, reason.
type FieldValidationError struct {
	Field     string
	Project   string
	IssueType string
	Operation string
	Reason    string
}

func (e *FieldValidationError) Error() string {
	if e == nil {
		return "<nil field-validation error>"
	}
	return fmt.Sprintf("field validation: %s/%s op=%s field=%s: %s",
		e.Project, e.IssueType, e.Operation, e.Field, e.Reason)
}

// ValidateFields runs Stage 3 of the mutation pipeline. In strict mode
// it aborts on the first invalid field; in best-effort it returns a
// copy with invalid fields stripped and one warning per drop.
func ValidateFields(fields map[string]any, schema ScreenSchema, mode adfmode.Mode) (map[string]any, []adf.Warning, error) {
	// Iterate keys in sorted order so strict mode reports a deterministic
	// "first invalid field" — important for test reproducibility.
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]any, len(fields))
	var warnings []adf.Warning
	for _, k := range keys {
		if schema.ValidFields[k] {
			out[k] = fields[k]
			continue
		}
		if mode == adfmode.ModeStrict {
			return nil, nil, &FieldValidationError{
				Field:     k,
				Project:   schema.Project,
				IssueType: schema.IssueType,
				Operation: "create-or-edit",
				Reason:    "field not on the screen for this project / issue type",
			}
		}
		warnings = append(warnings, adf.Warning{
			Type:    "field_not_on_screen",
			Message: fmt.Sprintf("field %q not on screen for %s/%s; dropped", k, schema.Project, schema.IssueType),
			Field:   k,
			Lossy:   true,
		})
	}
	return out, warnings, nil
}
