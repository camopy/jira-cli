# JQL

JQL (Jira Query Language) is Atlassian's filter syntax. `jira-cli` reaches
it three ways:

| Approach | When to use |
|----------|-------------|
| [`issue list`](issues.md#list) filter flags | Common case. The CLI assembles the JQL from `--project`, `--status`, etc. |
| [`jira jql build`](#build) | Same flag surface, but prints the JQL instead of running it. Useful as a preview, or as a starting point for a saved query. |
| [`search jql`](search.md#search-jql) | Hand-written JQL when the flag set can't express what you need. |
| [`jira jql validate`](#validate) | Check a query through Jira's own parser and report errors/warnings. |
| [`jira jql reference`](#reference) | List the fields (incl. custom fields), functions, and reserved words this instance exposes. |

`jira issue list` starts from a bounded default query:

```jql
updated >= -365d ORDER BY updated DESC
```

The commands that call Jira (`jql validate`, `jql reference`) accept
`-d` / `--debug` to print the HTTP request/response trace on stderr
(token redacted); stdout keeps the clean envelope. See
[Output](output.md#debug).

Pass `--assignee me` to scope that default to issues assigned to you, or
hand-write the full query via `--jql`:

```sh
jira issue list --jql 'project = <PROJECT_KEY> AND status = "In Progress"'
```

## build

Assemble a JQL string from the same filter flags [`issue list`](issues.md#list)
accepts, without calling Jira. Use it to preview the query a set of
flags resolves to before running a heavy search, or as a building
block for a saved query. See [Builder coverage](#builder-coverage)
for the full flag set and current limits.

```sh
jira jql build --project <PROJECT_KEY> --assignee me --priority Medium
jira jql build --key <PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12
jira jql build --project <PROJECT_KEY> --updated=-7d   # last 7 days
jira jql build --project <PROJECT_KEY> --desc=false
jira jql build                                        # no filters: defaults
```

=== "Human"

    The preview's job is to hand back the query, so human output is the
    bare JQL — copy/paste- and pipe-safe. On a TTY it's wrapped in a
    terminal hyperlink to the Jira search URL. Add `--debug` to restore
    the `board_scope.applied=… jql="…" precedence=…` diagnostic line.

    ```text
    project = <PROJECT_KEY> AND assignee = currentUser() AND priority = Medium ORDER BY updated DESC
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "jql.build", "timestamp": "…", "request_id": "…" },
      "data": {
        "board_scope": { "applied": false },
        "jql": "project = <PROJECT_KEY> AND assignee = currentUser() AND priority = Medium ORDER BY updated DESC",
        "precedence": "none",
        "url": "https://your-site.atlassian.net/issues/?jql=…"
      },
      "errors": [],
      "warnings": []
    }
    ```

### Defaults

*   No filters: `updated >= -365d ORDER BY updated DESC`.
*   Sort defaults to descending. `--desc` (default): `ORDER BY <field> DESC`.
    Pass `--desc=false` for ascending.
*   `--order-by`: defaults to `updated`; other values are `created`,
    `priority`, `status`, `key`, `summary`.

## validate

`jira jql validate 'JQL'` checks a query through Jira's own parser and
reports per-query errors and warnings — the same parser the server uses,
so it catches field/function/syntax problems the local builder can't.
It is a call to
[`POST /jql/parse`](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-jql/#api-rest-api-3-jql-parse-post),
so it needs a configured profile.

Pass one or more queries. `--mode` sets strictness: `strict` (default,
structural warnings count), `warn`, or `none`.

```sh
jira jql validate 'project = PROJ AND statusCategory != Done'
jira jql validate 'bad =' 'project = PROJ' --mode warn
```

A query that fails to parse is reported as a *result*, not a CLI error:
the command still exits `0` and the envelope is `ok: true`. Branch on
`data.queries[].valid` — do not rely on the exit code to detect invalid
JQL.

=== "Human"

    One line per query — `OK`, `OK (warnings)`, or `INVALID` with the
    parser message:

    ```text
    OK  project = PROJ AND statusCategory != Done
    INVALID  bad = — Error in the JQL Query: expecting a value
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "jql.validate", "timestamp": "…", "request_id": "…" },
      "data": {
        "queries": [
          { "query": "project = PROJ", "valid": true },
          { "query": "bad =", "valid": false,
            "errors": ["Error in the JQL Query: expecting a value"] }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

## reference

`jira jql reference` lists the JQL metadata *this* Jira instance exposes
— every queryable field (including custom fields like `Story Points`),
every function, and the reserved words — straight from
[`GET /jql/autocompletedata`](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-jql/#api-rest-api-3-jql-autocompletedata-get).
Use it to discover what you can actually query — especially custom
fields, which the builder's flag set doesn't cover. Needs a configured
profile.

```sh
jira jql reference                       # human: one field per line
jira jql reference --output=json | jq '.data.fields[] | select(.custom_field_id)'
```

=== "Human"

    One `value — displayName` line per field:

    ```text
    summary — Summary
    cf[10010] — Story Points
    ```

=== "JSON"

    `data.fields[]` (with `custom_field_id` on custom fields — Jira's
    JQL custom-field token, e.g. `cf[10010]`, the same form as `value`,
    *not* the `customfield_10010` REST selector; treat its presence as
    the "this is a custom field" marker), `data.functions[]`, and
    `data.reserved_words[]`.

    ```json
    {
      "ok": true,
      "meta": { "command": "jql.reference", "timestamp": "…", "request_id": "…" },
      "data": {
        "fields": [
          { "value": "summary", "display_name": "Summary" },
          { "value": "cf[10010]", "display_name": "Story Points",
            "custom_field_id": "cf[10010]" }
        ],
        "functions": [ { "value": "currentUser()", "display_name": "currentUser()" } ],
        "reserved_words": ["and", "or", "empty"]
      },
      "errors": [],
      "warnings": []
    }
    ```

## Builder coverage

!!! warning "Work in progress"

    The builder is intentionally limited today; the flag set will
    grow over time. Anything outside what's listed below (raw
    clauses, custom fields, `IN (…)` literals, `NOT`, `OR`,
    parentheses) needs hand-written JQL via
    [`search jql`](search.md#search-jql). File what you need.

Builder flags map to documented Jira JQL concepts:

*   Fields: `--project`, `--epic`, `--assignee`, `--reporter`,
    `--key`, `--status`, `--priority`, `--label`, `--type`,
    `--board`, `--board-id`
*   Dates: `--updated`, `--created`, `--resolved` (see
    [Date filters](#date-filters))
*   Sort: `--order-by <field>`, `--desc=false` for ascending order
*   Operators applied: `=`, `IN (...)` (for repeated flag values),
    `is EMPTY`, date comparators `>=` `<=` `>` `<`
*   Keywords/functions: `AND`, `ORDER BY`, `currentUser()` (via
    `--assignee me` or `--reporter me`)

### Date filters

`--updated`, `--created`, and `--resolved` filter by date. Each takes a
single value in one of these forms:

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

Relative durations use Jira's units (`w` `d` `h` `m`) and **must carry a
sign** — `-7d` is accepted, a bare `7d` is rejected. They pass through to
JQL unevaluated, so Jira resolves them server-side. Absolute dates are
`YYYY-MM-DD`. The range delimiter is `..` only; `:` is not accepted for
dates because it collides with the time-of-day `HH:mm`. Ranges are
inclusive both ends — note Jira resolves a date-only upper bound to
midnight, so `<= 2026-02-01` excludes that day's later events.

### Issue key ranges

`--key` accepts single issue keys, comma lists, repeated flags, and
inclusive ranges:

```sh
jira jql build --key <ISSUE_KEY>
jira jql build --key <ISSUE_KEY>,<OTHER_ISSUE_KEY>
jira jql build --key <PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12
jira jql build --key <PROJECT_KEY>-1..10 --key <OTHER_PROJECT_KEY>-1..12
```

Each comma member is parsed independently. Lists and repeated flags
may mix projects, but one range may not span projects:
`<PROJECT_KEY>-1:<OTHER_PROJECT_KEY>-100` is rejected instead of
crossing project prefixes. Keep comma lists tight
(`<ISSUE_KEY>,<OTHER_ISSUE_KEY>`); whitespace inside a `--key`
expression is not accepted.

## See also

*   [JQL fields](https://support.atlassian.com/jira-service-management-cloud/docs/jql-fields/)
*   [JQL operators](https://support.atlassian.com/jira-service-management-cloud/docs/jql-operators/)
*   [JQL keywords](https://support.atlassian.com/jira-service-management-cloud/docs/jql-keywords/)
*   [JQL developer status](https://support.atlassian.com/jira-service-management-cloud/docs/jql-developer-status/)
*   [Advanced Roadmaps custom fields](https://support.atlassian.com/jira-service-management-cloud/docs/search-for-advanced-roadmaps-custom-fields-in-jql/)
*   [`issue list`](issues.md#list): execute the same filter surface against Jira
*   [`search jql`](search.md#search-jql): run hand-written JQL
*   [`search saved`](search.md#search-saved): store a query on disk and run it by name
