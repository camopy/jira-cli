package cmdutil

import (
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/pflag"
)

// ExtendFlag attaches clib display metadata to the named flag in flags.
func ExtendFlag(flags *pflag.FlagSet, name string, extra clib.FlagExtra) {
	clib.Extend(flags.Lookup(name), extra)
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
		Enum:        []string{"key", "summary", "status", "assignee", "priority", "updated"},
		EnumTerse:   []string{"issue key", "title text", "workflow status", "assigned user", "priority level", "last-updated time"},
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
