// Package adfcmd implements the `jira adf` command group: standalone
// Markdown→ADF conversion and linting, plus the explicitly-lossy
// ADF→Markdown projection. Both are local-only — no profile, no network —
// so an agent can pre-flight rich text in isolation and submit the
// resulting document anywhere `--json-input` is accepted.
package adfcmd
