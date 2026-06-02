# JQL

JQL (Jira Query Language) is Atlassian's filter syntax. `jira-cli` reaches
it three ways:

| Approach | When to use |
|----------|-------------|
| [`issue list`](issues.md#list) filter flags | Common case. The CLI assembles the JQL from `--project`, `--status`, etc. |
| [`jira jql build`](#build) | Same flag surface, but prints the JQL instead of running it. Useful as a preview, or as a starting point for a saved query. |
| [`search jql`](search.md#search-jql) | Hand-written JQL when the flag set can't express what you need. |

`jira issue list` starts from a bounded default query:

```jql
updated >= -365d ORDER BY updated DESC
```

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

    ```text
    INF ℹ️ board_scope.applied=false jql="project = <PROJECT_KEY> AND assignee = currentUser() AND priority = Medium ORDER BY updated DESC" precedence=none
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "jql.build", "timestamp": "…", "request_id": "…" },
      "data": {
        "board_scope": { "applied": false },
        "jql": "project = <PROJECT_KEY> AND assignee = currentUser() AND priority = Medium ORDER BY updated DESC",
        "precedence": "none"
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
