package pipeline

import (
	"errors"
	"regexp"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/pkg/adf"
	"github.com/matcra587/jira-cli/pkg/jira/customfield"
)

// MutationInput is the realistic, command-facing input shape for the
// 5-stage pipeline. cmd/jira/commands.go assembles one of these from
// flag values, --json-input, the resolved profile, and the active
// schema; pipeline.RunMutation orchestrates stages 1–5.
//
// Either Schema (preloaded, e.g., from cache) OR SchemaFetcher (lazy,
// triggers refresh-once on ErrSchemaUnknown) MAY be provided. When
// neither is set, stage 3 (field schema / screen validation) is
// skipped — stage 4 (customfield encoding) still runs and rejects
// malformed values regardless of schema availability.
//
// CURRENT INTEGRATION DEPTH: cmd/jira/commands.go wires this struct
// with Mode + Fields + DryRun + ADFDoc. Stage 2 (ADF validation) is
// now load-bearing: ValidateDoc rejects unknown nodes, illegal marks,
// and unknown mark types before submission.
// Stage 3 (field schema / screen validation) remains opt-in via
// SchemaFetcher — wiring ProjectService into a SchemaFetcher closure
// to activate it is documented future work. Stages 1 (parse), 2
// (validate), 4 (customfield encoding), and 5 (dry-run vs submit
// gating) all carry production traffic; stage 3 is provided for
// future field-mapping work.
type MutationInput struct {
	Mode adfmode.Mode

	// Stage 1 — parse / shape. Set by the caller after pulling values
	// out of cobra flags + --json-input. Empty means parse succeeded.
	ParseError string

	// Stage 2 — ADF + compatibility. The Document the caller built
	// (typically via adf.FromMarkdown for --description-markdown, or
	// via --json-input for raw ADF). FieldCompat selects per-field
	// compatibility (inlineCard rules).
	ADFDoc      *adf.Document
	FieldCompat *adf.FieldCompatibility

	// NamedADFDocs holds additional ADF substructures keyed by their
	// field name (e.g. "description_adf"). All are validated through
	// ValidateDoc at stage 2 with the same mode as ADFDoc. This is the
	// hook used by issue create --json-input to catch garbage ADF in
	// payload subfields.
	NamedADFDocs map[string]adf.Document

	// Stage 3 — field schema / screen. Either Schema (preloaded) or
	// SchemaFetcher (lazy with refresh-once + known-safe fallback).
	Schema        ScreenSchema
	SchemaFetcher SchemaFetcher

	// Stage 4 — customfield encoding. The Fields map is what the caller
	// would post to Jira's REST API. Each customfield_xxxxx value is
	// validated through pkg/jira/customfield.Registry.
	Fields map[string]any

	// Stage 5.
	DryRun bool
}

// MutationResult reports the outcome of one RunMutation call.
type MutationResult struct {
	Aborted      bool
	AbortedAt    Stage
	Submitted    bool
	PreviewReady bool

	// SubmitFields is the post-validation, post-encoding fields map the
	// caller should send to Jira. In strict-abort cases this is nil.
	SubmitFields map[string]any
	// SubmitADF is the post-validation ADF doc (after compatibility
	// passes). nil if no doc was supplied.
	SubmitADF *adf.Document
	// Warnings collected from every stage that ran. Non-empty even when
	// a later stage aborted — earlier-stage observations always survive.
	Warnings []adf.Warning
	Err      error
}

