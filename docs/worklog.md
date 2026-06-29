---
title: Worklog
description: Record and inspect time-tracking entries with worklog add and worklog list — duration formats, the dry-run preview, and JSON input.
icon: material/timer-outline
---

# :stopwatch: Worklog

Two verbs: `add` logs time against an issue, `list` pages through what's already
logged. Both take issue-key lists and ranges with `-p` / `--parallelism`, and
`add` runs the same `--dry-run` preview as the issue mutations. JSON examples
below show the `data` block only — the envelope and exit codes live on
[Output](output.md), and each command links to its reference page for the full
flag and output-field tables.

## add

Log time against an issue. `--time-spent` takes compact Jira-style durations
(`1h30m`, `45m`, `2d`). `1d` resolves via the profile's `workday_seconds`
(default `28800` = 8h); set it under `[profiles.<name>]` if your team's workday
differs.

```sh
jira worklog add PROJ-123 --time-spent "1h30m" --comment-markdown "Investigating regression"
jira worklog add PROJ-1..PROJ-10 -p 4 --time-spent "15m" --comment-markdown "Bulk triage"
jira worklog add PROJ-123 --time-spent "2h" --started "2026-05-27T09:00:00.000-0400"
jira worklog add PROJ-123 --json-input worklog.json --dry-run
```

A live add returns `data.worklog` with Jira's `id`, `timeSpentSeconds`,
`started`, and the ADF `comment`. A `--dry-run` runs the full pipeline (duration
parsing, ADF encoding) but stops before the POST, echoing the resolved payload
so you can check the converted duration first:

```json
{
  "dry_run": true,
  "issue": "PROJ-123",
  "worklog": {
    "time_spent_seconds": 5400,
    "started": "",
    "comment": { "type": "doc", "version": 1, "content": [ … ] }
  }
}
```

!!! warning "Duration format is space-free"
    `--time-spent "1h 30m"` (with a space) fails with `invalid duration`. Use
    the compact `"1h30m"`. Fractional units (`"1.5h"`) are rejected as
    `unsupported duration unit` — combine integer units instead.

!!! note "Dry-run and submit name the duration field differently"
    The dry-run preview returns `time_spent_seconds` (snake_case); a real submit
    returns `timeSpentSeconds` (camelCase). A script that diffs dry-run output
    against the persisted record needs to handle both keys until that asymmetry
    is reconciled upstream.

To author the payload directly, pass `--json-input <file>` (or `--json-input -`
for stdin). The duration goes under `time_spent` as a Jira-style string (same
grammar as `--time-spent`), not a seconds integer:

```json
{
  "time_spent": "1h30m",
  "started": "2026-05-27T09:00:00.000-0400",
  "comment": {
    "type": "doc",
    "version": 1,
    "content": [
      { "type": "paragraph", "content": [{ "type": "text", "text": "Investigating regression" }] }
    ]
  }
}
```

For the comment body's ADF shape see [ADF](adf.md).

[Full flags & output fields →](reference/jira/worklog/add.md)

## list

Page through every worklog on an issue — an empty array when no time has been
logged. A single key keeps the `data.issue` shape below; multiple keys return
ordered `data.results[]` entries, each with `key`, `ok`, and either per-key
`data` or `error`.

```sh
jira worklog list PROJ-123
jira worklog list PROJ-1..PROJ-10 -p 4
```

```json
{
  "issue": "PROJ-123",
  "worklogs": [
    {
      "id": "10169",
      "timeSpentSeconds": 60,
      "started": "2026-05-27T17:28:53.802-0400",
      "comment": { "type": "doc", "version": 1, "content": [ … ] }
    }
  ]
}
```

Pagination meta rides along even on a single-worklog return; its `total` and
`maxResults` don't reflect the array length.

[Full flags & output fields →](reference/jira/worklog/list.md)

## See also

*   [Agents › `log_work`](agent.md#guides) — the agent-runbook view of this flow
*   [ADF](adf.md) — the comment body shape for `--json-input`
*   [Configuration](config.md) — `workday_seconds` per profile
*   [Output](output.md) — the JSON envelope and exit codes
</content>
