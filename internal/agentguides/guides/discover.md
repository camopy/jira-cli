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
    the full command tree with flags and output shapes; `--path` subsets one
    subtree. It is derived from the live binary and cannot go stale.
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
jira cache issuetypes    # exact type names for --type
jira cache fields        # field ids for JSON payloads
jira cache statuses      # status names for transitions and filters

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

An authenticated profile (`bootstrap`); every command here reads Jira, none
mutates it.

## Recover

*   A mutation rejected for an unknown type, field, or status usually means
    a stale cache — `jira cache refresh --force` refetches everything.
*   `jira cache refresh --dry-run` reports staleness without contacting
    Jira.
*   Multiple user matches → the envelope lists candidates; narrow the query
    or pick an `account_id` from the list.

## Next

*   `find-issues` — put the discovered names to work in filters.
*   `shape-issues` — use exact names in create and edit payloads.
