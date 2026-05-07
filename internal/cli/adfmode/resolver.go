// Package adfmode resolves the strict vs best-effort ADF mode for a given
// CLI invocation, applying both the precedence ladder and per-path defaults.
//
// Precedence (highest first): command flag > JIRA_ADF_STRICT env > profile
// adf_strict > per-path default (read/render → best-effort, mutation submit →
// strict). The resolver is the only place mode selection happens; commands
// pass their resolved Mode into pkg/adf entry points.
package adfmode

import (
	"fmt"
	"strings"
)

// Mode is the resolved ADF processing mode for a single invocation.
type Mode int

const (
	ModeBestEffort Mode = iota
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
	FlagUnset      FlagChoice = 0
	FlagStrict     FlagChoice = 1 << 0
	FlagBestEffort FlagChoice = 1 << 1
)

// Path is the kind of ADF interaction this invocation performs. It selects
// the default mode when no higher-priority signal is set.
type Path int

const (
	PathRead Path = iota
	PathRender
	PathPlainExtract
	PathRawEmit
	PathMutationSubmit
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
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("not a boolish value (expected 1/true/yes/on or 0/false/no/off)")
	}
}
