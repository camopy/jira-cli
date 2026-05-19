package main

import (
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/pflag"
)

func extendFlag(flags *pflag.FlagSet, name string, extra clib.FlagExtra) {
	clib.Extend(flags.Lookup(name), extra)
}

func extendFileFlag(flags *pflag.FlagSet, name, group, placeholder string) {
	extendFlag(flags, name, clib.FlagExtra{
		Group:       group,
		Placeholder: placeholder,
		Hint:        "file",
	})
}

func extendDryRunFlag(flags *pflag.FlagSet) {
	extendFlag(flags, "dry-run", clib.FlagExtra{Group: "Safety", Terse: "preview only"})
}

func extendForceFlag(flags *pflag.FlagSet) {
	extendFlag(flags, "force", clib.FlagExtra{Group: "Safety", Terse: "confirm destructive action"})
}

func extendPaginationFlags(flags *pflag.FlagSet) {
	extendFlag(flags, "limit", clib.FlagExtra{Group: "Pagination", Placeholder: "N"})
	extendFlag(flags, "all", clib.FlagExtra{Group: "Pagination"})
}

func extendRefreshFlags(flags *pflag.FlagSet) {
	extendFlag(flags, "refresh", clib.FlagExtra{Group: "Cache"})
	extendFlag(flags, "ttl-minutes", clib.FlagExtra{Group: "Cache", Placeholder: "N"})
	extendFlag(flags, "unbounded", clib.FlagExtra{Group: "Pagination"})
}

func extendSafetyFlag(flags *pflag.FlagSet, name string) {
	extendFlag(flags, name, clib.FlagExtra{Group: "Safety"})
}

func extendWatcherUserFlag(flags *pflag.FlagSet) {
	extendFlag(flags, "user", clib.FlagExtra{Group: "User", Placeholder: "IDENTIFIER"})
}

func extendWatcherValidationFlags(flags *pflag.FlagSet) {
	extendFlag(flags, "no-readback", clib.FlagExtra{Group: "Validation"})
	extendFlag(flags, "validate-remote", clib.FlagExtra{Group: "Validation"})
}
