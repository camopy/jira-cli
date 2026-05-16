package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/pkg/adf"
	"github.com/matcra587/jira-cli/pkg/jira/customfield"
)

// CustomFieldDecision is the outcome of CustomFieldDropPolicy for one
// custom field value: whether to abort the whole mutation (Fatal),
// whether to forward the value unchanged (Forward), and a human-facing
// warning to surface when the value is forwarded or dropped without an
// abort.
type CustomFieldDecision struct {
	Fatal   bool
	Forward bool
	Warning string
}

// CustomFieldDropPolicy is the single shared decision used by every
// non-TUI mutation path (create/edit/clone/move) so they treat a
// problematic user-supplied custom field identically.
//
//   - A malformed value for a KNOWN schema.custom type is fatal in
//     strict mode (dropping it would lose user input) and dropped with
//     a warning in best-effort mode.
//   - A value for an UNKNOWN type (no schema.custom token, e.g. a
//     marketplace/vendor field) is forwarded opaquely with a warning in
//     both modes — the CLI cannot validate the shape, and forwarding
//     with a warning is the shipped contract. It is never silently
//     dropped.
//
// fieldType is the schema.custom token resolved for the field, or ""
// when no type is known. malformed reports whether the value failed
// validation for a known type.
func CustomFieldDropPolicy(fieldID, fieldType string, malformed bool, mode adfmode.Mode) CustomFieldDecision {
	if fieldType == "" {
		// Unknown type: opaque pass-through with a warning. Forwarding a
		// value we cannot validate is the shipped contract; never drop
		// it. The warning is deliberately loud — in strict mode this is
		// the one custom-field value that reaches Jira unverified.
		return CustomFieldDecision{
			Forward: true,
			Warning: "WARNING: customfield " + fieldID + " has no known type (vendor/marketplace field); " +
				"its value is forwarded to Jira UNVERIFIED — the CLI cannot check its shape",
		}
	}
	if !malformed {
		return CustomFieldDecision{Forward: true}
	}
	if mode == adfmode.ModeBestEffort {
		return CustomFieldDecision{
			Warning: "customfield " + fieldID + " value is malformed for type " + fieldType + "; dropped",
		}
	}
	return CustomFieldDecision{Fatal: true}
}

// encodeCustomFieldByType validates and encodes one custom field value
// against a known schema.custom token. The token is mapped onto a
// customfield registry entry; the entry's validator/encoder pair is the
// single source of truth for both the accepted input shape and the wire
// shape — the encoder lifts a bare scalar/label into the typed wire
// shape Jira expects. Tokens with no registry mapping are treated as
// unknown so the caller falls back to opaque forwarding.
//
// The textarea token is handled specially: on Jira Cloud v3 a
// multi-line-text custom field takes an ADF document, not a registry
// row, so its value is encoded through the same Markdown->ADF path the
// issue description uses.
func encodeCustomFieldByType(fieldType string, value any) (encoded any, knownType bool, err error) {
	if fieldType == "textarea" {
		doc, terr := encodeTextareaADF(value)
		if terr != nil {
			return nil, true, terr
		}
		return doc, true, nil
	}
	name := registryNameForSchemaCustom(fieldType)
	if name == "" {
		return value, false, nil
	}
	entry, ok := customfield.Registry().Lookup(name)
	if !ok {
		return value, false, nil
	}
	if verr := entry.Validator(value); verr != nil {
		return nil, true, verr
	}
	out, eerr := entry.Encoder(value)
	if eerr != nil {
		return nil, true, eerr
	}
	return out, true, nil
}

// encodeTextareaADF encodes a multi-line-text custom field value into
// an ADF document. A plain string is treated as Markdown and converted
// via the shared Markdown->ADF path; a value already shaped as an ADF
// document is parsed and accepted as-is. Any other shape is rejected.
func encodeTextareaADF(value any) (adf.Document, error) {
	switch v := value.(type) {
	case string:
		doc, _, err := adf.FromMarkdownLossy(v)
		if err != nil {
			return adf.Document{}, err
		}
		return doc, nil
	case adf.Document:
		return v, nil
	case map[string]any:
		encoded, err := json.Marshal(v)
		if err != nil {
			return adf.Document{}, err
		}
		doc, _, perr := adf.Parse(encoded)
		if perr != nil {
			return adf.Document{}, perr
		}
		return doc, nil
	default:
		return adf.Document{}, fmt.Errorf("textarea value must be a string (Markdown) or an ADF document, got %T", value)
	}
}

// registryNameForSchemaCustom maps a Jira schema.custom token (the
// trailing identifier of com.atlassian.jira.plugin.system.customfieldtypes:*,
// or a bare token) onto a customfield registry entry name. The registry
// holds the validator/encoder; this table only bridges Jira's vocabulary
// to the registry's. Unknown tokens return "" so the value is forwarded
// opaquely rather than encoded against the wrong shape. The textarea
// token is handled by encodeCustomFieldByType, not here — it maps to an
// ADF document rather than a registry row.
func registryNameForSchemaCustom(token string) string {
	switch token {
	case "textfield", "url", "string":
		return "string"
	case "float", "number":
		return "number"
	case "datepicker", "date":
		return "date"
	case "datetime":
		return "datetime"
	case "labels":
		return "labels"
	case "select", "radiobuttons":
		return "select"
	case "multiselect", "multicheckboxes":
		return "multiselect"
	case "cascadingselect":
		return "cascadingselect"
	case "userpicker", "user":
		return "user"
	case "multiuserpicker":
		return "multiuser"
	case "grouppicker", "group":
		return "group"
	case "multigrouppicker":
		return "multigroup"
	case "version":
		return "versionpicker"
	case "multiversion":
		return "multiversionpicker"
	case "project":
		return "projectpicker"
	}
	return ""
}

// fieldTypeFor returns the schema.custom token the screen schema
// declares for a field id, or "" when the schema declares none.
func fieldTypeFor(schema ScreenSchema, fieldID string) string {
	if schema.FieldTypes == nil {
		return ""
	}
	return schema.FieldTypes[fieldID]
}
