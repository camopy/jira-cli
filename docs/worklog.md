# Worklog

Record and inspect time-tracking entries on a Jira issue. The two
subcommands (`add`, `list`) share the standard envelope and `--output`
modes. `worklog add` accepts the same `--dry-run` preview path as the
issue mutations.

## add

Log time against an issue. `--time-spent` accepts compact Jira-style
durations (`1h30m`, `45m`, `2d`). `1d` resolves via the profile's
`workday_seconds` (default `28800` = 8h); set it under `[profiles.<name>]`
in the config if your team's workday differs.

```sh
jira worklog add JCT-32 --time-spent "1h30m" --comment-markdown "Investigating regression"
jira worklog add JCT-32 --time-spent "2h" --started "2026-05-27T09:00:00.000-0400"
jira worklog add JCT-32 --json-input worklog.json
```

!!! warning "Duration format is space-free"
    `--time-spent "1h 30m"` (with a space) fails with `invalid duration`.
    Use the compact form `"1h30m"`. Fractional units (`"1.5h"`) are
    rejected as `unsupported duration unit`; combine integer units
    instead.

=== "Human"

    ```text
    INF ℹ️ added worklog dry_run=false issue=JCT-32 worklog={...}
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": {
        "command": "worklog.add",
        "timestamp": "…",
        "request_id": "…",
        "pagination": { "startAt": 0, "maxResults": 0, "total": 0, "isLast": true }
      },
      "data": {
        "dry_run": false,
        "issue": "JCT-32",
        "worklog": {
          "id": "10169",
          "timeSpentSeconds": 60,
          "started": "2026-05-27T17:28:53.802-0400",
          "comment": {
            "type": "doc", "version": 1,
            "content": [
              { "type": "paragraph", "content": [{ "type": "text", "text": "Investigating regression" }] }
            ]
          }
        }
      },
      "errors": [],
      "warnings": []
    }
    ```

!!! note "Dry-run vs submit field naming differs"
    The dry-run preview returns `time_spent_seconds` (snake_case) while a
    real submit returns `timeSpentSeconds` (camelCase). Scripts that
    diff dry-run output against the persisted record need to handle
    both keys until that asymmetry is reconciled upstream.

### Dry-run

`--dry-run` runs the full validation pipeline (duration parsing, ADF
encoding of the comment) but stops before the worklog POST. The resolved
payload echoes under `data.worklog` so you can verify the converted
duration before committing.

```json
{
  "ok": true,
  "meta": { "command": "worklog.add", "timestamp": "…", "request_id": "…" },
  "data": {
    "dry_run": true,
    "issue": "JCT-32",
    "worklog": {
      "time_spent_seconds": 5400,
      "started": "",
      "comment": { "type": "doc", "version": 1, "content": [ … ] }
    }
  },
  "errors": [],
  "warnings": []
}
```

### JSON input

Pass `--json-input <file>` (or `--json-input -` for stdin) to author the
payload directly. The duration goes under `time_spent` as a Jira-style
string (same grammar as `--time-spent`), not as a seconds integer:

```json
{
  "time_spent": "1h30m",
  "started": "2026-05-27T09:00:00.000-0400",
  "comment": {
    "type": "doc", "version": 1,
    "content": [
      { "type": "paragraph", "content": [{ "type": "text", "text": "Investigating regression" }] }
    ]
  }
}
```

For the comment body's ADF shape see [ADF](adf.md).

## list

Page through every worklog on an issue. Returns an empty array when no
time has been logged.

```sh
jira worklog list JCT-32
```

=== "Human"

    ```text
    INF ℹ️ listed worklogs issue=JCT-32 worklogs="[3 items]"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": {
        "command": "worklog.list",
        "timestamp": "…",
        "request_id": "…",
        "pagination": { "startAt": 0, "maxResults": 0, "total": 0, "isLast": true }
      },
      "data": {
        "issue": "JCT-32",
        "worklogs": [
          {
            "id": "10169",
            "timeSpentSeconds": 60,
            "started": "2026-05-27T17:28:53.802-0400",
            "comment": {
              "type": "doc", "version": 1,
              "content": [
                { "type": "paragraph", "content": [{ "type": "text", "text": "Investigating regression" }] }
              ]
            }
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

Pagination meta is populated even on a single-worklog return; the
`total` and `maxResults` fields don't reflect the array length.

## See also

*   [Agents › `log_work`](agent.md#guides) — the agent-runbook view of the same flow.
*   [ADF](adf.md) — comment body shape for `--json-input`.
*   [Configuration](config.md) — `workday_seconds` per profile.
