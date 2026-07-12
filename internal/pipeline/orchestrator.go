package pipeline

import (
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// Stage identifies which step of the mutation pipeline the pipeline reached.
type Stage int

const (
	// StageNone is the zero value: no stage has run.
	StageNone Stage = iota
	// StageParse is payload parsing and shape validation.
	StageParse
	// StageADF is ADF and Markdown compatibility validation.
	StageADF
	// StageFieldSchema is field-schema and screen validation.
	StageFieldSchema
	// StageCustomField is custom-field encoding and validation.
	StageCustomField
	// StageSubmit is the terminal stage: the mutation is ready to send.
	StageSubmit
)

func (s Stage) String() string {
	switch s {
	case StageParse:
		return "parse"
	case StageADF:
		return "adf"
	case StageFieldSchema:
		return "field_schema"
	case StageCustomField:
		return "custom_field"
	case StageSubmit:
		return "submit"
	}
	return "none"
}

// Input is the contract test surface for the pipeline orchestrator. It
// abstracts over the actual stage implementations so unit tests can
// inject failures at any stage without wiring full schemas. Production
// callers populate the same fields from real validation results.
type Input struct {
	Mode adfmode.Mode

	// Stage 1 — parse.
	ParseError string

	// Stage 2 — ADF.
	ADFInvalid bool   // strict: fatal; best-effort: degradable iff ADFWarning set
	ADFWarning string // if non-empty, surfaces as a stage-2 warning

	// Stage 3 — field schema.
	FieldOnScreen bool

	// Stage 4 — customfield encoding.
	CustomEncodeOK bool

	// Stage 5.
	DryRun bool
}

// Result reports what happened. Aborted/AbortedAt cooperate so the CLI
// can map the abort to a specific exit code; Warnings always include
// every observation from earlier stages even when a later stage aborts.
type Result struct {
	Aborted      bool
	AbortedAt    Stage
	Submitted    bool
	PreviewReady bool
	Warnings     []adf.Warning
	Err          error
}

// Run executes stages 1–5 in deterministic order.
func Run(in Input) Result {
	var res Result

	// --- Stage 1: parse / shape ---
	// Always fatal in both modes when there's a parse error.
	if in.ParseError != "" {
		res.Aborted = true
		res.AbortedAt = StageParse
		return res
	}

	// --- Stage 2: ADF + compatibility ---
	if in.ADFWarning != "" {
		res.Warnings = append(res.Warnings, adf.Warning{
			Type:    "adf_compatibility",
			Message: in.ADFWarning,
			Lossy:   true,
		})
	}
	if in.ADFInvalid {
		// In best-effort mode, fatal only when no valid ADF could be produced
		// — proxied here by "no ADFWarning was emitted to indicate degradation".
		if in.Mode == adfmode.ModeStrict || in.ADFWarning == "" {
			res.Aborted = true
			res.AbortedAt = StageADF
			return res
		}
	}

	// --- Stage 3: field schema / screen ---
	if !in.FieldOnScreen {
		if in.Mode == adfmode.ModeStrict {
			res.Aborted = true
			res.AbortedAt = StageFieldSchema
			return res
		}
		// best-effort: drop with warning, continue.
		res.Warnings = append(res.Warnings, adf.Warning{
			Type:    "field_not_on_screen",
			Message: "field dropped — not on the active screen",
			Lossy:   true,
		})
	}

	// --- Stage 4: customfield registry ---
	// Encoding failures are fatal in both modes.
	if !in.CustomEncodeOK {
		res.Aborted = true
		res.AbortedAt = StageCustomField
		return res
	}

	// --- Stage 5: dry-run preview / live submit ---
	res.AbortedAt = StageNone
	if in.DryRun {
		res.PreviewReady = true
		return res
	}
	res.Submitted = true
	return res
}
