package adfmode

import (
	"fmt"

	xstrings "github.com/gechr/x/strings"
)

// Mode is the resolved ADF processing mode for a single invocation.
type Mode int

const (
	// ModeBestEffort tolerates ADF that cannot be fully represented, degrading
	// with warnings; the default for reads and rendering.
	ModeBestEffort Mode = iota
	// ModeStrict rejects ADF that cannot be fully represented; the default for
	// mutation submission.
	ModeStrict
)

func (m Mode) String() string {
	switch m {
	case ModeStrict:
		return "strict"
	case ModeBestEffort:
		return "best-effort"
	default:
		return fmt.Sprintf("Mode(%d)", int(m))
	}
}

// FlagChoice is a bitmask so the resolver can detect if the caller wired both
// --adf-strict and --adf-best-effort on the same invocation (a programmer
// error — cobra mutex should prevent it, but we defend anyway).
type FlagChoice uint8

const (
	// FlagUnset means neither --adf-strict nor --adf-best-effort was passed.
	FlagUnset FlagChoice = 0
	// FlagStrict is the bit set by --adf-strict.
	FlagStrict FlagChoice = 1 << 0
	// FlagBestEffort is the bit set by --adf-best-effort.
	FlagBestEffort FlagChoice = 1 << 1
)

// Path is the kind of ADF interaction this invocation performs. It selects
// the default mode when no higher-priority signal is set.
type Path int

const (
	// PathRead reads an issue's ADF back from Jira (best-effort default).
	PathRead Path = iota
	// PathRender renders ADF to the terminal (best-effort default).
	PathRender
	// PathPlainExtract extracts plain text from ADF (best-effort default).
	PathPlainExtract
	// PathRawEmit emits the raw ADF document unchanged (best-effort default).
	PathRawEmit
	// PathMutationSubmit encodes ADF for a write to Jira (strict default).
	PathMutationSubmit
	// PathDryRun previews a mutation's ADF encoding (strict default).
	PathDryRun
)

// Inputs is the resolver input set. All fields are optional; zero values map
// to "unset" and the resolver falls through to the next precedence level.
type Inputs struct {
	Flag    FlagChoice
	Env     string // raw value of JIRA_ADF_STRICT
	Profile *bool  // nil = unset
	Path    Path
}

// Resolve returns the ADF Mode for the given inputs.
func Resolve(in Inputs) (Mode, error) {
	// 1. Flag wins outright. Defend against accidental both-set.
	if in.Flag&FlagStrict != 0 && in.Flag&FlagBestEffort != 0 {
		return 0, fmt.Errorf("adfmode: --adf-strict and --adf-best-effort are mutually exclusive")
	}
	if in.Flag&FlagStrict != 0 {
		return ModeStrict, nil
	}
	if in.Flag&FlagBestEffort != 0 {
		return ModeBestEffort, nil
	}

	// 2. Env override.
	if in.Env != "" {
		v, err := parseBoolish(in.Env)
		if err != nil {
			return 0, fmt.Errorf("adfmode: JIRA_ADF_STRICT=%q: %w", in.Env, err)
		}
		if v {
			return ModeStrict, nil
		}
		return ModeBestEffort, nil
	}

	// 3. Profile override.
	if in.Profile != nil {
		if *in.Profile {
			return ModeStrict, nil
		}
		return ModeBestEffort, nil
	}

	// 4. Per-path default.
	switch in.Path {
	case PathMutationSubmit, PathDryRun:
		return ModeStrict, nil
	case PathRead, PathRender, PathPlainExtract, PathRawEmit:
		return ModeBestEffort, nil
	default:
		return ModeBestEffort, nil
	}
}

// parseBoolish accepts the truthy/falsy set: 1/true/yes/on vs
// 0/false/no/off. Case-insensitive. Anything else is an error.
func parseBoolish(s string) (bool, error) {
	switch {
	case xstrings.IsTruthy(s):
		return true, nil
	case xstrings.IsFalsy(s):
		return false, nil
	default:
		return false, fmt.Errorf("not a boolish value (expected 1/true/yes/on or 0/false/no/off)")
	}
}
