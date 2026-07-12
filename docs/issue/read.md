---
title: Reading issues
description: Read issues with view, list, and mine — filter flags, JQL, and the output shapes.
icon: material/book-open-variant
---

# :book: Reading issues

Three read verbs: `list` and `mine` to *find* issues, `view` to *read one* in
full. JSON examples below show the `data` block only — the envelope wrapper and
exit codes live on [Output](../output.md), and each command links to its
reference page for the complete flag and output-field tables.

## view

Read one issue when you already know its key. `view` returns everything `list`
drops: the ADF description, custom fields, and link, comment, and watcher
detail. To find the key first, use [`list`](#list) or [`mine`](#mine).

```sh
jira issue view PROJ-123
jira issue view PROJ-123 --web
jira issue view PROJ-1 PROJ-2 PROJ-3 -p 4 --output=json
```

Human output prints the summary, status, assignee, and priority, a one-line
`transitions:` list of the valid workflow moves (`Name (id)`), then the
rendered description. The JSON `data` is the raw Jira REST shape:

```json
{
  "issue": {
    "id": "10401",
    "key": "PROJ-123",
    "fields": {
      "summary": "Checkout returns 500 on empty cart",
      "status": { "name": "In Progress" },
      "priority": { "name": "High" },
      "assignee": { "accountId": "712020:…", "displayName": "John Doe" },
      "description": { "type": "doc", "version": 1, "content": [ … ] },
      "customfield_10019": "0|i0007r:",
      "customfield_10001": null
    },
    "transitions": [
      { "id": "21", "name": "In Progress", "hasScreen": false }
    ],
    "editmeta": {
      "fields": {
        "priority": {
          "name": "Priority",
          "required": false,
          "operations": ["set"],
          "schema": { "type": "priority" },
          "allowedValues": [ { "id": "1", "name": "Highest" } ]
        }
      }
    }
  }
}
```

Both `transitions` (the workflow moves valid from the current status) and
`editmeta.fields` (editable fields with their `required` flag, `operations`, and
`allowedValues`) ride the same read at no extra call — one `view` answers "what
can I transition to" and "what may I edit" without a follow-up request.

!!! warning "Raw Jira passthrough"
    `data.issue.fields` is Jira's own field shape: custom fields appear by their
    stable IDs (`customfield_10019`, …) with no label resolution. Human output
    applies the labels you'd see in the web UI; JSON consumers map the IDs
    themselves. The [field-type reference](../custom-fields.md) lists the common
    ones.

### Reading many at once

`view` accepts several keys — separate arguments, comma lists, or inclusive
ranges (`PROJ-1..PROJ-5`). Add `-p` / `--parallelism` to fan the reads out
(default `1`, max `16`). A single key keeps the `data.issue` shape above; multiple
keys return ordered results under `data.results[]`, each with `ok` and either
`issue` or `error`. If any key fails the command exits non-zero but keeps the
successful reads:

```json
{
  "results": [
    { "key": "PROJ-1", "ok": true, "issue": { "key": "PROJ-1", "fields": { "…": "…" } } },
    { "key": "PROJ-9", "ok": false, "error": { "type": "not_found", "message": "issue does not exist" } }
  ],
  "succeeded": 1,
  "failed": 1
}
```

[Full flags & output fields →](../reference/jira/issue/view.md)

## list

Filter by project, key, assignee, reporter, status, priority, label, type, epic,
board, or date, and `list` builds the JQL for you and runs it. Drop or add flags
until the result is right, then reuse the resolved query with `--jql`. This is
the main terminal triage workflow.

```sh
jira issue list --project PROJ --status "In Progress"
jira issue list --project PROJ --status '!Done' --order-by created
jira issue list --updated -7d --type Bug --assignee me
jira issue list --jql "status = Done AND assignee = currentUser()" --count
jira issue list --project PROJ --status Done --as-jql --output=json
```

The default projection is a summary table, one row per issue:

```text
INF ℹ️ Listed issues count=2
KEY       SUMMARY                             STATUS       ASSIGNEE    PRIORITY
PROJ-123  Checkout returns 500 on empty cart  In Progress  John Doe    High
PROJ-118  Flaky login redirect                To Do        unassigned  Medium
```

Each `data.issues[]` row carries the summary fields plus `status_category`
(Jira's workflow category — `new`, `indeterminate`, or `done`) and
`status_color` (the category colour used to tint the human output):

```json
{
  "board_scope": { "applied": false },
  "detail": false,
  "issues": [
    {
      "key": "PROJ-123",
      "summary": "Checkout returns 500 on empty cart",
      "status": "In Progress",
      "status_category": "indeterminate",
      "status_color": "yellow",
      "priority": "High",
      "assignee": null,
      "updated": "2026-06-01T22:00:29.281-0400"
    }
  ]
}
```

A few flags change what `list` does rather than what it filters:

*   `--as-jql` builds and prints the JQL **without calling Jira** (`meta.command`
  becomes `issue.list.jql`, `data.issues` is always `[]`). Use it to confirm a
  filter resolves the way you expect before a heavy read.
*   `--count` asks Jira for an approximate match count and fetches no issues — a
  fast "how many?" Human output is the bare number; JSON carries `count`. It
  calls Jira, so it can't combine with the offline `--as-jql`.
*   `--key` narrows to known keys (`PROJ-1`, comma lists, `PROJ-1..PROJ-10`
  ranges). Large key sets chunk and tolerate gaps; add `-p N` to run the chunks
  concurrently.
*   `--limit` / `--all` / `--cursor` / `--unbounded` page the built query with
  the same contract as [`search jql`](../search.md#pagination): one page of
  `--limit` (default 50) with `meta.pagination.nextCursor` to resume via
  `--cursor`, or `--all` to drain (bounded; `--unbounded` lifts the caps). A
  `--key` listing is looked up whole, so it refuses the pagination flags.

!!! warning "`--detail` is heavy"
    `--detail` returns the full `view` payload — description ADF, every custom
    field — for *every* match. Reach for it only when a consumer needs the full
    record, and pair it with a tight `--project` or `--jql`.

Date flags (`--updated`, `--created`, `--resolved`) take a relative window
(`-7d`), an absolute date (`2026-01-01`), a comparator (`>=2026-01-01`), or an
inclusive `A..B` range. A bare value is a lower bound. The full grammar is on the
[JQL](../jql.md) page.

[Full flags & output fields →](../reference/jira/issue/list.md)

## mine

What's assigned to you right now — `list` with the assignee pinned to the current
user. It shares `list`'s runner and output shape, and takes the same filter,
sort, and date flags, *except* `--assignee` and `--reporter` (assignee is the
point). There's no `--count` or `-p` here; reach for `list` when you need those.

```sh
jira issue mine
jira issue mine --status "In Progress" --priority High
jira issue mine --label regression --type Bug
jira issue mine --project PROJ --as-jql --output=json
```

Output is identical to [`list`](#list) — same table, same `data.issues[]` shape.

[Full flags & output fields →](../reference/jira/issue/mine.md)

## See also

*   [Creating & editing issues](create-edit.md) — once you've found the issue
*   [JQL](../jql.md) — the query language behind the filter flags
*   [Output](../output.md) — the JSON envelope and exit codes
*   [Search](../search.md) — raw and saved queries
