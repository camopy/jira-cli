---
name: jira-cli
description: Use when working with matcra587/jira-cli for Jira issues, tickets, JQL/search, comments, transitions, worklogs, issue links, attachments, boards, auth/profile setup, 1Password-backed credentials, ADF, custom fields, JSON envelopes, or headless/agent Jira automation. Start from the embedded `jira agent` guide, schema, ADF matrix, and fieldtype registry instead of hand-written recipes.
allowed-tools: Bash(jira:*)
---

# jira-cli

This skill is a router into the binary's embedded agent references. Do not
duplicate command recipes here; the live binary is the source of truth.

## Start Here

Run the smallest `jira agent` reference that matches the task:

| Need | Command | Use it for |
| --- | --- | --- |
| Workflow guidance | `jira agent guide` | Cross-command contracts, per-workflow recipes, recovery notes |
| One workflow | `jira agent guide <section>` | Focused sections such as `core_contract`, `auth_setup`, `create_issue`, `edit_issue`, `safe_mutation`, `adf_reference` |
| Live command surface | `jira agent schema --output=compact` | Command paths, flags, mutexes, input schemas, output schemas |
| Rich text support | `jira agent adf-matrix --output=json` | ADF nodes/marks before authoring descriptions, comments, or worklog comments |
| Custom fields | `jira agent fieldtypes --output=json` | `customfield_NNNN` type handling before creating/editing custom fields |

If the guide and schema disagree, trust the live command/schema output and call
out the docs drift.

## Output Contract

Use the global `--output` flag, not legacy boolean output flags.

| Mode | Use |
| --- | --- |
| `--output=json` | Full machine envelope: `ok`, `meta`, `data`, `errors`, `warnings` |
| `--output=compact` | Success `data` only; failures still emit the full error envelope |
| `--output=human` | Human/clog output; structured agent subcommands print pretty JSON |
| `--output=auto` | TTY -> human, pipe -> JSON, detected agent -> compact |

Parse only `json` or `compact`. Do not parse human output.

## Operating Rules

*   Read `jira agent schema --output=compact` before constructing commands or
  payloads from memory.
*   Read `jira agent guide core_contract` before relying on exit codes, envelope
  shape, pagination, read-only behavior, or agent auto-detection.
*   For headless mutations, use `--no-input`; preview non-trivial writes with
  `--dry-run`; destructive operations still need `--force`.
*   For non-trivial writes, prefer `--json-input` over shell-quoted field flags.
*   ADF values go under the real Jira field name (`description`, `environment`,
  `customfield_NNNN`). There is no ADF suffix convention.
*   Read `jira agent fieldtypes --output=json` and refresh the field cache before
  writing custom fields.
*   Read `jira agent adf-matrix --output=json` before sending ADF that uses
  marks, panels, tables, cards, or extension nodes.
*   Use `jira jql build` or `jira issue list --as-jql --output=json` when
  composing JQL from user input.
*   For auth and 1Password behavior, read `jira agent guide auth_setup`; the
  backend uses the Go SDK and desktop-app authorization is per account and per
  process.

## Do Not

*   Do not keep local quick-reference tables of Jira commands in this skill.
*   Do not invent flags, payload fields, or backend fallback behavior.
*   Do not treat a successful dry run as proof Jira accepted the live write.
*   Do not bypass read-only mode with `--force`; read-only blocks mutations at
  the HTTP layer.
