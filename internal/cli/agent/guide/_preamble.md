# Jira CLI Agent Guide

Comprehensive steering document for AI coding assistants and developers
using `jira-cli`. This file is embedded in the binary and surfaced via
`jira agent guide`. Pair with `jira agent schema --output=compact` for
the machine-readable command tree, `jira agent adf-matrix --output=json`
for the ADF support matrix, and `jira agent fieldtypes --output=json`
for the customfield encoder registry.

The guide assumes you are interacting through the JSON envelope path
(`--output=json` or auto-detected agent mode); flags and recipes still
apply to TTY humans, the output rendering differs.

### How to read this guide

Each workflow below is a goal-oriented runbook. The layout is consistent:

- **Goal** — one sentence on the outcome.
- **When** — the trigger that should make an agent load this runbook (reference sections use **When to use this** instead).
- **Decide** — which flags / inputs to pick.
- **Run** — canonical command shapes.
- **Save** — the JSON fields you carry forward (requires `--output=json`).
- **Preconditions** — non-obvious constraints.
- **Behavior** — runtime quirks worth knowing.
- **Recover** — symptom → cause → next, including the exit code or error code.
- **Next** — which workflows naturally follow; `→ ` + a backticked slug
  points at another runbook in this guide.

Run `jira agent guide <slug>` to print a single workflow without the
rest of the file. The slug is the lowercased heading with `_` replaced
by `-` (so `## core_contract` is reachable as `core_contract` or
`core-contract`).

### Workflows

Cross-cutting:

- `core_contract` — output modes, envelope shape, exit codes, headless writes, pagination, read-only mode, debug logging.
- `identity_setup` — resolve and persist the active profile's account id.
- `auth_setup` — wire a profile + credential through one of the supported backends.
- `inspect_schema` — agent-facing schema / guide / ADF matrix / field-types surfaces.
- `configure_editor` — editor resolution for the bare-form `issue edit`.
- `safe_mutation` — the cross-cutting `--dry-run` / `--force` / `--no-input` contract every mutating workflow inherits.

Read:

- `read_issue` — `jira issue view KEY`.
- `list_issues` — `jira issue list`, board filtering, `--detail` vs `--full`.
- `search_jql` — `jira search jql`, `jira search saved`, `jira jql build`.

Discover:

- `discover_board` — `jira boards list` and the board cache primer.
- `cache_metadata` — per-profile JSON cache for labels / projects / fields / issuetypes / linktypes / boards / epics.

Create / mutate issues:

- `create_issue` — native ADF + alias-driven create.
- `create_subtask` — `issue_type: "Subtask"` + `parent.key`.
- `edit_issue` — field flags or `--json-input` (no editor under agent).
- `transition_issue` — list, then execute a workflow transition.

Side-channel writes:

- `add_comment` — `jira issue comment` (ADF preferred).
- `list_comments` — read / paginate / delete comments.
- `attach_file` — `jira issue attachment` add / list / download / delete.
- `manage_watchers` — `jira issue watchers` add / remove / list.
- `link_issues` — `jira issue link` (direction-aware), plus the link-type cache.
- `add_weblink` — remote URL attachments via `jira issue weblink`.
- `log_work` — `jira worklog add`.

Destructive:

- `clone_issue` — GET → sanitize → POST.
- `move_issue` — swap project / issuetype in place.
- `delete_issue` — `--force`-gated delete, with `--delete-subtasks` for parents.

Reference (not workflows — lookup material):

- `adf_reference` — ADF node / mark catalogue, gotchas, strict-vs-best-effort.
- `jql_reference` — JQL operators, keywords, functions, recipes.
