---
title: JQL
description: Build, validate, and explore JQL with jql build, jql validate, and jql reference — plus the builder's flag-to-clause mapping.
icon: material/filter-variant
---

# :mag_right: JQL

JQL is Atlassian's filter syntax. `jira-cli` reaches it
four ways:

| Approach | When to use |
|----------|-------------|
| [`issue list`](issue/read.md#list) filter flags | Common case. The CLI assembles the JQL from `--project`, `--status`, etc. |
| [`jira jql build`](#build) | Same flags, but prints the JQL instead of running it. A preview, or a starting point for a saved query. |
| [`jira jql validate`](#validate) | Check a query through Jira's own parser and report errors and warnings. |
| [`jira jql reference`](#reference) | List the fields (including custom fields), functions, and reserved words this instance exposes. |

For hand-written JQL the flags can't express, run it with
[`search jql`](search.md#search-jql). JSON examples below show the `data` block
only — the envelope and exit codes live on [Output](output.md), and each command
links to its reference page for the full tables.

`issue list` starts from a bounded default query:

```jql
updated >= -365d ORDER BY updated DESC
```

Pass `--assignee me` to scope that to your issues, or hand-write the whole query
with `--jql`:

```sh
jira issue list --jql 'project = PROJ AND status = "In Progress"'
```

## build

Assemble a JQL string from the same filter flags
[`issue list`](issue/read.md#list) accepts, without calling Jira. Use it to
preview the query a set of flags resolves to before a heavy search, or as a
building block for a saved query. See [Builder coverage](#builder-coverage) for
the full flag set and current limits.

```sh
jira jql build --project PROJ --assignee me --priority Medium
jira jql build --key PROJ-1..PROJ-10
jira jql build --project PROJ --updated=-7d
jira jql build --project PROJ --desc=false
jira jql build
```

Human output is the bare JQL — copy/paste- and pipe-safe (wrapped in a terminal
hyperlink to the Jira search URL on a TTY). The JSON `data` adds the resolved
`url` and `board_scope`:

```json
{
  "board_scope": { "applied": false },
  "jql": "project = PROJ AND assignee = currentUser() AND priority = Medium ORDER BY updated DESC",
  "precedence": "none",
  "url": "https://example.atlassian.net/issues/?jql=…"
}
```

Defaults: no filters gives `updated >= -365d ORDER BY updated DESC`. Sort defaults to descending
on `updated` (`--order-by` takes `created`, `priority`,
`status`, `key`, `summary`; `--desc=false` sorts ascending).

[Full flags & output fields →](reference/jira/jql/build.md)

## validate

Check a query through Jira's own parser — the same parser the server uses, so it
catches field, function, and syntax problems the local builder can't. It's a
call to
[`POST /jql/parse`](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-jql/#api-rest-api-3-jql-parse-post),
so it needs a configured profile. Pass one or more queries; `--mode` sets
strictness (`strict` default, `warn`, or `none`).

```sh
jira jql validate 'project = PROJ AND statusCategory != Done'
jira jql validate 'bad =' 'project = PROJ' --mode warn
```

A query that fails to parse is reported as a *result*, not a CLI error: the
command still exits 0 and the envelope is `ok: true`. Branch on
`data.queries[].valid` — don't rely on the exit code to detect invalid JQL.

```json
{
  "queries": [
    { "query": "project = PROJ", "valid": true },
    {
      "query": "bad =",
      "valid": false,
      "errors": ["Error in the JQL Query: expecting a value"]
    }
  ]
}
```

[Full flags & output fields →](reference/jira/jql/validate.md)

## reference

List the JQL metadata *this* Jira instance exposes — every queryable field
(including custom fields like `Story Points`), every function, and the reserved
words — straight from
[`GET /jql/autocompletedata`](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-jql/#api-rest-api-3-jql-autocompletedata-get).
Use it to discover what you can query, especially custom fields, which the
builder's flag set doesn't cover. Needs a configured profile.

```sh
jira jql reference
jira jql reference --output=json | jq '.data.fields[] | select(.custom_field_id)'
```

Human output is one `value — displayName` line per field. The JSON `data` splits
into `fields[]`, `functions[]`, and `reserved_words[]`. Custom fields carry a
`custom_field_id` — Jira's JQL token (`cf[10010]`, the same form as `value`, not
the `customfield_10010` REST selector). Treat its presence as the "this is a
custom field" marker:

```json
{
  "fields": [
    { "value": "summary", "display_name": "Summary" },
    {
      "value": "cf[10010]",
      "display_name": "Story Points",
      "custom_field_id": "cf[10010]"
    }
  ],
  "functions": [ { "value": "currentUser()", "display_name": "currentUser()" } ],
  "reserved_words": ["and", "or", "empty"]
}
```

[Full flags & output fields →](reference/jira/jql/reference.md)

## Builder coverage

!!! warning "Work in progress"
    The builder is intentionally limited today; the flag set will grow. Anything
    outside what's listed below — raw clauses, custom fields, `IN (…)` literals,
    `NOT`, `OR`, parentheses — needs hand-written JQL via
    [`search jql`](search.md#search-jql). File what you need.

Builder flags map to documented Jira JQL concepts:

*   Fields: `--project`, `--epic`, `--assignee`, `--reporter`, `--key`,
    `--status`, `--priority`, `--label`, `--type`, `--board`, `--board-id`
*   Dates: `--updated`, `--created`, `--resolved` (see [Date filters](#date-filters))
*   Sort: `--order-by <field>`, `--desc=false` for ascending
*   Operators: `=`, `IN (…)` (for repeated flag values), `is EMPTY`, date
    comparators `>=` `<=` `>` `<`
*   Keywords and functions: `AND`, `ORDER BY`, `currentUser()` (via
    `--assignee me` or `--reporter me`)

### Date filters

`--updated`, `--created`, and `--resolved` each take a single value in one of
these forms:

| Value | Meaning | JQL |
|-------|---------|-----|
| `-7d` | relative, last 7 days (bare = lower bound) | `updated >= -7d` |
| `2026-01-01` | absolute, on or after | `created >= "2026-01-01"` |
| `>=2026-01-01` | explicit comparator (`>` `>=` `<` `<=`) | `created >= "2026-01-01"` |
| `2026-01-01..2026-02-01` | inclusive range | `created >= "2026-01-01" AND created <= "2026-02-01"` |
| `2026-01-01..` | open upper bound | `created >= "2026-01-01"` |
| `..2026-02-01` | open lower bound | `created <= "2026-02-01"` |

```sh
jira jql build --updated=-7d
jira jql build --created 2026-01-01..2026-02-01
jira jql build --resolved '<=2026-02-01'
```

Relative durations use Jira's units (`w` `d` `h` `m`) and **must carry a sign** —
`-7d` is accepted, a bare `7d` is rejected. They pass through to JQL unevaluated,
so Jira resolves them server-side. Absolute dates are `YYYY-MM-DD`. The range
delimiter is `..` only — `:` is not accepted for dates because it collides with
the time-of-day `HH:mm`. Ranges are inclusive both ends; note Jira resolves a
date-only upper bound to midnight, so `<= 2026-02-01` excludes that day's later
events.

### Issue key ranges

`--key` accepts single keys, comma lists, repeated flags, and inclusive ranges:

```sh
jira jql build --key PROJ-123
jira jql build --key PROJ-1,PROJ-2
jira jql build --key PROJ-1..PROJ-10
jira jql build --key PROJ-1..PROJ-10 --key OTHER-1..OTHER-12
```

Each comma member is parsed independently. Lists and repeated flags may mix
projects, but one range may not span projects — `PROJ-1..OTHER-100` is rejected
rather than crossing prefixes. The range delimiter is `..` only. Keep comma
lists tight (`PROJ-1,PROJ-2`); whitespace inside a `--key` expression is not
accepted.

## See also

*   [`issue list`](issue/read.md#list) — execute the same filter surface against Jira
*   [`search jql`](search.md#search-jql) — run hand-written JQL
*   [`search saved`](search.md#search-saved) — store a query on disk and run it by name
*   [JQL fields](https://support.atlassian.com/jira-service-management-cloud/docs/jql-fields/), [operators](https://support.atlassian.com/jira-service-management-cloud/docs/jql-operators/), [keywords](https://support.atlassian.com/jira-service-management-cloud/docs/jql-keywords/) — Atlassian's reference
</content>
