# Epic

Browse and manage Jira epics — list them, add or remove child issues,
or jump to the agile board for an epic. `jira epic` is a thin
convenience layer on top of the underlying issue API; for the full
`epic` issue type's lifecycle (create / edit / transition) use the
[`issue`](issues.md) commands with `--type Epic`.

## list

Page through every epic visible to the active profile. The default
projection returns the summary fields (`status`, `summary`); pass
`--detail` for the full issue payload per epic.

```sh
jira epic list
jira epic list --detail
```

=== "Human"

    ```text
    INF ℹ️ listed epics detail=false epics="[5 items]" jql="issuetype = Epic"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": {
        "command": "epic.list",
        "timestamp": "…",
        "request_id": "…",
        "pagination": { "startAt": 0, "maxResults": 50, "total": 5, "isLast": true }
      },
      "data": {
        "detail": false,
        "jql": "issuetype = Epic",
        "epics": [
          {
            "id": "10000",
            "key": "SAM1-1",
            "self": "https://example.atlassian.net/rest/api/3/issue/10000",
            "fields": {
              "status": { "name": "To Do" },
              "summary": "Example epic summary"
            }
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

The result is also kept in the local epic cache so subsequent
`--epic <key>` resolutions on [`issue list`](issues.md#list) and
[`jql build`](jql.md#build) don't pay the round trip. See
[Cache › epics](cache.md#epics) for the cache mechanics.

## add

Attach an issue to a parent epic. The first positional argument is the
child issue, the second is the parent epic.

```sh
jira epic add SAM1-10 SAM1-1
jira epic add SAM1-10 SAM1-1 --dry-run
```

`--dry-run` skips the Jira call and echoes the resolved pair so you can
verify the wiring before committing. Child and parent must live in the
same Jira project; cross-project attaches are rejected upstream with
`validation` / `pid: "Issues with this Issue Type must be created in the
same project as the parent."`.

=== "Human"

    ```text
    INF ℹ️ added=true dry_run=false epic=SAM1-1 issue=SAM1-10
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": {
        "command": "epic.add",
        "timestamp": "…",
        "request_id": "…",
        "pagination": { "startAt": 0, "maxResults": 0, "total": 0, "isLast": true }
      },
      "data": {
        "added": true,
        "dry_run": false,
        "epic": "SAM1-1",
        "issue": "SAM1-10"
      },
      "errors": [],
      "warnings": []
    }
    ```

## remove

Detach an issue from its current epic. The issue stays in place; only
the epic link is cleared.

```sh
jira epic remove SAM1-10
jira epic remove SAM1-10 --dry-run
```

The call is idempotent — running `epic remove` against an issue that
isn't attached to any epic still returns `removed: true`.

=== "Human"

    ```text
    INF ℹ️ dry_run=false issue=SAM1-10 removed=true
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": {
        "command": "epic.remove",
        "timestamp": "…",
        "request_id": "…",
        "pagination": { "startAt": 0, "maxResults": 0, "total": 0, "isLast": true }
      },
      "data": {
        "removed": true,
        "dry_run": false,
        "issue": "SAM1-10"
      },
      "errors": [],
      "warnings": []
    }
    ```

## board

Open the agile board scoped to a specific epic in the system browser.
Interactive only — there's no JSON output.

```sh
jira epic board SAM1-1
```

## See also

*   [Issues](issues.md) — create an epic with `--type Epic`; transition or edit it via the issue commands.
*   [Cache › epics](cache.md#epics) — local epic-key cache used by `--epic <key>` filters.
*   [JQL](jql.md) — `issuetype = Epic` is the JQL behind `epic list`.
