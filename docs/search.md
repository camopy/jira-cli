# Search

Three ways to find issues, in order of how much JQL you want to write:

| Command | When to use |
|---------|-------------|
| [`issue list`](issues.md#list) | Common case. Build the query from filter flags. |
| [`search jql`](#search-jql) | Hand-written JQL, one-off. |
| [`search saved`](#search-saved) | A query you reach for repeatedly, stored on disk. |

To preview the JQL a set of `issue list` filters would resolve to
without running the search, see [`jira jql build`](jql.md#build).

## search jql

Execute a JQL query passed inline. `--fields` narrows the per-issue
projection to a comma-separated list; `--full` requests Jira's complete
issue payload (`fields=*all`). The two are mutually exclusive.

!!! warning "Common mistake"
    `--full` on a broad JQL is expensive. It fetches every customfield,
    every comment, and the full description ADF for each match — bytes
    on the wire, tokens in your agent context. Pin the result set first
    (`project = X AND updated >= -1d`), then opt in to `--full` only if
    a downstream consumer needs the complete envelope.

```sh
jira search jql 'project = <PROJECT_KEY> AND status = "To Do"'
jira search jql 'project = <PROJECT_KEY>' --fields key,summary,status
jira search jql 'key = <ISSUE_KEY>' --full
jira search jql 'project = <PROJECT_KEY>' --count   # estimate only, no issues fetched
jira search jql 'project = <PROJECT_KEY>' --all      # walk every page (bounded)
```

### Pagination

`/search/jql` is token-paginated and returns no reliable total, so by
default `search jql` returns one page (`--limit`, default 50). Add
`--all` to walk every page until the server reports `isLast`. The drain
is bounded — 100 pages / 10 000 issues — and a truncated result carries
a `search-truncated` warning in the envelope. Pass `--unbounded` with
`--all` to lift the caps. `--all`/`--limit` can't combine with `--count`
(fetches nothing) or `--web`.

### Count

`--count` returns Jira's *approximate* match count for the query and
fetches no issues — a fast "how many match?" preview before a heavy
read. It is a single call to
[`POST /search/approximate-count`](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/),
so it needs a configured profile. The count is an estimate (no error
bound), and the endpoint ignores any `ORDER BY`. Because no issues are
returned, `--count` can't combine with `--fields`, `--full`, or
`--web`. The same flag is available on [`issue list`](issues.md#list).

=== "Human"

    The bare number, so it pipes straight into a shell:

    ```text
    4242
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "search.count", "timestamp": "…", "request_id": "…" },
      "data": { "source": "inline", "jql": "project = <PROJECT_KEY>", "count": 4242 },
      "errors": [],
      "warnings": []
    }
    ```

=== "Human"

    Human output for `search jql` currently emits the issues array as
    an escape-encoded JSON string on one `INF ℹ️` line. Use
    `--output=json` for any consumer that needs structure; for table
    formatting, [`issue list`](issues.md#list) renders the same JQL
    result as a clean table.

    ```text
    INF ℹ️ searched issues issues="[17 items]" jql="..." source=inline
    ```

=== "JSON"

    The default projection is the flat per-issue summary: `key`,
    `summary`, `status`, `priority`, `assignee`, `updated`.

    ```json
    {
      "ok": true,
      "meta": {
        "command": "search.jql",
        "timestamp": "…",
        "request_id": "…",
        "pagination": { "startAt": 0, "maxResults": 50, "total": 0, "isLast": true }
      },
      "data": {
        "issues": [
          {
            "key": "<ISSUE_KEY>",
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

### `--fields` projection

With `--fields key,summary,status` the issues array switches to the
raw Jira REST shape (each issue carries `id`, `key`, `self`, plus a
nested `fields` object containing only the requested keys):

```json
{
  "data": {
    "issues": [
      {
        "id": "10401",
        "key": "<ISSUE_KEY>",
        "self": "https://example.atlassian.net/rest/api/3/issue/10401",
        "fields": {
          "status": { "name": "To Do" },
          "summary": "Example issue summary"
        }
      }
    ]
  }
}
```

### `--full` projection

`--full` asks Jira for `*all` and returns every field on each
issue: status, priority, reporter, description (as ADF), the
comment block, the worklog, and every `customfield_*` configured
on the project. Use it when a downstream consumer needs the
complete field set rather than the default flat summary or a
hand-picked `--fields` projection.

This is **not** the same as [`issue view`](issues.md#view), which
has no projection flag and returns a curated subset
(`status`, `priority`, `summary`, `description`, `reporter`,
`comment`, `worklog`, and a handful of customfields) regardless of
how many fields the project defines.

### Bad JQL

A query Jira can't parse exits 3 with a `jira_bad_request` error.
The original Jira message lands in `errors[0].upstream_messages` so a
script doesn't have to parse the formatted `message`:

```json
{
  "ok": false,
  "meta": { "command": "search.jql", "exit_code": 3, "timestamp": "…", "request_id": "…" },
  "data": null,
  "errors": [
    {
      "type": "validation",
      "code": "jira_bad_request",
      "message": "{\"errorMessages\":[\"Error in the JQL Query: Expecting 'IN' but got 'valid'. (line 1, character 16)\"],\"errors\":{}}",
      "hint": "Jira rejected the request, check the upstream_messages and upstream_field_errors fields for the specifics, then correct the input before resubmitting.",
      "retryable": false,
      "http_status": 400,
      "rate_limit_remaining": 199,
      "provider": "jira",
      "upstream_messages": [
        "Error in the JQL Query: Expecting 'IN' but got 'valid'. (line 1, character 16)"
      ]
    }
  ],
  "warnings": []
}
```

## search saved

Run a query stored on disk by name. Saved queries live under
`~/.config/jira-cli/queries/<name>.jql` (override the directory via
`queries_path` in the profile config). The file format is one JQL
statement with optional YAML or TOML frontmatter for metadata.

```sh
jira search saved my-open-bugs
jira search saved my-open-bugs --fields key,summary,priority
jira search saved my-open-bugs --full
```

### File format

YAML frontmatter (delimited by `---`), TOML frontmatter (delimited by
`+++`), or no frontmatter at all. The JQL body follows. Fields:

```yaml
---
name: my-open-bugs                # optional; defaults to the filename
description: Bugs assigned to me  # optional; echoed in the envelope
project: <PROJECT_KEY>                      # optional; informational
---
project = <PROJECT_KEY> AND assignee = currentUser() AND statusCategory != Done
ORDER BY priority DESC, updated DESC
```

The filename (minus `.jql`) is the lookup key; `name` in the
frontmatter is purely informational.

=== "Human"

    Same `INF ℹ️` log-line shape as `search jql`; the saved-query
    metadata appears as `key=<name>`, `name=<name>`, `project=<key>`,
    `source=saved`, and the body as `jql="…"`.

    ```text
    INF ℹ️ searched issues description="Bugs assigned to me" issues="[17 items]" jql="project = <PROJECT_KEY> AND assignee = currentUser() …" key=my-open-bugs name=my-open-bugs project=<PROJECT_KEY> source=saved
    ```

=== "JSON"

    The envelope adds a top-level `description` to the data block
    (echoing the frontmatter). The `issues` array follows the same
    projection rules as `search jql` (default summary, or raw Jira
    shape under `--fields` / `--full`).

    ```json
    {
      "ok": true,
      "meta": { "command": "search.saved", "timestamp": "…", "request_id": "…" },
      "data": {
        "description": "Bugs assigned to me",
        "issues": [
          {
            "key": "<ISSUE_KEY>",
            "summary": "…",
            "status": "To Do",
            "priority": "Medium",
            "assignee": null,
            "updated": "…"
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

### Query not found

```json
{
  "ok": false,
  "meta": { "command": "search.saved", "exit_code": 2, "timestamp": "…", "request_id": "…" },
  "data": null,
  "errors": [
    {
      "type": "not_found",
      "code": "not_found",
      "message": "saved query \"no-such-query\" not found",
      "hint": "",
      "retryable": false
    }
  ],
  "warnings": []
}
```

## See also

*   [`issue list`](issues.md#list): same filter surface, but
    executes the query instead of just printing it.
*   [`jira jql build`](jql.md#build): preview the JQL a set of
    `issue list` filters would resolve to without calling Jira.
*   [JQL reference](jql.md): the field, operator, and function
    set available in queries.
