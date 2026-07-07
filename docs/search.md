---
title: Search
description: Run hand-written and saved JQL with search jql and search saved — projection, pagination, and counts.
icon: material/magnify
---

# :mag: Search

Three ways to find issues, in order of how much JQL you write:

| Command | When to use |
|---------|-------------|
| [`issue list`](issue/read.md#list) | Common case. Build the query from filter flags. |
| [`search jql`](#search-jql) | Hand-written JQL, one-off. |
| [`search saved`](#search-saved) | A query you reach for repeatedly, stored on disk. |

To preview the JQL a set of `issue list` filters resolves to without running it,
see [`jira jql build`](jql.md#build). JSON examples below show the `data` block
only — the envelope wrapper and exit codes live on [Output](output.md), and each
command links to its reference page for the full flag and output-field tables.

## search jql

Run a JQL query passed inline. `--fields` narrows the per-issue projection to a
comma-separated list; `--full` requests Jira's complete issue payload
(`fields=*all`). The two are mutually exclusive.

```sh
jira search jql 'project = PROJ AND status = "To Do"'
jira search jql 'project = PROJ' --fields key,summary,status
jira search jql 'key = PROJ-123' --full
jira search jql 'project = PROJ' --count
jira search jql 'project = PROJ' --all --output=json
```

The default projection is the flat per-issue summary — `key`, `summary`,
`status`, `status_category`, `assignee`, `priority`, `updated`.
`status_category` is the stable workflow bucket (`new`, `indeterminate`, or
`done`), with a `status_color` field alongside when Jira reports the category's
colour:

```json
{
  "issues": [
    {
      "key": "PROJ-123",
      "summary": "Checkout returns 500 on empty cart",
      "status": "To Do",
      "status_category": "new",
      "status_color": "blue-gray",
      "assignee": null,
      "priority": "Medium",
      "updated": "…"
    }
  ]
}
```

Human output for `search jql` prints the issues array as an escape-encoded
string on one `INF ℹ️` line — fine to eyeball, awkward to parse. For a clean
table use [`issue list`](issue/read.md#list) on the same JQL; for structure use
`--output=json`.

!!! warning "`--full` on a broad query is expensive"
    `--full` fetches every custom field, every comment, and the full
    description ADF for each match — bytes on the wire, tokens in an agent's
    context. Pin the result set first (`project = PROJ AND updated >= -1d`),
    then opt in to `--full` only if a consumer needs the complete payload.

### Projection shapes

`--fields` keeps the flat summary shape and trims it to the requested fields;
`key` always stays as the row identity. Summary fields keep the names and
types the default projection publishes — requesting `status` still yields the
flat `status` string with `status_category` (and `status_color`) alongside,
never a nested status object:

```json
{
  "issues": [
    {
      "key": "PROJ-123",
      "summary": "Checkout returns 500 on empty cart",
      "status": "To Do",
      "status_category": "new",
      "status_color": "blue-gray"
    }
  ]
}
```

A requested field outside the summary set — `issuetype`, `labels`, a
`customfield_*` id — rides top-level under its Jira id carrying the wire
value, or `null` when Jira does not return it:

```json
{
  "issues": [
    {
      "key": "PROJ-123",
      "issuetype": { "name": "Bug" },
      "customfield_10010": "Sprint 5"
    }
  ]
}
```

`--full` is the only mode that switches to the raw Jira REST shape — each
issue carries `id`, `key`, `self`, and a nested `fields` object. It asks for
`*all` and returns every field on each issue — status,
priority, reporter, description ADF, the comment block, the worklog, and every
`customfield_*` on the project. This is **not** the same as
[`issue view`](issue/read.md#view): `view` has no projection flag and returns a
curated subset regardless of how many fields the project defines.

### Pagination

`/search/jql` is token-paginated and returns no reliable total, so `search jql`
returns one page by default (`--limit`, default 50) and `meta.pagination` omits
`total` — `isLast` and `nextCursor` are the walk signals. Page by page, pass a
returned `nextCursor` back via `--cursor`; the same flag resumes after a
context reset or checkpoint. Add `--all` to walk every page until the server
reports `isLast`. The drain is bounded — 100 pages / 10 000 issues — and a
truncated result carries a `search-truncated` warning plus, when the cut fell
on a page boundary, the resume cursor. Pass `--unbounded` with `--all` to lift
the caps. `--all`, `--limit`, and `--cursor` can't combine with `--count` or
`--web`. [`issue list`](issue/read.md#list) accepts the same
`--limit` / `--all` / `--cursor` / `--unbounded` set against its built query.

### Count

`--count` returns Jira's approximate match count and fetches no issues — a fast
"how many?" before a heavy read. Human output is the bare number; JSON carries
`count`. It's a single call to
[`POST /search/approximate-count`](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/),
so it needs a configured profile. The count is an estimate with no error bound,
and the endpoint ignores any `ORDER BY`. Because nothing is fetched, `--count`
can't combine with `--fields`, `--full`, or `--web`. The same flag is on
[`issue list`](issue/read.md#list).

A query Jira can't parse exits 3 with a `jira_bad_request` error.
`errors[0].message` is the clean parser message; the raw upstream array is kept
in `errors[0].upstream_messages` so a script can branch on it. See
[Output](output.md) for the error envelope.

[Full flags & output fields →](reference/jira/search/jql.md)

## search saved

Run a query stored on disk by name. Saved queries live under
`~/.config/jira-cli/queries/<name>.jql` (on Windows,
`%AppData%\jira-cli\queries\<name>.jql`; override with `queries_path` in the
profile config). The file is one JQL statement with optional frontmatter for
metadata. Projection, pagination, and count flags behave exactly as on
[`search jql`](#search-jql).

```sh
jira search saved my-open-bugs
jira search saved my-open-bugs --fields key,summary,priority
jira search saved my-open-bugs --full
```

The JSON `data` carries the saved-query metadata — `source: "saved"`, `key`,
`name`, `description`, `project` (each echoing the frontmatter) — alongside the
`jql` that ran and the `issues` array:

```json
{
  "source": "saved",
  "key": "my-open-bugs",
  "name": "my-open-bugs",
  "description": "Bugs assigned to me",
  "project": "PROJ",
  "jql": "project = PROJ AND assignee = currentUser() AND statusCategory != Done",
  "issues": [ … ]
}
```

A name with no matching file is rejected as invalid input: exit 3 with a
`validation` error (`code=arg_value_invalid`) — the lookup is a local file,
not a Jira resource.

### File format

The file takes YAML frontmatter (delimited by `---`), TOML frontmatter
(delimited by `+++`), or no frontmatter at all, then the JQL body. The filename
minus `.jql` is the lookup key; `name` in the frontmatter is informational only.

```yaml
---
name: my-open-bugs                # optional; defaults to the filename
description: Bugs assigned to me  # optional; echoed in the envelope
project: PROJ                     # optional; informational
---
project = PROJ AND assignee = currentUser() AND statusCategory != Done
ORDER BY priority DESC, updated DESC
```

[Full flags & output fields →](reference/jira/search/saved.md)

## See also

*   [`issue list`](issue/read.md#list) — same filter surface, but executes the query
*   [`jira jql build`](jql.md#build) — preview the JQL filters resolve to, offline
*   [JQL](jql.md) — the field, operator, and function set queries use
*   [Output](output.md) — the JSON envelope and exit codes
</content>
</invoke>
