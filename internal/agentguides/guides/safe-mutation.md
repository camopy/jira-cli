---
slug: safe-mutation
title: Preview every write before sending it
description: The write discipline — dry-run first, gate flags for headless runs, then verify what the server actually applied.
when_to_use: Before any create, edit, transition, delete, or other mutation, and when a write is refused by a gate.
commands: []
order: 5
---

## Decide

Every mutation follows the same sequence: **preview → submit → verify**.
Skipping the preview trades a free local check for a live write you may
have to undo.

Know which gate applies before running:

*   `--dry-run` — on every mutation, local-only, never contacts Jira. It
    validates and encodes the full payload through the same pipeline as a
    live write and prints what would be sent, with `data.dry_run: true`.
*   `--validate-remote` — with `--dry-run`, additionally resolves the
    payload against live Jira (create/edit screens, transition lists,
    watcher targets). Read-only; still writes nothing. The schema shows
    which commands carry it.
*   `--no-input` — required for any headless mutation (implied outside a
    TTY, but pass it explicitly).
*   `--force` — additionally required for destructive commands: Jira ones
    (delete, clone, move, comment/attachment/link delete) and local-state
    ones (`cache clear`, `alias delete`). Never scripted around; the
    refusal is the contract working.
*   `JIRA_READ_ONLY=1` — blocks all writes at the transport; useful as a
    belt-and-braces guard for exploratory sessions.

## Run

The shape, using edit as the example:

```sh
jira issue edit PROJ-123 --summary "New title" --dry-run   # preview
jira issue edit PROJ-123 --summary "New title" --no-input  # submit
jira issue view PROJ-123                                   # verify
```

Create and edit also take `--verify`, which re-fetches after the live write
and warns about requested fields the server silently dropped.

## Save

*   From the preview: the encoded payload — confirm fields resolved the way
    you intended before submitting.
*   From the live write: `data.dry_run: false` plus the issue identity;
    multi-key runs return per-key `data.results[]`.
*   From `--verify`: warnings naming any field Jira ignored.

## Preconditions

An authenticated profile with write permission on the target project, and
`JIRA_READ_ONLY` unset.

## Recover

*   Refused with a force/input hint (exit 3) → add the named gate flag
    after confirming intent; the refusal is by design.
*   Preview passed but the live write failed → the server rejected what the
    local pipeline could not know (permissions, screen config); the error
    `hint` names the next step. Re-run with `--dry-run --validate-remote`
    to see the server-side view.
*   Partial multi-key failure → successful keys are in `data.results[]`;
    fix and re-run only the failed keys.

## Next

*   `shape-issues`, `annotate-issues`, `restructure-issues` — the mutation
    families this discipline applies to.
*   `core-contract` — gate flags and envelope fields in full.
