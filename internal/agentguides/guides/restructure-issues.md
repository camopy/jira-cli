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
*   Move is intended to change an issue's project or type — but on Jira
    Cloud it currently cannot: the edit API silently ignores those
    fields, so a move exits 0 with an **empty** `data.result` and the
    issue unchanged. Treat an empty `data.result` as failure. To change
    project today, clone into the target project and delete the source.
*   Rank reorders backlog issues relative to an anchor issue.
*   Delete is permanent. There is no undo and no trash. If the issue has
    subtasks, Jira refuses unless you pass `--delete-subtasks` — which
    deletes those too. Prefer a "won't do" transition when history matters.

## Run

All three destructive ops (clone, move, delete) confirm with `--force`
headless. Move takes its target via `--json-input`, but see Decide — it
does not work on Jira Cloud today.

```sh
jira issue clone PROJ-123 --dry-run    # the preview lists exactly what is carried
jira issue clone PROJ-123 --no-input --force

jira issue rank PROJ-123 --before PROJ-9 --dry-run   # previews order and chunks

jira issue delete PROJ-999 --dry-run
jira issue delete PROJ-999 --no-input --force
```

## Save

*   From clone: the resulting issue lives at `data.result.key`;
    `data.issue` echoes the source. A move that "succeeds" with an empty
    `data.result` changed nothing.
*   From a delete preview: the exact set of issues that would go, subtasks
    included.

## Preconditions

Write permission on the target project (both projects, for move). All
three destructive ops need `--force` headless — by design, not an
obstacle.

## Recover

*   Delete refused over subtasks → decide whether the subtasks should die
    with the parent; only then add `--delete-subtasks`.
*   Deleted the wrong issue → it is gone. Recreate from the delete
    preview's data if you kept it; this is why the preview comes first.
*   Move exited 0 but the issue did not change → expected on Jira Cloud
    (the API ignores project/type on edit); use clone + delete instead.
*   Rank of >50 keys chunks transparently; on a mid-run failure the
    already-ranked chunks persist and the error says how many — resume
    with the remainder, not the full list.

## Next

*   `safe-mutation` — the gates in full; read it before your first delete.
*   `shape-issues` — recreating or adjusting what you restructured.
