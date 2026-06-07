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
