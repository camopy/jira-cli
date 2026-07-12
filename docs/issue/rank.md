---
title: Rank
description: Reorder backlog issues relative to an anchor issue — the web UI's drag-to-reorder, headlessly.
icon: material/sort
---

# :material-sort: Rank

`rank` moves issues in backlog order — the headless equivalent of dragging
rows in the web UI's backlog. Jira stores that order as *LexoRank*: every
issue carries an opaque, lexicographically sortable rank string, so any issue
can slot between two others without renumbering the rest — which is why
ranking is its own operation rather than an `edit`. JSON examples below show the `data`
block only; the envelope wrapper and exit codes live on
[Output](../output.md), and the [reference page](../reference/jira/issue/rank.md)
carries the complete flag table.

## rank

Place one or more issues immediately before or after an anchor issue. Exactly
one anchor is required: `--before` puts the issues above it, `--after` below
it. The issues keep the order you pass them in, and key lists and ranges
expand like every multi-key command.

```sh
jira issue rank PROJ-7 PROJ-9 --before PROJ-3
jira issue rank PROJ-20:24 --after PROJ-50 --output=json
```

```json
{
  "anchor": "PROJ-50",
  "position": "after",
  "order": ["PROJ-20", "PROJ-21", "PROJ-22", "PROJ-23", "PROJ-24"],
  "chunks": 1,
  "dry_run": false
}
```

More than 50 keys — the API's per-request cap — chunk transparently: each
later chunk anchors after the last key of the one before it, so the requested
order survives end-to-end. Chunks run sequentially, a failure halts the ones
after it, and the error says how many issues already ranked so you can resume
with the remainder.

`--dry-run` previews the submission order and chunk count locally and never
contacts Jira. Verify a live result with
`jira issue list --jql "project = PROJ ORDER BY Rank ASC"` — note Jira's
search index can trail a rank mutation by a few seconds.

Ranking is Jira Software board functionality: a project without a board (no
rank field) rejects it with the stable `rank_rejected` code, and a 207
partial success surfaces the refused issues as `rank_partial` with Jira's
per-issue reasons in the message.
