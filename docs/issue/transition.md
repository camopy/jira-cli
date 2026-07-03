---
title: Transition & move
description: Advance an issue through its workflow with transition, and relocate it to another project with move.
icon: material/transit-connection-variant
---

# :twisted_rightwards_arrows: Transition & move

Two ways an issue changes hands: `transition` walks it through its workflow
(To Do → In Progress → Done), and `move` relocates it to a different project.
JSON examples below show the `data` block only — the envelope wrapper and exit
codes live on [Output](../output.md), and each command links to its reference
page for the complete flag and output-field tables.

## transition

Advance an issue to a new workflow status. Give the target as a trailing
positional (`"In Progress"`) or with `--transition`; either accepts a status
**name** or a numeric **transition id**. A name is resolved against *that*
issue's available transitions at runtime, so the same name can map to different
ids across workflows. With **no target**, `transition` lists the transitions
allowed from the current status instead of mutating — Jira filters that set by
source status and caller permissions, so listing first beats guessing.

```sh
jira issue transition PROJ-123
jira issue transition PROJ-123 "In Progress"
jira issue transition PROJ-123 --transition 21
jira issue transition PROJ-1..PROJ-5 Done -p 4 --output=json
```

1.  No target → list the transitions available from the current status.
2.  Target as a trailing positional; the name is resolved per issue.
3.  `--transition` takes the same value — a status name or a numeric id.

A transition can carry a payload, applied atomically with the status change:
`--markdown` / `--markdown-file` attach a comment, and `--json-input` submits
transition-screen fields (such as `resolution`) plus an optional ADF `comment`
key. This only works when the workflow's transition **has a screen**: Jira
silently discards payloads sent to a screenless transition (the norm in
team-managed projects), so the CLI refuses those with a validation error
rather than letting the content vanish — transition bare, then comment with
`issue comment add`.

```sh
jira issue transition PROJ-123 Done --markdown "released in v1.2.3"
jira issue transition PROJ-1..PROJ-5 Done -p 4 --markdown "released in v1.2.3"
```

The no-target form is a read; its `data` is the available-transition list (the
shape an agent picks an id from). `meta.command` is `issue.transitions` (plural)
for the list and `issue.transition` (singular) for an executed move:

```json
{
  "issue": "PROJ-123",
  "transitions": [
    { "id": "11", "name": "To Do" },
    { "id": "21", "name": "In Progress" },
    { "id": "31", "name": "Done" }
  ]
}
```

An executed transition returns just the resolved id: `{ "issue": "PROJ-123",
"transition": "21", "dry_run": false }`.

!!! warning "`--dry-run` is a local preview and never contacts Jira"
    Dry-run echoes the requested target back verbatim (`dry_run: true`) without
    resolving a name to an id — so it accepts any `--transition` value,
    including invalid ones. Only the live call validates against Jira's
    workflow.

### Transitioning many at once

`transition` accepts several keys — separate arguments, comma lists, or
inclusive ranges (`PROJ-1..PROJ-5`). Add `-p` / `--parallelism` to fan the calls
out (default `1`, max `16`). With a target, the same status name is re-resolved
per issue (an id can differ between workflow states) and applied to each;
without a target, each key gets its own available-transition list. Multiple keys
return ordered results under `data.results[]`, each with `ok` and either the
transition data or an `error`; if any key fails the command exits non-zero but
keeps the successes.

[Full flags & output fields →](../reference/jira/issue/transition.md)

## move

Relocate an issue to a different project, optionally changing its issue type on
the way. Unlike [`clone`](create-edit.md), `move` does not create a new issue:
comments, worklogs, attachments, and watchers travel with it. If the destination
project requires fields the source didn't carry, declare them in the same
`--json-input` payload alongside the project / type swap.

```sh
jira issue move PROJ-123 --json-input move.json --force
jira issue move PROJ-1..PROJ-5 -p 4 --json-input move.json --force
jira issue move PROJ-123 --json-input move.json --dry-run
```

The payload is a standard `fields` object — at minimum the target project, plus
the issue type if it changes:

```json
{ "fields": { "project": { "key": "OPS" }, "issuetype": { "name": "Task" } } }
```

A dry-run echoes the validated payload back as `data.payload.fields` and never
contacts Jira. A live submit returns `data.result` (often `{}` — Jira answers
the move with `204 No Content`) and the unchanged `data.issue` key:

```json
{
  "dry_run": true,
  "issue": "PROJ-123",
  "payload": {
    "fields": {
      "project":   { "key": "OPS" },
      "issuetype": { "name": "Task" }
    }
  }
}
```

!!! warning "`--force` is required for headless and bulk moves"
    A live move needs `--force` in headless, agent, or `--no-input` mode; an
    interactive terminal is prompted instead. A multi-key live move (`KEY...`)
    always requires `--force`, even in a TTY.

!!! info "Dry-run validation is optimistic"
    `--dry-run` runs the payload through the local validate-and-encode pipeline
    but skips Jira, so it can accept a payload the real submit rejects (for
    example a field the destination screen demands). Treat a clean dry-run as a
    shape check, not a guarantee.

[Full flags & output fields →](../reference/jira/issue/move.md)

## See also

*   [Reading issues](read.md) — find the key and check the current status first
*   [Creating & editing issues](create-edit.md) — `clone` to copy instead of relocate
*   [Output](../output.md) — the JSON envelope and exit codes
