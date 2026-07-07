// Package errtax is the CLI's error taxonomy: the stable error types and
// codes the JSON envelope emits, and the registry that binds every code to
// its classification. It is a stdlib-only leaf so jira, config, adf,
// issuekey, and cli can all describe their errors with a [Code] without
// import cycles; the cli error mapper derives everything else from here.
//
// # Registry
//
// The registry is the single source of truth for code → {type, exit, hint,
// retryable}. An error carries only its [Code]; [Lookup] supplies the rest.
// Rows are declared once in a composite literal and never mutated. The
// taxonomy guard test enumerates [Codes] and holds the registry to the
// frozen, human-reviewed contract table, so a new code cannot ship without
// a reviewed row.
package errtax
