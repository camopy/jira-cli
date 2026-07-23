---
slug: restructure-issues
title: Clone, move, rank, and delete issues
description: The structural operations — duplicating an issue, moving it between projects, reordering a backlog, and deleting.
when_to_use: Duplicating an issue as a template, moving work to another project, reordering a backlog, or removing an issue for good.
commands: [jira issue clone, jira issue move, jira issue rank, jira issue delete]
order: 8
---

## Decide

These four change where an issue lives rather than what it says. Pick with
care:

*   Clone copies an issue — cheap and reversible (delete the copy).
*   Move changes the original issue's project or type through a
    `--json-input` fields payload. Comments, worklogs, attachments, and
    watchers remain on the issue. Jira commonly answers the update with
    `204 No Content`, so an empty `data.result` is normal — verify the
    resulting project and type with `jira issue view`.
*   Rank reorders backlog issues relative to an anchor issue.
*   Delete is permanent. There is no undo and no trash. If the issue has
    subtasks, Jira refuses unless you pass `--delete-subtasks` — which
    deletes those too. Prefer a "won't do" transition when history matters.

## Run

Clone, move, and delete confirm with `--force` headless. Their dry-runs are
local-only: they validate caller-supplied payloads but do not fetch source
fields or enumerate server-side subtasks.

```sh
jira issue clone PROJ-123 --dry-run    # previews supplied overrides, not source fields
jira issue clone PROJ-123 --no-input --force

jira issue move PROJ-123 --json-input move.json --dry-run
jira issue move PROJ-123 --json-input move.json --no-input --force

jira issue rank PROJ-123 --before PROJ-9 --dry-run   # previews order and chunks

jira issue view PROJ-999              # preserve what you may need to recreate
jira issue delete PROJ-999 --dry-run
jira issue delete PROJ-999 --no-input --force
```

## Save

*   From clone: the resulting issue lives at `data.result.key`;
    `data.issue` echoes the source. A clone preview cannot show fields
    copied from the source because it makes no Jira request.
*   From move: `data.issue` keeps the original key. `data.result` may be
    empty after a successful 204 response, so save a verified readback.
*   Before delete: save a live `jira issue view` result if recreation
    might be necessary. The local preview confirms intent and input, not
    a restorable snapshot.

## Preconditions

Write permission on the target project (both projects, for move). All
three destructive ops need `--force` headless — by design, not an
obstacle.

## Recover

*   Delete refused over subtasks → decide whether the subtasks should die
    with the parent; only then add `--delete-subtasks`.
*   Deleted the wrong issue → it is gone. Recreate from a live read you
    saved before deletion; the dry-run does not fetch the issue.
*   Move returned an empty `data.result` → verify with
    `jira issue view`; the empty body alone is not a failure signal.
*   Rank of >50 keys chunks transparently; on a mid-run failure the
    already-ranked chunks persist and the error says how many — resume
    with the remainder, not the full list.

## Next

*   `safe-mutation` — the gates in full; read it before your first delete.
*   `shape-issues` — recreating or adjusting what you restructured.
