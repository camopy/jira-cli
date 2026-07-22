---
slug: find-issues
title: Locate and read work
description: Read one issue, list and filter issues, build and preview JQL locally, and page through search results.
when_to_use: Any read — inspecting an issue, finding work by filter, running JQL, counting matches, or resuming a paginated search.
commands: [jira issue view, jira issue list, jira issue mine, jira search jql, jira search saved, jira jql build, jira jql validate]
order: 4
---

## Decide

Three tiers, cheapest first:

*   Known key → `jira issue view KEY`.
*   Common filters (project, status, assignee, dates) → `jira issue list`
    flags; the same flags on `jira jql build` emit the JQL without running
    it — the builder always produces valid JQL, so prefer it over
    hand-writing when the flags can express the query. Status comparators
    (`<Done`, `>=In Progress`) compare the three workflow **categories**,
    not status names. `--key` takes commas and ranges (`PROJ-1..PROJ-5`);
    ranges never cross projects.
*   Anything the flags cannot express → hand-written JQL via
    `jira search jql`.

Narrow the payload deliberately: default fields are the working set,
`--fields summary,status,assignee` trims further, `--full` fetches
everything. `--count` answers "how many" without fetching any issue.

## Run

```sh
jira issue view PROJ-123

# Flag-driven filtering; --jql escapes the flag grammar entirely
jira issue list --project PROJ --status '<Done' --assignee me --updated -7d

# Preview the JQL a filter set produces — no Jira call
jira jql build --project PROJ --status '!Abandoned' --order-by updated --desc

# Validate hand-written JQL before spending a search on it
jira jql validate 'project = PROJ AND status CHANGED AFTER -7d'

# Run it, count first if the result set is unknown
jira search jql 'project = PROJ AND sprint IN openSprints()' --count
jira search jql 'project = PROJ AND sprint IN openSprints()' --limit 50

# Recurring queries: one .jql file per name under the configured
# queries_path, run by name
jira search saved triage
```

## Save

*   `data.results[]` issue objects; each identity is `{key, id, self}`.
    `jira issue view` with one key returns `data.issue`; with several it
    switches to `data.results[]` — never parse `data.issue` on a
    multi-key view.
*   `meta.pagination.nextCursor` — resume the next page with `--cursor`;
    `--all` walks every page (capped at 100 pages / 10 000 issues unless
    `--unbounded`).
*   The built JQL string from `jira jql build` — reusable in
    `jira search jql` and saved queries.

## Preconditions

An authenticated profile. Status, type, and board names must match the
tenant exactly — `discover` covers finding them.

## Recover

*   Exit 2 on a key → the issue does not exist or the account cannot see
    it; confirm project visibility with `jira issue list --project`.
*   A bad query on `jira search jql` is validation (exit 3) with Jira's
    own message. `jira jql validate` is different: it exits 0 either way —
    a parse failure is a *result*; read `data.queries[].valid`, not the
    exit code.
*   Truncated-looking results → pagination caps; follow the cursor or pass
    `--all`.

## Next

*   `discover` — exact names for filters.
*   `shape-issues` — act on what you found.
*   `core-contract` — pagination and result-shape details.