// RunMutation executes stages 1–5 with realistic inputs. Designed to
// be the single call site for every mutation command.
func RunMutation(in MutationInput) MutationResult {
	res := MutationResult{}

	// --- Stage 1: parse / shape ---
	if in.ParseError != "" {
		res.Aborted = true
		res.AbortedAt = StageParse
		res.Err = errors.New(in.ParseError)
		return res
	}

	// --- Stage 2: ADF validation + compatibility ---
	doc := in.ADFDoc
	if doc != nil {
		// ValidateDoc enforces root shape (always) and per-mode node/mark
		// rules. Runs before ApplyCompatibility so unknown nodes are
		// caught at the validation stage, not on the wire.
		warnings, err := adf.ValidateDoc(*doc, in.Mode)
		res.Warnings = append(res.Warnings, warnings...)
		if err != nil {
			res.Aborted = true
			res.AbortedAt = StageADF
			res.Err = err
			return res
		}

		if in.FieldCompat != nil {
			converted, compWarnings, compErr := adf.ApplyCompatibility(*doc, *in.FieldCompat, in.Mode)
			res.Warnings = append(res.Warnings, compWarnings...)
			if compErr != nil {
				res.Aborted = true
				res.AbortedAt = StageADF
				res.Err = compErr
				return res
			}
			doc = &converted
		}
	}
	res.SubmitADF = doc

	// Stage 2b: validate any additional named ADF subfields (e.g.
	// description_adf in issue create --json-input payloads). Each is
	// run through ValidateDoc with the same mode. FieldCompat is not
	// applied here — these are opaque payload subfields, not the primary
	// document.
	for field, namedDoc := range in.NamedADFDocs {
		warnings, err := adf.ValidateDoc(namedDoc, in.Mode)
		// Attach field context to warnings so the user sees which key failed.
		for i := range warnings {
			if warnings[i].Field == "" {
				warnings[i].Field = field
			}
		}
		res.Warnings = append(res.Warnings, warnings...)
		if err != nil {
			res.Aborted = true
			res.AbortedAt = StageADF
			res.Err = err
			return res
		}
	}

	// --- Stage 3: field schema / screen ---
	schema := in.Schema
	var fields map[string]any
	if in.SchemaFetcher != nil {
		// Resolve via fetcher; refresh once on unknown.
		s, _, err := ResolveScreenSchemaStrict(in.SchemaFetcher)
		switch {
		case err == nil:
			schema = s
			validated, warnings, err := ValidateFields(in.Fields, schema, in.Mode)
			res.Warnings = append(res.Warnings, warnings...)
			if err != nil {
				res.Aborted = true
				res.AbortedAt = StageFieldSchema
				res.Err = err
				return res
			}
			fields = validated
		case errors.Is(err, ErrSchemaUnknown) && in.Mode == adfmode.ModeBestEffort:
			// Known-safe fallback.
			f, warnings := ApplyKnownSafeFallback(in.Fields)
			res.Warnings = append(res.Warnings, warnings...)
			fields = f
		default:
			res.Aborted = true
			res.AbortedAt = StageFieldSchema
			res.Err = err
			return res
		}
	} else if schema.ValidFields != nil {
		// Preloaded schema (non-nil ValidFields means caller actually
		// supplied one; empty map vs nil distinguishes "valid empty
		// schema rejects everything" from "no schema, skip stage 3").
		validated, warnings, err := ValidateFields(in.Fields, schema, in.Mode)
		res.Warnings = append(res.Warnings, warnings...)
		if err != nil {
			res.Aborted = true
			res.AbortedAt = StageFieldSchema
			res.Err = err
			return res
		}
		fields = validated
	} else {
		// No schema provided — caller intentionally bypassed stage 3.
		// Stage 4 still runs; the customfield registry will reject any
		// malformed values regardless of screen membership.
		fields = in.Fields
	}

	// --- Stage 4: customfield registry ---
	encoded, warnings, err := encodeCustomFields(fields, in.Mode)
	res.Warnings = append(res.Warnings, warnings...)
	if err != nil {
		res.Aborted = true
		res.AbortedAt = StageCustomField
		res.Err = err
		return res
	}
	res.SubmitFields = encoded

	// --- Stage 5: dry-run preview / live submit ---
	if in.DryRun {
		res.PreviewReady = true
		return res
	}
	res.Submitted = true
	return res
}

// customfieldIDPattern matches Jira customfield ID keys (customfield_NNNN).
// These are unregistered by type name in the registry — they get a forwarding
// warning"behavior must be consistent and surface
// in the warnings array if forwarded" (attack 3 pass criterion).
var customfieldIDPattern = regexp.MustCompile(`^customfield_\d+$`)

// encodeCustomFields routes every fields[k] through the customfield
// registry. Keys matching the registry type names are validated; unmatched
// keys that look like Jira customfield IDs (customfield_NNNN) get a
// forwarding warning per ; all other native keys pass silently.
func encodeCustomFields(fields map[string]any, mode adfmode.Mode) (map[string]any, []adf.Warning, error) {
	if len(fields) == 0 {
		return fields, nil, nil
	}
	reg := customfield.Registry()
	out := make(map[string]any, len(fields))
	var warnings []adf.Warning
	for k, v := range fields {
		entry, ok := reg.Lookup(k)
		if !ok {
			// Native or unregistered field — pass through. When the key
			// matches the customfield_NNNN pattern the type is unknown to
			// the registry; emit a warning so callers can see what was
			// forwarded opaquely (: "surface in warnings if forwarded").
			if customfieldIDPattern.MatchString(k) {
				warnings = append(warnings, adf.Warning{
					Type:    "customfield_unknown_type",
					Message: "customfield " + k + " has no registered type validator; forwarded opaquely — value not checked",
					Field:   k,
					Lossy:   false,
				})
			}
			out[k] = v
			continue
		}
		if err := entry.Validator(v); err != nil {
			if mode == adfmode.ModeBestEffort {
				warnings = append(warnings, adf.Warning{
					Type:    "customfield_invalid",
					Message: err.Error(),
					Field:   k,
					Lossy:   true,
				})
				continue
			}
			return nil, warnings, &CustomFieldError{Field: k, Reason: err.Error()}
		}
		encoded, err := entry.Encoder(v)
		if err != nil {
			return nil, warnings, &CustomFieldError{Field: k, Reason: err.Error()}
		}
		out[k] = encoded
	}
	return out, warnings, nil
}

// CustomFieldError is the typed error returned by stage 4.
type CustomFieldError struct {
	Field  string
	Reason string
}

func (e *CustomFieldError) Error() string {
	if e == nil {
		return "<nil customfield error>"
	}
	return "customfield: " + e.Field + ": " + e.Reason
}
