// Package issuekey parses and expands issue-key expressions into canonical Jira
// keys. It accepts single keys (PROJ-1), comma lists, and inclusive ranges
// written with ":" or ".." (PROJ-1..PROJ-5 or PROJ-1..5).
//
// A local expansion cap bounds the blow-up: a range is expanded client-side, so
// an unbounded PROJ-1..PROJ-9999999 would otherwise materialize millions of
// keys before any Jira call. The cap is enforced here, before the network, and
// a hit is reported as a typed ExpansionLimitError.
package issuekey
