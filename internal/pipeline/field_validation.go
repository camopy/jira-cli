package pipeline

import (
	"fmt"

	xmaps "github.com/gechr/x/maps"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// ScreenSchema is the per-project / per-issue-type field whitelist.
// Fields not in ValidFields are "not on screen" and trigger a
// strict abort or best-effort drop.
//
// FieldTypes maps a field id (customfield_NNNNN or a system field id)
// to the schema.custom token Jira reports for it (e.g. "select",
// "multiselect", "datepicker"). Stage 4 uses it to encode each custom
// field per its declared type instead of guessing from registry
// type-name keys. A field absent from FieldTypes has no declared type
// and is forwarded opaquely.
type ScreenSchema struct {
	Project     string
	IssueType   string
	ValidFields map[string]bool
	FieldTypes  map[string]string
}

// customFieldsKey is the contract-level wrapper key that carries
// per-field custom-field values nested one level below the fields map.
const customFieldsKey = "custom_fields"

// FlattenCustomFields lifts a contract-level `custom_fields` sub-map
// into top-level customfield_NNNNN keys so screen validation and
// encoding operate on one flat namespace. Raw customfield_NNNNN keys
// already at the top level are left in place.
//
// It is fatal when:
//   - the custom_fields value is present but not an object — the CLI
//     cannot route it and silently dropping it would lose input;
//   - a nested key collides with an existing top-level key — the CLI
//     will not silently pick a winner.
//
// The input map is not mutated; a flattened copy is returned.
func FlattenCustomFields(fields map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		if k == customFieldsKey {
			continue
		}
		out[k] = v
	}
	raw, present := fields[customFieldsKey]
	if !present || raw == nil {
		return out, nil
	}
	nested, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("custom_fields must be an object mapping customfield ids to values, got %T", raw)
	}
	for k, v := range nested {
		if _, clash := out[k]; clash {
			return nil, fmt.Errorf("custom_fields key %q also set at the top level; supply it in exactly one place", k)
		}
		out[k] = v
	}
	return out, nil
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
	out := make(map[string]any, len(fields))
	var warnings []adf.Warning
	for k, v := range xmaps.Sorted(fields) {
		if schema.ValidFields[k] {
			out[k] = v
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
