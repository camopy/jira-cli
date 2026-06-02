# Issues

Every `jira issue` subcommand emits the same envelope shape and respects the
same `--output {human,json,compact}` modes. The examples below are real
runs against a live tenant with placeholders applied (display names,
account IDs, sites). Destructive paths (`delete`, `clone`, `move`,
`comment delete`, `link delete`, `attachment delete`) require `--force`
under `--no-input`.

## view

Read a single issue when you already know its key. Returns everything
[`list`](#list) drops: ADF description, comment thread, custom fields,
link / watcher / attachment counts. To find the key first, use
[`list`](#list) or [`mine`](#mine).

```sh
jira issue view <ISSUE_KEY>
```

=== "Human"

    ```text
    <ISSUE_KEY>  Example issue summary
      status: To Do  priority: Medium  assignee: unassigned
      Example description body, rendered from ADF.
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.view", "timestamp": "…", "request_id": "…" },
      "data": {
        "issue": {
          "id": "10401",
          "key": "<ISSUE_KEY>",
          "self": "https://example.atlassian.net/rest/api/3/issue/10401",
          "fields": {
            "summary": "…",
            "status": { "name": "To Do" },
            "priority": { "name": "Medium" },
            "reporter": {
              "accountId": "712020:00000000-0000-4000-8000-000000000000",
              "displayName": "John Doe",
              "emailAddress": "john.doe@example.com",
              "active": true
            },
            "description": { "type": "doc", "version": 1, "content": [ … ] },
            "comment": {},
            "worklog": {},
            "customfield_10019": "0|i0007r:",
            "customfield_10001": null
          }
        }
      },
      "errors": [],
      "warnings": []
    }
    ```

!!! warning "Raw Jira passthrough"
    `data.issue.fields` is the raw Jira REST shape: custom fields appear
    by their stable IDs (`customfield_10019`, …) with no label resolution.
    The human output applies the same labels you'd see in the web UI;
    JSON consumers need to map the IDs themselves.

### Multiple keys

`issue view` accepts more than one key. Pass separate arguments, comma
lists, or inclusive ranges, and add `-p` / `--parallelism` when the
read can safely fan out. Parallelism defaults to `1` and is capped at
`16`.

```sh
jira issue view <ISSUE_KEY> <OTHER_ISSUE_KEY> --output=json
jira issue view <PROJECT_KEY>-1:5 -p 4 --output=json
```

Single-key reads keep the existing `data.issue` payload. Multi-key reads
return ordered per-key results under `data.results[]`; each entry has
`ok: true` plus `issue`, or `ok: false` plus `error`. If one key fails,
the command exits non-zero and writes the standard error envelope to
stderr while preserving successful keys in `data.results[]`; stdout is
empty for JSON failure envelopes. Human output keeps the
success table on stdout; failed-key diagnostics go to stderr and are
truncated for large batches. Use `--output=json` for the full per-key
failure list.

```json
{
  "ok": false,
  "meta": { "command": "issue.view", "exit_code": 2, "timestamp": "...", "request_id": "..." },
  "data": {
    "results": [
      {
        "key": "<ISSUE_KEY>",
        "ok": true,
        "issue": { "id": "10401", "key": "<ISSUE_KEY>", "fields": { "summary": "..." } }
      },
      {
        "key": "<OTHER_ISSUE_KEY>",
        "ok": false,
        "error": {
          "type": "not_found",
          "code": "not_found",
          "message": "issue does not exist",
          "retryable": false
        }
      }
    ],
    "succeeded": 1,
    "failed": 1
  },
  "errors": [
    {
      "type": "not_found",
      "code": "not_found",
      "message": "issue does not exist",
      "retryable": false
    }
  ],
  "warnings": []
}
```

### Issue-key expansion support

The commands below accept issue-key lists and ranges such as
`<PROJECT_KEY>-1..10` or `<PROJECT_KEY>-1:10`. Add `-p N` /
`--parallelism N` to run up to `N` independent Jira requests at once
(`1` by default, `16` maximum). Single-key calls keep their existing
payload shape; multi-key calls return ordered `data.results[]` entries
with `key`, `ok`, and either command-specific `data` or `error`.
Expanded key sets are capped at 1000 keys and exit with a flag error
before credentials, network calls, or dry-run mutation work when that
cap is exceeded.

| Command | Expanded argument | Parallelism | Notes |
| --- | --- | --- | --- |
| `jira issue view KEY...` | positional issue keys | per issue GET | Full issue payloads; failures are per key. |
| `jira issue list --key KEY...` | `--key` values | per search chunk | Sparse ranges return visible issues; chunk failures preserve successful chunks. |
| `jira issue edit KEY...` | positional issue keys | per issue edit pipeline | Explicit field/json edits only; the bare editor flow remains single-key. |
| `jira issue clone KEY...` | positional source issue keys | per source clone | Applies the same override payload to each source. |
| `jira issue move KEY...` | positional issue keys | per issue move | Applies the same move payload to each issue. |
| `jira issue delete KEY...` | positional issue keys | per issue delete | Multi-key live delete requires `--force`; `--dry-run` previews every key. |
| `jira issue attachment add KEY...` | positional issue keys | per issue upload | Same files are uploaded to each issue; use `--file` for unambiguous multi-key uploads. |
| `jira issue attachment list KEY...` | positional issue keys | per issue attachment read | `download` and `delete` remain single-target because they take an attachment id. |
| `jira issue comment add KEY...` | positional issue keys | per issue comment POST | Same body and visibility are applied to each issue. |
| `jira issue comment list KEY...` | positional issue keys | per issue comment read | Per-key warnings are included in that result's `data.warnings`. |
| `jira issue link KEY... --to KEY --type NAME` | positional inward issue keys | per link create | `--to` stays a single target; link delete remains single-target because it takes a link id. |
| `jira issue link list KEY...` | positional issue keys | per issue link read | Lists existing links for each issue. |
| `jira issue weblink KEY...` | positional issue keys | per web-link create | Same URL/title are attached to each issue. |
| `jira issue watch KEY...` / `unwatch KEY...` | positional issue keys | per watcher mutation | Adds/removes the current user on each issue. |
| `jira issue watchers add/remove KEY...` | positional issue keys | per watcher mutation | Same `--user` is resolved once and applied to each issue. |
| `jira issue watchers list KEY...` | positional issue keys | per watcher read | Lists watcher state for each issue. |
| `jira issue transition KEY...` | positional issue keys | per transition read or execution | With `--transition`, applies the same transition id to each issue. |
| `jira worklog add KEY...` | positional issue keys | per worklog POST | Same worklog payload is added to each issue. |
| `jira worklog list KEY...` | positional issue keys | per worklog read | Lists worklogs for each issue. |
| `jira epic add ISSUE_KEY... EPIC_KEY` | positional issue keys | per epic membership update | Adds each issue to one epic. |
| `jira epic remove ISSUE_KEY...` | positional issue keys | per epic membership update | Removes each issue from its epic. |

Commands that combine an issue key with a single secondary id remain
single-key by design: comment edit/delete, attachment download/delete,
and link delete. The secondary id identifies one Jira object, so ranges
would be ambiguous rather than a safe fan-out.

## list

Filter issues by project, issue key, assignee, status, priority,
label, type, epic, board, or date (`--updated` / `--created` /
`--resolved`). The binary translates the flags into JQL and runs it.
Add or drop flags until the result is what you want, then save the
resolved query as `--jql` for the next run. `--as-jql` prints the
query without calling Jira, useful when a flag like `--label foo`
might not resolve the way you expect.

The default projection is a summary table, one row per issue. Pass
`--detail` for full per-issue records (same shape as
[`view`](#view)).

!!! warning "Common mistake"

    `--detail` without filters returns the full Jira issue object
    (description ADF, comment thread, every customfield) for every
    match. Default summary projection is much lighter — reach for
    `--detail` only when a downstream consumer needs the full
    payload, and pair it with a tight `--project` / `--jql`.

```sh
jira issue list --project <PROJECT_KEY> --status "To Do" --order-by updated --desc
jira issue list --key <PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12 --as-jql
jira issue list --key <PROJECT_KEY>-1:100,<OTHER_PROJECT_KEY>-1:200 -p 15
jira issue list --project <PROJECT_KEY> --updated=-7d         # changed in the last 7 days
jira issue list --project <PROJECT_KEY> --created 2026-01-01..2026-02-01
jira issue list --project <PROJECT_KEY> --as-jql              # show the JQL only
jira issue list --jql 'project = <PROJECT_KEY> ORDER BY updated DESC'
jira issue list --board "Engineering" --detail
```

Date flags accept a relative window (`-7d`, signed, Jira units
`w`/`d`/`h`/`m`), an absolute `YYYY-MM-DD`, an explicit comparator
(`>=2026-01-01`, `<2026-02-01`), or an inclusive `..` range
(`2026-01-01..2026-02-01`, open-ended as `2026-01-01..` or
`..2026-02-01`). A bare value is a lower bound (`>=`). See
[JQL → Date filters](jql.md#date-filters) for the full grammar.

=== "Human"

    ```text
    INF ℹ️ listed issues count=2 detail=false
    KEY       SUMMARY                STATUS       ASSIGNEE    PRIORITY
    <ISSUE_KEY>    Example issue summary  In Progress  unassigned  Medium
    <OTHER_ISSUE_KEY>    Example issue summary  To Do        unassigned  Medium
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.list", "timestamp": "…", "request_id": "…" },
      "data": {
        "board_scope": { "applied": false },
        "detail": false,
        "issues": [
          {
            "key": "<ISSUE_KEY>",
            "summary": "Example issue summary",
            "status": "In Progress",
            "priority": "Medium",
            "assignee": null,
            "updated": "2026-06-01T22:00:29.281-0400"
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

=== "Human"

    ```text
    INF ℹ️ listed issues count=3 detail=false jql="..."
    KEY     SUMMARY                                                     STATUS  ASSIGNEE    PRIORITY
    <ISSUE_KEY_3>  auth status: top-level ok:true and errors:[] despite …      To Do   unassigned  Medium
    <OTHER_ISSUE_KEY>  issue view --output json: raw Jira passthrough …            To Do   unassigned  Medium
    <ISSUE_KEY>  auth login: hint field empty on 401; remediation …          To Do   unassigned  Medium
    ```

=== "JSON"

    The default projection is summary-only; pass `--detail` to receive
    the same shape as `issue view` for every issue.

    ```json
    {
      "ok": true,
      "meta": {
        "command": "issue.list",
        "timestamp": "…",
        "request_id": "…",
        "pagination": { "startAt": 0, "maxResults": 50, "total": 0, "isLast": true }
      },
      "data": {
        "board_scope": { "applied": false },
        "detail": false,
        "issues": [
          {
            "key": "<ISSUE_KEY_3>",
            "summary": "Example issue summary",
            "status": "To Do",
            "priority": "Medium",
            "assignee": null,
            "updated": "2026-05-27T07:12:38.839-0400"
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

`--as-jql` builds and prints the JQL without calling Jira:
`meta.command` flips to `issue.list.jql` and `data.issues` is always
`[]`. Useful for confirming the filter flags resolve to the query you
expect before running a heavy `--detail` pull.

`--key` narrows the generated JQL to known issue keys. It accepts a
single key (`<ISSUE_KEY>`), comma lists
(`<ISSUE_KEY>,<OTHER_ISSUE_KEY>`), repeated flags, and inclusive
ranges (`<PROJECT_KEY>-1:10`, `<PROJECT_KEY>-1..10`). Comma members
are expanded independently, so
`<PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12` is valid. A single range
must stay within one project prefix:
`<PROJECT_KEY>-1:<OTHER_PROJECT_KEY>-100` exits with a flag error
instead of crossing projects. Do not put spaces inside a `--key`
value. Expanded key sets are capped at 1000 keys.

When `--key` expands to a large set, add `-p N` / `--parallelism N`
to split the key set into bounded search chunks and run up to `N`
requests concurrently. This keeps `issue list` tolerant of sparse
ranges: Jira returns the visible existing issues, rather than a per-key
error record for every missing key. Chunked `--key` output is ordered by
the requested keys, not by Jira's returned order. If one chunk request
fails, successful chunks are retained in the error envelope and JSON
adds `data.failed_key_chunks[]`; the command still exits non-zero. Use
[`issue view`](#view) with `-p` when you need full per-key success/error
results.

## mine

Shows what's assigned to you right now. Equivalent to
`list --assignee me` with the assignee pinned and a narrower flag
surface. Add `--status`, `--project`, the date flags (`--updated` /
`--created` / `--resolved`), `--order-by` / `--desc`, or arbitrary
`--jql` to narrow and sort further. Like `issue list`, the default
sort is `updated` descending.

```sh
jira issue mine
jira issue mine --status "In Progress"
jira issue mine --status Done --resolved=-7d --order-by resolved   # recently closed, newest first
jira issue mine --as-jql --output=json   # print the JQL without calling Jira
```

## create

File a new issue. The CLI authors creates through `--json-input <file>`
because most creates need more than a summary: project, type, ADF
description, labels, components, and whatever custom fields the
project requires. Put the whole payload in one JSON file and pass it
to the binary. Add `--dry-run` to let the validation pipeline catch
shape errors before Jira sees them.

```sh
jira issue create --no-input --json-input new-issue.json
jira issue create --no-input --json-input new-issue.json --dry-run
```

Minimal payload (snake_case keys; `description` is an ADF document):

```json
{
  "project_key": "<PROJECT_KEY>",
  "issue_type": "Task",
  "summary": "issue summary",
  "description": {
    "type": "doc",
    "version": 1,
    "content": [
      { "type": "paragraph", "content": [{ "type": "text", "text": "body" }] }
    ]
  }
}
```

!!! warning "`create` and `edit` take different payload shapes"

    `issue create --json-input` reads flat CLI-alias keys at the top
    level: `project_key`, `issue_type`, `summary`, `description`,
    `assignee_account_id`, plus any bare Jira field name. There is
    **no** `fields` wrapper and **no** `{"project": {"key": ...}}` /
    `{"issuetype": {"name": ...}}` nesting; the CLI normalises the
    aliases internally before submission.

    [`issue edit --json-input`](#edit) is the opposite: it follows
    Atlassian's REST `editIssue` shape with a top-level `fields`
    object holding bare field names.

    Sending the edit-shape envelope to create fails with
    `screen schema could not be resolved in strict mode:
    pipeline: project/issue-type schema unknown` because the schema
    resolver looks for `project_key`/`issue_type`, not
    `fields.project.key`.

=== "Human"

    ```text
    INF ℹ️ created issue dry_run=false issue="{...}"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.create", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "issue": {
          "id": "10404",
          "key": "<CREATED_ISSUE_KEY>",
          "self": "https://example.atlassian.net/rest/api/3/issue/10404"
        }
      },
      "errors": [],
      "warnings": []
    }
    ```

## edit

Change one or more fields on an existing issue. Use `--summary` or
`--assignee` for single-field tweaks; pass `--json-input` for
everything else (ADF description, custom fields, several fields at
once). The payload follows
[Atlassian's REST `editIssue`](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-issueidorkey-put)
shape: a top-level `fields` object containing bare field names.
Different from [`issue create`](#create), which takes flat
CLI-alias keys (`project_key`, `summary`, …) at the top level.

Bare `jira issue edit KEY` opens `$EDITOR` on the description. That
works for humans; under `--no-input` the CLI refuses it so agents
don't hang on a TTY prompt.

```sh
jira issue edit <ISSUE_KEY> --summary "new title"
jira issue edit <PROJECT_KEY>-1..10 -p 4 --summary "bulk title"
jira issue edit <ISSUE_KEY> --assignee me
jira issue edit <ISSUE_KEY> --no-input --json-input fields.json
jira issue edit <ISSUE_KEY> --no-input --summary "preview only" --dry-run
```

=== "Human"

    ```text
    INF ℹ️ edited issue dry_run=false fields.summary="new title" issue=<ISSUE_KEY> result={}
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.edit", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "fields": { "summary": "new title" },
        "issue": "<ISSUE_KEY>",
        "result": {}
      },
      "errors": [],
      "warnings": []
    }
    ```

`--dry-run` returns the same shape with `dry_run: true` and no
`result` field, the call never reaches Jira.

## comment

Read or write the conversation thread on an issue. `add` writes one,
`list` reads them, `edit` rewrites one by id, `delete` removes one.
Every write path takes `--dry-run` for a local preview.

`add` and `edit` accept either `--body-markdown` (converted to ADF on
the way out) or `--json-input` carrying native ADF. ADF is the wire
format; markdown is the convenience wrapper.

```sh
jira issue comment add <ISSUE_KEY> --body-markdown "looks good"
jira issue comment add <PROJECT_KEY>-1..10 -p 4 --body-markdown "bulk note"
jira issue comment add <ISSUE_KEY> --no-input --json-input adf.json
jira issue comment list <ISSUE_KEY> --all
jira issue comment list <PROJECT_KEY>-1..10 -p 4
jira issue comment edit <ISSUE_KEY> 10244 --body-markdown "edited"
jira issue comment delete <ISSUE_KEY> 10244 --force
```

### comment add

=== "Human"

    ```text
    INF ℹ️ comment.author.account_id=712020:00000000-0000-4000-8000-000000000000
        comment.author.display_name="John Doe"
        comment.author.email_address=john.doe@example.com
        comment.body="..."
        comment.created=2026-05-27T07:13:03.338-0400
        comment.id=10244
        comment.update_author.…
        comment.updated=2026-05-27T07:13:03.338-0400
        dry_run=false
        issue=<ISSUE_KEY>
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.comment.add", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "issue": "<ISSUE_KEY>",
        "comment": {
          "id": "10244",
          "body": "looks good\n",
          "created": "…",
          "updated": "…",
          "author": {
            "account_id": "712020:00000000-0000-4000-8000-000000000000",
            "display_name": "John Doe",
            "email_address": "john.doe@example.com"
          },
          "update_author": { "…": "same shape as author" },
          "visibility": null
        }
      },
      "errors": [],
      "warnings": []
    }
    ```

### comment list

`comment list` accepts the same issue-key lists and ranges as
[`issue view`](#issue-key-expansion-support). Multi-key reads return
`data.results[]`; each successful entry's `data` contains that issue's
`comments`, `pagination`, and any per-key `warnings`.

=== "Human"

    ```text
    Comments  (2 comments)
    #10244    John Doe       2026-05-27T07:13:03.338-0400  looks good
    #10245    John Doe       2026-05-27T07:13:03.697-0400  follow-up
    ```

=== "JSON"

    ```json
    {
      "data": {
        "comments": [
          { "id": "10244", "body": "looks good\n", "author": { … }, "update_author": { … }, "visibility": null, "created": "…", "updated": "…" }
        ],
        "pagination": {
          "is_last": false,
          "max_results": 50,
          "next_page_token": "",
          "start_at": 0,
          "total": 2
        }
      }
    }
    ```

### comment delete

=== "Human"

    ```text
    INF ℹ️ comment_id=10244 deleted=true
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.comment.delete", "timestamp": "…", "request_id": "…" },
      "data": { "comment_id": "10244", "deleted": true }
    }
    ```

## watchers

Subscribe yourself or someone else to issue updates. `watch` and
`unwatch` add or remove the caller; `watchers add` and `watchers
remove` take an explicit `--user`. `watchers list` returns the
current set plus an `is_watching` flag, so a script can ask "am I on
this one?" without comparing account IDs by hand.

```sh
jira issue watch <ISSUE_KEY>              # add yourself
jira issue unwatch <ISSUE_KEY>            # remove yourself
jira issue watch <PROJECT_KEY>-1..10 -p 4
jira issue watchers list <ISSUE_KEY>
jira issue watchers list <PROJECT_KEY>-1..10 -p 4
jira issue watchers add <ISSUE_KEY> --user <accountId>
jira issue watchers add <PROJECT_KEY>-1..10 -p 4 --user <accountId>
jira issue watchers remove <ISSUE_KEY> --user <accountId>
```

### watchers list

`watchers list` supports issue-key lists and ranges. Multi-key reads
return `data.results[]`, with each successful entry carrying that
issue's `watchers`, `is_watching`, and `watch_count`.

=== "Human"

    ```text
    Watchers  (1 watcher)  (you are watching)
      John Doe  …000000  john.doe@example.com
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.watchers.list", "timestamp": "…", "request_id": "…" },
      "data": {
        "is_watching": true,
        "watch_count": 1,
        "watchers": [
          {
            "account_id": "712020:00000000-0000-4000-8000-000000000000",
            "display_name": "John Doe",
            "email_address": "john.doe@example.com"
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

### watch

`was_already_watching` distinguishes "we added you just now" from
"you were already on the list".

=== "Human"

    ```text
    INF ℹ️ is_watching=true was_already_watching=false watch_count=1 watchers="[1 items]"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.watchers.add", "timestamp": "…", "request_id": "…" },
      "data": {
        "is_watching": true,
        "was_already_watching": false,
        "watch_count": 1,
        "watchers": [ { "account_id": "…", "display_name": "John Doe", "email_address": "…" } ]
      }
    }
    ```

### unwatch

=== "Human"

    ```text
    INF ℹ️ is_watching=false was_already_watching=true watch_count=0 watchers=[]
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.watchers.remove", "timestamp": "…", "request_id": "…" },
      "data": {
        "is_watching": false,
        "was_already_watching": true,
        "watch_count": 0,
        "watchers": []
      }
    }
    ```

`watch`, `unwatch`, and `watchers add/remove` all accept `--dry-run`;
the preview is local-only and reports `dry_run: true` plus the
resolved account id (`user_resolved`, `account_id_resolved`).

## link

Record relationships between issues: "blocks", "is duplicated by",
"relates to", and so on. Links show up in the Jira sidebar, in reports,
and in JQL through `linkedIssue`. Use them instead of comments when a
planner or board view needs to see the connection.

Create a link with the bare form `link KEY --to KEY --type NAME`
(there is no `link add` subcommand). `list`, `delete`, and `types`
are explicit. `types` reads from the local cache and is effectively
free once you've run [`cache prime`](cache.md).

```sh
jira issue link <ISSUE_KEY> --to <OTHER_ISSUE_KEY> --type Blocks
jira issue link <PROJECT_KEY>-1..10 -p 4 --to <OTHER_ISSUE_KEY> --type Blocks
jira issue link list <ISSUE_KEY>
jira issue link list <PROJECT_KEY>-1..10 -p 4
jira issue link delete <ISSUE_KEY> 10173 --force
jira issue link types
```

### link create

=== "Human"

    ```text
    INF ℹ️ dry_run=false inward_issue=<ISSUE_KEY> outward_issue=<OTHER_ISSUE_KEY> type=Blocks
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.link", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "inward_issue": "<ISSUE_KEY>",
        "outward_issue": "<OTHER_ISSUE_KEY>",
        "type": "Blocks"
      }
    }
    ```

`--dry-run` returns the same shape with `dry_run: true` and never
contacts Jira.

### link list

Each link carries the type definition inline (so consumers don't have
to round-trip to `link types`) plus the linked issue's summary and
status. `link list` supports issue-key lists and ranges; multi-key
reads return `data.results[]`, with each successful entry carrying
that issue's `key`, `links`, and `count`.

=== "Human"

    Each link renders on one line: outward verb, the linked issue
    key, and its summary. When no links exist:

    ```text
    Links on <ISSUE_KEY>
      (no links)
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.link.list", "timestamp": "…", "request_id": "…" },
      "data": {
        "count": 1,
        "key": "<ISSUE_KEY>",
        "links": [
          {
            "id": "10173",
            "self": "https://example.atlassian.net/rest/api/3/issueLink/10173",
            "type": {
              "id": "10000",
              "name": "Blocks",
              "inward": "is blocked by",
              "outward": "blocks",
              "self": "https://example.atlassian.net/rest/api/3/issueLinkType/10000"
            },
            "direction": "outward",
            "other_issue": {
              "key": "<OTHER_ISSUE_KEY>",
              "summary": "…",
              "status": "To Do"
            }
          }
        ]
      }
    }
    ```

### link types

`cache_state` and `from_cache` reveal whether the answer came from the
local cache. `cache_source_state: "fresh"` means the cache was
refreshed from Jira on this call.

=== "Human"

    ```text
    Link types  (source: cache, fetched_at: …)
    Blocks        outward: blocks              inward: is blocked by
    Cloners       outward: clones              inward: is cloned by
    Duplicate     outward: duplicates          inward: is duplicated by
    Relates       outward: relates to          inward: relates to
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.link.types", "timestamp": "…", "request_id": "…" },
      "data": {
        "cache_empty": false,
        "cache_source_state": "fresh",
        "cache_state": "fresh",
        "count": 4,
        "fetched_at": "…",
        "from_cache": true,
        "link_types": [
          { "id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks", "self": "…" },
          { "id": "10001", "name": "Cloners", "inward": "is cloned by", "outward": "clones", "self": "…" },
          { "id": "10002", "name": "Duplicate", "inward": "is duplicated by", "outward": "duplicates", "self": "…" },
          { "id": "10003", "name": "Relates", "inward": "relates to", "outward": "relates to", "self": "…" }
        ]
      }
    }
    ```

### link delete

=== "Human"

    ```text
    INF ℹ️ deleted=true dry_run=false key=<ISSUE_KEY> link_id=10173
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.link.delete", "timestamp": "…", "request_id": "…" },
      "data": {
        "deleted": true,
        "dry_run": false,
        "key": "<ISSUE_KEY>",
        "link_id": "10173"
      }
    }
    ```

## weblink

Pin an external URL to an issue: runbook, design doc, PR, dashboard,
or anything that belongs in the sidebar rather than the comment
thread.

The CLI only exposes `add`. List, edit, or delete existing weblinks
through the Jira web UI.

```sh
jira issue weblink <ISSUE_KEY> --url https://example.com/spec --title "Spec"
jira issue weblink <ISSUE_KEY> --url https://example.com/spec --dry-run
```

=== "Human"

    ```text
    INF ℹ️ dry_run=false issue=<ISSUE_KEY> title="Spec" url=https://example.com/spec
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.weblink", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "issue": "<ISSUE_KEY>",
        "title": "Spec",
        "url": "https://example.com/spec"
      }
    }
    ```

`--dry-run` returns the same shape with `dry_run: true` and an extra
`url_remote_checked: false` flag, the URL is not fetched and Jira is
not contacted.

## attachment

Manage files attached to an issue, the same set you'd see by
drag-and-drop in the web UI. `add` uploads one or more local paths
(positional, or via repeatable `--file`). `list` shows what's
attached, including the numeric ids Jira assigns. `download` pulls
one to your filesystem by id. `delete` removes one.

Use this for logs, screenshots, exports, and other binary evidence
that doesn't belong in the comment thread.

```sh
jira issue attachment add <ISSUE_KEY> ./screenshot.png
jira issue attachment add <ISSUE_KEY> --file ./a.log --file ./b.log
jira issue attachment add <PROJECT_KEY>-1..10 -p 4 --file ./a.log
jira issue attachment list <ISSUE_KEY>
jira issue attachment list <PROJECT_KEY>-1..10 -p 4
jira issue attachment download <ISSUE_KEY> 10135 --to ./downloads/
jira issue attachment delete <ISSUE_KEY> 10135 --force
```

### attachment add

=== "Human"

    ```text
    INF ℹ️ attachments="[{…}]" dry_run=false key=<ISSUE_KEY>
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.attachment.add", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "key": "<ISSUE_KEY>",
        "attachments": [
          {
            "id": "10135",
            "filename": "screenshot.png",
            "mime_type": "image/png",
            "size": 4096,
            "created": "…",
            "author": {
              "account_id": "712020:00000000-0000-4000-8000-000000000000",
              "display_name": "John Doe"
            }
          }
        ]
      }
    }
    ```

`--dry-run` previews the upload locally: the response carries
`dry_run: true` and a `files: [{ path, size, mime_inferred }]` array
instead of the uploaded `attachments`.

### attachment list

`attachment list` supports issue-key lists and ranges. Multi-key reads
return `data.results[]`, with each successful entry carrying that
issue's `attachments` and `pagination`.

=== "Human"

    ```text
    Attachments  (2 attachments)
      10135  screenshot.png  4 KB  by John Doe  now
      10136  notes.txt       1 KB  by John Doe  now
    ```

=== "JSON"

    ```json
    {
      "data": {
        "attachments": [ { … same shape as add } ],
        "pagination": {
          "is_last": true,
          "max_results": 50,
          "next_page_token": null,
          "start_at": 0,
          "total": 2
        }
      }
    }
    ```

### attachment download

=== "Human"

    ```text
    INF ℹ️ attachment_id=10135 bytes=4096 mode=current-dir written_to=screenshot.png
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.attachment.download", "timestamp": "…", "request_id": "…" },
      "data": {
        "attachment_id": "10135",
        "bytes": 4096,
        "mode": "current-dir",
        "written_to": "screenshot.png"
      },
      "errors": [],
      "warnings": []
    }
    ```

### attachment delete

=== "Human"

    ```text
    INF ℹ️ attachment_id=10135 deleted=true
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.attachment.delete", "timestamp": "…", "request_id": "…" },
      "data": { "attachment_id": "10135", "deleted": true }
    }
    ```

## transition

Move an issue through its workflow (To Do, In Progress, Done, or
whatever your tenant has configured). Two-step pattern: call
`transition KEY` with no flags to see the transitions allowed from
the current status, then re-call with `--transition <id>` to execute
one.

Jira filters available transitions by source status and caller
permissions, so the valid set changes from issue to issue. Listing
first avoids guessing.

```sh
jira issue transition <ISSUE_KEY>                    # list available transitions
jira issue transition <PROJECT_KEY>-1..10 -p 4       # list for several issues
jira issue transition <ISSUE_KEY> --transition 21    # execute (e.g. to In Progress)
jira issue transition <PROJECT_KEY>-1..10 -p 4 --transition 21
jira issue transition <ISSUE_KEY> --transition 21 --dry-run
```

### List available transitions

The no-flag form lists the transitions allowed from the current status.
`meta.command` for this form is `issue.transitions` (plural). The
listing form supports issue-key lists and ranges; multi-key reads return
`data.results[]`, with each successful entry carrying that issue's
`transitions`. The execute form (`--transition <id>`) also accepts
issue-key lists and ranges; it applies the same transition id to each
issue and reports per-key failures when a transition is not valid for
one issue's current workflow state.

=== "Human"

    ```text
    INF ℹ️ issue=<ISSUE_KEY> transitions="[3 items]"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.transitions", "timestamp": "…", "request_id": "…" },
      "data": {
        "issue": "<ISSUE_KEY>",
        "transitions": [
          { "id": "11", "name": "To Do" },
          { "id": "21", "name": "In Progress" },
          { "id": "31", "name": "Done" }
        ]
      }
    }
    ```

### Execute a transition

`meta.command` for the execute form is `issue.transition` (singular).

=== "Human"

    ```text
    INF ℹ️ transitioned issue dry_run=false issue=<ISSUE_KEY> transition=21
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.transition", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "issue": "<ISSUE_KEY>",
        "transition": "21"
      }
    }
    ```

`--dry-run` returns the same shape with `dry_run: true` and never
contacts Jira.

!!! note "Dry-run doesn't validate the transition id"
    `--dry-run` accepts any `--transition` value, including invalid
    ones. Only the executed call validates against Jira's workflow.

## clone

Copy an issue into a new one. The clone keeps the source's editable
fields (summary, ADF description, type, priority, labels, components,
custom fields) and drops lifecycle and identity (key, status, created,
comments, worklogs, links). Override any carried field via
`--json-input`, including `project.key` to land the clone in a
different project.

The source survives untouched. Use clone for a new issue based on an
existing one; use [`move`](#move) when you want to relocate the
original instead.

```sh
jira issue clone <ISSUE_KEY> --no-input --force
jira issue clone <PROJECT_KEY>-1..10 -p 4 --no-input --force
jira issue clone <ISSUE_KEY> --no-input --dry-run    # preview, no --force needed
```

=== "Human"

    `data.issue` is the **source** key; `data.result.key` is the
    **new** clone.

    ```text
    INF ℹ️ cloned issue dry_run=false issue=<ISSUE_KEY> result="{...}"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.clone", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "issue": "<ISSUE_KEY>",
        "result": {
          "id": "10407",
          "key": "<CLONED_ISSUE_KEY>",
          "self": "https://example.atlassian.net/rest/api/3/issue/10407"
        }
      }
    }
    ```

`--dry-run` returns the same shape with `dry_run: true`, drops
`result`, and adds `data.payload.fields` echoing the would-be POST body.
No Jira call.

## move

Relocate an issue to a different project, optionally changing its
issue type along the way. Unlike [`clone`](#clone), `move` does not
create a new issue: comments, worklogs, attachments, and watchers
travel with the issue. If the destination project requires fields the
source didn't carry, declare them in the same `--json-input` payload
alongside the project / type swap.

```sh
jira issue move <ISSUE_KEY> --no-input --json-input move.json --force
jira issue move <PROJECT_KEY>-1..10 -p 4 --no-input --json-input move.json --force
jira issue move <ISSUE_KEY> --no-input --json-input move.json --dry-run
```

Canonical override shape:

```json
{ "fields": { "project": { "key": "<TARGET_PROJECT_KEY>" }, "issuetype": { "name": "Task" } } }
```

=== "Human"

    ```text
    INF ℹ️ moved issue dry_run=true issue=<ISSUE_KEY> payload.fields.issuetype.name=Task payload.fields.project.key=<TARGET_PROJECT_KEY>
    ```

    Real submits emit the same line with `dry_run=false`.

=== "JSON"

    Dry-run echoes the payload back as `data.payload.fields` and never
    contacts Jira. Real submits return `data.result` (often `{}` from
    Jira's 204 No Content) and an unchanged `data.issue` key:

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.move", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": true,
        "issue": "<ISSUE_KEY>",
        "payload": {
          "fields": {
            "issuetype": { "name": "Task" },
            "project":   { "key": "<TARGET_PROJECT_KEY>" }
          }
        }
      },
      "errors": [],
      "warnings": []
    }
    ```

    `--dry-run` is also a false positive (accepts payloads the real
    submit rejects). Cross-project moves currently need to go through
    the Jira web UI.

