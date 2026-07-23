---
slug: discover
title: Learn the tenant's shape before acting
description: Discover commands and flags from the schema, and the tenant's projects, issue types, fields, and boards from the metadata cache.
when_to_use: Before a first mutation in an unfamiliar project, when a name (type, field, status, board) must be exact, or when deciding cache versus live reads.
commands: [jira cache refresh, jira cache fields, jira cache issuetypes, jira boards list, jira user search]
order: 3
---

## Decide

Two different questions, two different sources:

*   *What can the CLI do?* — the runtime schema. `jira agent schema` prints
    the full command tree with flags; payload shapes are marked
    (`has_input_schema`) and fetched per command with `--path`, or all at
    once with `--shapes`. Derived from the live binary, it cannot go
    stale.
*   *What does this tenant contain?* — the metadata cache. Projects, issue
    types, fields, statuses, priorities, labels, and link types are
    per-tenant data; exact names matter because mutation flags match on
    them.

Prefer the cache for names and ids; go live (`jira issue view`,
`jira search jql`) for issue state, which the cache never holds.

## Run

```sh
# Refresh whatever is stale, then read what you need
jira cache refresh
jira cache issuetypes    # exact type names for --type (instance-wide)
jira cache fields        # field ids for JSON payloads
jira cache statuses      # status names for transitions and filters
jira agent fieldtypes    # how each custom-field type is encoded
jira jql reference       # queryable fields and functions, live

# Boards and the people space are their own lookups
jira boards list
jira user search "jane"  # account ids for assignee/watcher payloads
```

## Save

*   Exact issue-type, status, and priority names — mutation flags and JQL
    match on them verbatim.
*   Field ids (`customfield_10020`-style) for `--json-input` payloads.
*   `account_id` values from `jira user search` — watcher and some assignee
    payloads take account ids, not emails.

## Preconditions

An authenticated profile (`bootstrap`) for refreshes and live tenant
lookups. Schema and field-type inspection are local; cache resource
commands serve a fresh entry locally but fetch and rewrite it when missing,
stale, or explicitly refreshed. None sends a Jira mutation.

## Recover

*   A mutation rejected for an unknown type, field, or status usually means
    a stale cache — `jira cache refresh --force` refetches everything.
    One exception: the issuetypes cache is instance-wide, so a type can
    be valid there yet missing from one project's create screen —
    `--dry-run --validate-remote` sees the real screen.
*   `jira cache refresh --dry-run` reports staleness without contacting
    Jira.
*   Multiple user matches → the envelope lists candidates; narrow the query
    or pick an `account_id` from the list.

## Next

*   `find-issues` — put the discovered names to work in filters.
*   `shape-issues` — use exact names in create and edit payloads.
