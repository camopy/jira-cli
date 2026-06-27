package cmdutil

import (
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/pflag"
)

// Flag registration helpers that declare a flag and attach its clib metadata in
// a single call, so a flag can never be registered without the group,
// placeholder, enum and completion data that the terminal help and the
// generated command reference render from (via clib.FlagMeta). Every helper is
// a thin wrapper over the matching pflag method plus clib.Extend.
//
// The helpers are deliberately named with an "Add" prefix (AddStringVar, not
// StringVar) rather than mirroring pflag's names. Two reasons: it reads as
// "this is our register-and-extend helper, not pflag" (gh uses the same Add*
// convention for AddJSONFlags etc.), and — concretely — it lets the ruleguard
// "raw pflag declaration" rule (ruleguard/rules.go) match pflag's StringVar /
// BoolVar / … by method name without also flagging these wrappers, which a
// name-mirroring design would make indistinguishable.
//
// Raw pflag declaration methods are banned outside this package by that rule;
// command code must use these helpers or one of the semantic bundlers
// (AddDryRunFlag, ExtendPaginationFlags, AddIssueColumnFlags, …). The bundlers
// are the preferred path for recurring flag clusters; reach for the generic
// helpers below for genuine one-offs.
//
// This file depends only on clib and pflag — no jira-cli-specific types — so it
// is portable: copy it (and the ruleguard rule) into any other clib-backed
// cobra CLI to get the same "metadata is mandatory" guarantee.

// AddStringVar declares a string flag bound to p and attaches its clib metadata.
func AddStringVar(fs *pflag.FlagSet, p *string, name, value, usage string, extra clib.FlagExtra) {
	fs.StringVar(p, name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
}

// AddBoolVar declares a bool flag bound to p and attaches its clib metadata.
func AddBoolVar(fs *pflag.FlagSet, p *bool, name string, value bool, usage string, extra clib.FlagExtra) {
	fs.BoolVar(p, name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
}

// AddIntVar declares an int flag bound to p and attaches its clib metadata.
func AddIntVar(fs *pflag.FlagSet, p *int, name string, value int, usage string, extra clib.FlagExtra) {
	fs.IntVar(p, name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
}

// AddStringSliceVar declares a repeatable string flag bound to p (comma-split,
// pflag's StringSlice semantics) and attaches its clib metadata.
func AddStringSliceVar(fs *pflag.FlagSet, p *[]string, name string, value []string, usage string, extra clib.FlagExtra) {
	fs.StringSliceVar(p, name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
}

// AddStringArrayVar declares a repeatable string flag bound to p (one value per
// use, no comma-split, pflag's StringArray semantics) and attaches its clib
// metadata.
func AddStringArrayVar(fs *pflag.FlagSet, p *[]string, name string, value []string, usage string, extra clib.FlagExtra) {
	fs.StringArrayVar(p, name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
}

// AddString declares a string flag, attaches its clib metadata, and returns the
// bound pointer. Use the Var form when you already hold the target variable.
func AddString(fs *pflag.FlagSet, name, value, usage string, extra clib.FlagExtra) *string {
	p := fs.String(name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
	return p
}

// AddStringP declares a string flag with a shorthand letter, attaches its clib
// metadata, and returns the bound pointer.
func AddStringP(fs *pflag.FlagSet, name, shorthand, value, usage string, extra clib.FlagExtra) *string {
	p := fs.StringP(name, shorthand, value, usage)
	clib.Extend(fs.Lookup(name), extra)
	return p
}

// AddBool declares a bool flag, attaches its clib metadata, and returns the
// bound pointer.
func AddBool(fs *pflag.FlagSet, name string, value bool, usage string, extra clib.FlagExtra) *bool {
	p := fs.Bool(name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
	return p
}

// AddBoolP declares a bool flag with a shorthand letter, attaches its clib
// metadata, and returns the bound pointer.
func AddBoolP(fs *pflag.FlagSet, name, shorthand string, value bool, usage string, extra clib.FlagExtra) *bool {
	p := fs.BoolP(name, shorthand, value, usage)
	clib.Extend(fs.Lookup(name), extra)
	return p
}

// AddInt declares an int flag, attaches its clib metadata, and returns the
// bound pointer.
func AddInt(fs *pflag.FlagSet, name string, value int, usage string, extra clib.FlagExtra) *int {
	p := fs.Int(name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
	return p
}

// AddDuration declares a time.Duration flag, attaches its clib metadata, and
// returns the bound pointer.
func AddDuration(fs *pflag.FlagSet, name string, value time.Duration, usage string, extra clib.FlagExtra) *time.Duration {
	p := fs.Duration(name, value, usage)
	clib.Extend(fs.Lookup(name), extra)
	return p
}