## delete

Permanently remove the issue. The delete is irreversible and the key
becomes available for the project's next issue. If you want the
record preserved, use [`transition`](#transition) to a terminal
status (`Done`, `Cancelled`) instead.

```sh
jira issue delete <ISSUE_KEY> --no-input --force
jira issue delete <PROJECT_KEY>-1..10 -p 4 --no-input --force
jira issue delete <ISSUE_KEY> --no-input --dry-run
jira issue delete <ISSUE_KEY> --no-input --force --delete-subtasks   # cascade
```

Live multi-key delete always requires `--force`, even in an interactive
TTY. This avoids a long prompt loop and makes bulk deletion explicit.

=== "Human"

    ```text
    INF ℹ️ deleted issue dry_run=false issue=<ISSUE_KEY> result=null
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "issue.delete", "timestamp": "…", "request_id": "…" },
      "data": {
        "dry_run": false,
        "issue": "<ISSUE_KEY>",
        "result": null
      }
    }
    ```

`--dry-run` returns the same shape with `dry_run: true`, `result: null`,
and `data.payload.fields` echoing the would-be DELETE body. No Jira call.

!!! warning "Subtasks block delete unless --delete-subtasks is set"
    Jira refuses to delete an issue that has subtasks unless the
    caller asks for the cascade. The `--delete-subtasks` flag is
    also listed (but ignored) in `clone --help` and `move --help`
    because the underlying mutation handler is shared, it only
    takes effect on `delete`.
