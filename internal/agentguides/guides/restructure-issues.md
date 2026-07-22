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
*   Move changes the issue's project; the key changes with it. Old links
    keep working through Jira redirects, but nothing you stored does.
*   Rank reorders backlog issues relative to an anchor issue.
*   Delete is permanent. There is no undo and no trash. If the issue has
    subtasks, Jira refuses unless you pass `--delete-subtasks` — which
    deletes those too. Prefer a "won't do" transition when history matters.

## Run

All three destructive ops (clone, move, delete) confirm with `--force`
headless. Move has no field flags — the target lives in a `--json-input`
payload.

```sh
jira issue clone PROJ-123 --dry-run    # the preview lists exactly what is carried
jira issue clone PROJ-123 --no-input --force

jira issue move PROJ-123 --json-input move.json --dry-run
jira issue move PROJ-123 --json-input move.json --no-input --force

jira issue rank PROJ-123 --before PROJ-9 --dry-run

jira issue delete PROJ-999 --dry-run
jira issue delete PROJ-999 --no-input --force
```

## Save

*   From clone and move: the resulting issue lives at `data.result.key`;
    `data.issue` echoes the source. After a move, update anything that
    stored the old key.
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
*   Move rejected → the target project's screens may lack required fields;
    the error hint names them.

## Next

*   `safe-mutation` — the gates in full; read it before your first delete.
*   `shape-issues` — recreating or adjusting what you restructured.
