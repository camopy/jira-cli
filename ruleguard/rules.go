//go:build ruleguard

// Package gorules holds the ruleguard rules golangci-lint runs via gocritic.
// The build tag keeps `go build`/`go vet` from compiling this file — only the
// linter's bundled ruleguard engine reads it.
package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// unwrappedBlockingCall flags a blocking Jira service call that takes
// cmd.Context() directly. Such a call runs with no spinner and no debug
// lifecycle — the terminal just hangs. Route it through cmdutil.Spin (single
// op) or cmdutil.FanOutKeysProgress (many keys), whose closures hand the call
// a ctx, so the user gets feedback. Calls already wrapped use the closure's
// ctx, not cmd.Context(), so they do not match.
//
// Matched by method name: the list below is every request-making method across
// the jira *Service interfaces. Keep it in sync when a service gains a method
// (the bundled ruleguard can't resolve the interface types directly, so a
// type-based rule isn't available here). Test files are exempt via
// .golangci.yml, so a local lookalike (a credential store's Get) in a test
// never trips it. BoardService.ResolveOne is deliberately absent: it is a
// cache-only resolver that never hits the network, so it needs no spinner.
func unwrappedBlockingCall(m dsl.Matcher) {
	m.Match(
		`$svc.$method(cmd.Context(), $*_)`,
		`$svc.$method(cmd.Context())`,
	).
		Where(m["method"].Text.Matches(`^(Add|AddComment|AddIssue|AddRemoteLink|AddWithVisibility|ApproximateCount|AutocompleteData|Clone|Create|Delete|Download|Edit|Get|GetEditSchemaForProfile|GetFieldSchema|GetFieldSchemaForProfile|IssuesInEpic|Link|List|ListAll|Move|MyPermissions|Myself|ProjectsForBoard|Remove|RemoveIssue|ResolveAccountID|ResolveUser|Search|Transition|Transitions|Update)$`)).
		Report(`blocking Jira call passes cmd.Context() directly — wrap it in cmdutil.Spin or cmdutil.FanOutKeysProgress so the user gets a spinner + debug lifecycle`)
}

// rawPflagDeclaration flags a flag declared straight on a pflag.FlagSet without
// clib metadata. jira-cli renders both its terminal help and its generated
// command reference from clib metadata (group, placeholder, enum, completion),
// so a flag registered with a bare pflag setter shows up ungrouped, with a
// mechanical placeholder and no allowed values. Declare flags through the
// cmdutil register-and-extend helpers (cmdutil.StringVar / BoolVar / IntVar /
// StringSliceVar / StringArrayVar / String / StringP / Bool / BoolP / Int /
// Duration) or a semantic bundler (ExtendDryRunFlag, ExtendPaginationFlags,
// AddIssueColumnFlags, …), which attach the metadata in the same call. The
// cmdutil package (the wrapper layer itself) and test files are exempt via
// .golangci.yml.
//
// The rule matches by pflag method NAME on any receiver — the bundled ruleguard
// in this golangci build resolves neither receiver types (.Type.Is) nor chained
// receiver patterns ($x.Flags().$method) reliably, so name matching is the only
// construct that works. Two things share these names and must NOT be flagged:
//   - the cmdutil register-and-extend helpers — named with an "Add" prefix
//     (AddStringVar, AddBoolP, …) precisely so they fall outside this regex;
//   - clog's structured-logging builders (.Int/.Bool/.Duration/…) — excluded
//     because the regex requires a Var / VarP / P suffix, which clog never uses.
//
// Matched: the *Var, *VarP and *P pflag forms, plus the bare Var/VarP. Bare
// typed forms (cmd.Flags().String(...)) are deliberately NOT matched — they
// collide with clog's bare .String/.Int/.Bool — so a future raw bare form is a
// known backstop hole; the existing bare declarations were all migrated to
// cmdutil.AddString/AddInt/etc.
func rawPflagDeclaration(m dsl.Matcher) {
	m.Match(`$fs.$method($*_)`).
		Where(m["method"].Text.Matches(`^(String|Bool|Int|Int8|Int16|Int32|Int64|Uint|Uint8|Uint16|Uint32|Uint64|Float32|Float64|Duration|Count|IP|IPNet|IPMask|BytesHex|BytesBase64|StringSlice|StringArray|IntSlice|Int32Slice|Int64Slice|UintSlice|BoolSlice|Float32Slice|Float64Slice|DurationSlice|IPSlice|StringToString|StringToInt|StringToInt64)(Var)?P$|^(String|Bool|Int|Int8|Int16|Int32|Int64|Uint|Uint8|Uint16|Uint32|Uint64|Float32|Float64|Duration|Count|IP|IPNet|IPMask|BytesHex|BytesBase64|StringSlice|StringArray|IntSlice|Int32Slice|Int64Slice|UintSlice|BoolSlice|Float32Slice|Float64Slice|DurationSlice|IPSlice|StringToString|StringToInt|StringToInt64)Var$|^VarP?$`)).
		Report(`flag declared without clib metadata — use a cmdutil register-and-extend helper (cmdutil.AddStringVar/AddBoolVar/AddIntVar/AddStringSliceVar/AddStringArrayVar/AddString/AddStringP/AddBool/AddBoolP/AddInt/AddDuration) or a bundler so help and the docs reference get groups, placeholders and allowed values`)
}
