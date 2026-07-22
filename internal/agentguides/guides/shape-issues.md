---
slug: shape-issues
title: Create and update issues
description: Create issues and subtasks, edit fields, and transition status — choosing the right body source each time.
when_to_use: Creating an issue or subtask, changing summary, description, assignee or custom fields, or moving an issue to a new status.
commands: [jira issue create, jira issue edit, jira issue transition]
order: 6
---

## Decide

Pick the body source first:

*   Simple fields → flags (`--summary`, `--type`, `--assignee me`,
    `--label`, `--priority`, `--parent`).
*   Rich description → `--markdown` / `--markdown-file` (converted to ADF,
    lossy by design).
*   Custom fields or full control → `--json-input file.json`, the
    canonical agent path. Field ids and encodings come from `discover`.

A subtask is `jira issue create --parent KEY` with a subtask type; an epic
child is the same flag with an epic parent. For transitions, resolve the
legal target names from the issue itself before executing; resolve/close
transitions often require a `resolution` field (`jira cache resolutions`
lists the names).

## Run

```sh
# Create: preview, then submit
jira issue create --project PROJ --type Task --summary "Fix the flaky test" \
  --markdown "Steps in the CI log." --dry-run
jira issue create --project PROJ --type Task --summary "Fix the flaky test" \
  --markdown "Steps in the CI log." --no-input

# Edit one field
jira issue edit PROJ-123 --assignee me --no-input

# Transition: --dry-run --validate-remote resolves the target against the
# issue's live transitions without writing
jira issue transition PROJ-123 "In Review" --dry-run --validate-remote
jira issue transition PROJ-123 "In Review" --no-input
```

A transition can carry an atomic comment via `--markdown`.

## Save

*   From create: `data.issue.key` — the handle every later step needs.
*   From a `--verify` run: warnings listing fields the server dropped.
*   From a transition preview: the resolved transition id and name.

## Preconditions

Exact type, status, and field names for the tenant (`discover`), and the
preview discipline from `safe-mutation`.

## Recover

*   Unknown type or field (exit 3) → names are tenant-specific; refresh
    with `jira cache refresh --force` and match exactly. If the type is
    valid instance-wide but still refused, it is missing from that
    project's create screen — `--dry-run --validate-remote` shows what
    the screen accepts.
*   Transition refused → the workflow forbids that hop from the current
    status; the validate-remote preview lists what is legal.
*   Transition with a comment or fields refused (exit 3) → that
    transition has no screen (the norm in team-managed projects), so it
    cannot carry a payload. Transition bare, then
    `jira issue comment add`.
*   `--assignee <email>` refused under `--dry-run` → resolving an email
    needs a live lookup; pass `accountId:<id>` to preview.
*   Editing with no field flags opens `$EDITOR` on a TTY and refuses
    headless (exit 3) — always pass explicit fields when scripting.

## Next

*   `safe-mutation` — the preview/submit/verify sequence in full.
*   `write-rich-text` — when Markdown conversion is not enough.
*   `annotate-issues` — comments, links, and other attachments to the
    issue you shaped.
