package cmdutil

import (
	"encoding/json"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/pflag"
)

// ExtendFlag attaches clib display metadata to the named flag in flags.
func ExtendFlag(flags *pflag.FlagSet, name string, extra clib.FlagExtra) {
	clib.Extend(flags.Lookup(name), extra)
}

// clibExtraAnnotationKey mirrors the (unexported) annotation key clib's cobra
// adapter stores a FlagExtra under. clib exposes Extend to write an extra but
// no reader, so merging one field into an existing extra means reading the raw
// annotation here. The key and its JSON payload are a stable interop contract:
// clib's own help and completion parse this same annotation.
const clibExtraAnnotationKey = "clib.extra"

// ExtendFlagEnum attaches enum completion values and their parallel terse
// descriptions to flag as clib metadata, preserving any FlagExtra already set
// on it rather than overwriting it. It exists so a flag whose completion
// originates outside the cmdutil.Add* helpers (a mounted third-party command)
// can be taught to the clib-driven completion path, which never invokes
// cobra's own completion callbacks. A nil flag or empty value set is a no-op;
// a mismatched description set is omitted rather than handing invalid metadata
// to clib.
func ExtendFlagEnum(flag *pflag.Flag, enum, enumTerse []string) {
	if flag == nil || len(enum) == 0 {
		return
	}
	extra := clib.FlagExtra{Enum: enum}
	if len(enumTerse) == len(enum) {
		extra.EnumTerse = enumTerse
	}
	if raw := flag.Annotations[clibExtraAnnotationKey]; len(raw) > 0 {
		var existing clib.FlagExtra
		if err := json.Unmarshal([]byte(raw[0]), &existing); err == nil {
			existing.Enum = enum
			existing.EnumTerse = extra.EnumTerse
			extra = existing
		}
	}
	clib.Extend(flag, extra)
}

// ExtendFileFlag marks a flag as a file-path input with the given group and
// placeholder label.
func ExtendFileFlag(flags *pflag.FlagSet, name, group, placeholder string) {
	ExtendFlag(flags, name, clib.FlagExtra{
		Group:       group,
		Placeholder: placeholder,
		Hint:        "file",
	})
}

// ExtendDryRunFlag attaches Safety-group metadata to the --dry-run flag.
func ExtendDryRunFlag(flags *pflag.FlagSet) {
	ExtendFlag(flags, "dry-run", clib.FlagExtra{Group: "Safety", Terse: "preview only"})
}

// ExtendForceFlag attaches Safety-group metadata to the --force flag.
func ExtendForceFlag(flags *pflag.FlagSet) {
	ExtendFlag(flags, "force", clib.FlagExtra{Group: "Safety", Terse: "confirm destructive action"})
}

// ExtendPaginationFlags attaches Pagination-group metadata to the --limit and
// --all flags.
func ExtendPaginationFlags(flags *pflag.FlagSet) {
	ExtendFlag(flags, "limit", clib.FlagExtra{Group: "Pagination", Placeholder: "N"})
	ExtendFlag(flags, "all", clib.FlagExtra{Group: "Pagination"})
}

// ExtendRefreshFlags attaches Cache-group metadata to the --refresh and
// --ttl-minutes flags, and Pagination-group metadata to --unbounded.
func ExtendRefreshFlags(flags *pflag.FlagSet) {
	ExtendFlag(flags, "refresh", clib.FlagExtra{Group: "Cache"})
	ExtendFlag(flags, "ttl-minutes", clib.FlagExtra{Group: "Cache", Placeholder: "N"})
	ExtendFlag(flags, "unbounded", clib.FlagExtra{Group: "Pagination"})
}

// ExtendSafetyFlag attaches Safety-group metadata to the named flag.
func ExtendSafetyFlag(flags *pflag.FlagSet, name string) {
	ExtendFlag(flags, name, clib.FlagExtra{Group: "Safety"})
}

// AddIssueColumnFlags registers the --columns and --tsv human-output flags on
// flags, binding them to columns and tsv, and groups them under Output.
func AddIssueColumnFlags(flags *pflag.FlagSet, columns *[]string, tsv *bool) {
	flags.StringSliceVar(columns, "columns", nil, "Select and order columns for human/TSV output")
	flags.BoolVar(tsv, "tsv", false, "Render as tab-separated values for scripts (off a TTY this implies `--output=human`)")
	ExtendFlag(flags, "columns", clib.FlagExtra{
		Group:       "Output",
		Placeholder: "COLS",
		Terse:       "output columns",
		Enum:        []string{"key", "summary", "status", "assignee", "priority", "created", "updated"},
		EnumTerse:   []string{"issue key", "title text", "workflow status", "assigned user", "priority level", "creation time", "last-updated time"},
	})
	ExtendFlag(flags, "tsv", clib.FlagExtra{Group: "Output"})
}

// ExtendWatcherUserFlag attaches User-group metadata to the --user flag.
func ExtendWatcherUserFlag(flags *pflag.FlagSet) {
	ExtendFlag(flags, "user", clib.FlagExtra{Group: "User", Placeholder: "IDENTIFIER"})
}

// ExtendWatcherValidationFlags attaches Validation-group metadata to the
// --no-readback and --validate-remote flags.
func ExtendWatcherValidationFlags(flags *pflag.FlagSet) {
	ExtendFlag(flags, "no-readback", clib.FlagExtra{Group: "Validation"})
	ExtendFlag(flags, "validate-remote", clib.FlagExtra{Group: "Validation"})
}

// AddDryRunFlag declares the standard --dry-run preview flag bound to p with the
// given usage text, grouped under Safety. Prefer this over a raw BoolVar so the
// dry-run flag is grouped and described consistently across every mutation.
func AddDryRunFlag(flags *pflag.FlagSet, p *bool, usage string) {
	AddBoolVar(flags, p, "dry-run", false, usage, clib.FlagExtra{Group: "Safety", Terse: "preview only"})
}

// AddForceFlag declares the standard --force flag bound to p with the given
// usage text, grouped under Safety.
func AddForceFlag(flags *pflag.FlagSet, p *bool, usage string) {
	AddBoolVar(flags, p, "force", false, usage, clib.FlagExtra{Group: "Safety", Terse: "confirm destructive action"})
}

// AddFileFlag declares a string flag bound to p that names a file-path input,
// grouped under group with the given placeholder and a file value hint (for
// shell completion). Prefer this for any flag whose value is a path.
func AddFileFlag(flags *pflag.FlagSet, p *string, name, value, usage, group, placeholder string) {
	AddStringVar(flags, p, name, value, usage, clib.FlagExtra{Group: group, Placeholder: placeholder, Hint: "file"})
}
