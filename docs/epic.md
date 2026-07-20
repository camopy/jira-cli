---
title: Epics
description: Browse and manage epics with epic list, add, remove, and board — child membership, the dry-run preview, and the status roll-up.
icon: material/rocket-launch-outline
---

# :rocket: Epics

Four verbs to work with epics: `list` them, `add` or `remove` child issues, and
`board` for a status roll-up. `jira epic` is a thin convenience layer over the
issue API; for the epic issue type's own lifecycle (create, edit, transition)
use the [`issue`](issue/read.md) commands with `--type Epic`. JSON examples below
show the `data` block only — the envelope and exit codes live on
[Output](output.md), and each command links to its reference page for the full
flag and output-field tables.

## list

Page through every epic visible to the active profile, each with its summary
fields (`status`, `summary`):

```sh
jira epic list
```

```json
{
  "detail": false,
  "jql": "issuetype = Epic",
  "epics": [
    {
      "id": "10000",
      "key": "PROJ-100",
      "self": "https://example.atlassian.net/rest/api/3/issue/10000",
      "fields": {
        "status": { "name": "To Do" },
        "summary": "Checkout rework"
      }
    }
  ]
}
```

The result is cached locally so later `--epic <key>` resolutions on
[`issue list`](issue/read.md#list) and [`jql build`](jql.md#build) skip the round
trip. See [Cache › epics](cache.md#the-list-primers) for the mechanics.

[Full flags & output fields →](reference/jira/epic/list.md)

## add

Attach one or more issues to a parent epic. The trailing argument is the epic;
everything before it is the child issue, list, or range. Add `-p` /
`--parallelism` to run the membership updates concurrently; multi-key output
uses ordered `data.results[]` entries.

```sh
jira epic add PROJ-123 PROJ-100
jira epic add PROJ-1..PROJ-10 PROJ-100 -p 4
jira epic add PROJ-123 PROJ-100 --dry-run
```

`--dry-run` skips the Jira call and echoes the resolved pair so you can check the
wiring first. Child and parent must live in the same project; a cross-project
attach is rejected upstream with `validation` / "Issues with this Issue Type must
be created in the same project as the parent."

```json
{
  "added": true,
  "dry_run": false,
  "epic": "PROJ-100",
  "issue": { "key": "PROJ-123" }
}
```

[Full flags & output fields →](reference/jira/epic/add.md)

## remove

Detach an issue from its current epic. The issue stays put; only the epic link
clears. Accepts issue-key lists and ranges with `-p` / `--parallelism`.

```sh
jira epic remove PROJ-123
jira epic remove PROJ-1..PROJ-10 -p 4
jira epic remove PROJ-123 --dry-run
```

The call is idempotent — `epic remove` against an issue with no epic still
returns `removed: true`.

```json
{
  "removed": true,
  "dry_run": false,
  "issue": { "key": "PROJ-123" }
}
```

[Full flags & output fields →](reference/jira/epic/remove.md)

## board

A compact board report: list epics and count each one's child issues by status —
a terminal summary instead of opening Jira in a browser. Takes no arguments.

```sh
jira epic board
```

It runs one child lookup per epic (capped by the epic list limit). The JSON
`data` carries per-epic rows under `data.epics` and the roll-up under
`data.totals`. With no profile configured it returns empty rows and zero totals.

[Full flags & output fields →](reference/jira/epic/board.md)

## See also

*   [Issues](issue/read.md) — create an epic with `--type Epic`; transition or edit it via the issue commands
*   [Cache › epics](cache.md#the-list-primers) — the local epic-key cache `--epic` filters use
*   [JQL](jql.md) — `issuetype = Epic` is the query behind `epic list`
*   [Output](output.md) — the JSON envelope and exit codes
</content>
