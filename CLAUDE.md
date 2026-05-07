# jira-cli — agent notes

Terminal-first Jira CLI for developer and agent workflows, built in Go
with Cobra.

## Layout

- `cmd/jira/` — command wiring, the embedded agent guide, and the
  manpage source.
- `internal/` — runtime, config, credential backends, output rendering,
  the mutation pipeline, the editor, and the TUI.
- `pkg/jira/` — the Jira REST client and typed services.
- `pkg/adf/` — Atlassian Document Format parsing, validation, rendering.
- `tests/` — `contract`, `integration`, `unit`, and `guardrails` suites.

## Conventions

- Output is one `--output` flag: `auto`, `human`, `json`, or `compact`.
  Non-TTY and recognized agent environments emit JSON envelopes.
- Mutations route through the validate-and-encode pipeline. `--dry-run`
  is a local-only preview and never contacts Jira.
- `internal/tui/` is the persistent dashboard — leave it alone unless a
  change is explicitly about the TUI.
- Run `mise run ci` before proposing a change.
