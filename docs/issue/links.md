---
title: Linking issues
description: Connect issues with link, inspect them with link list and link types, remove one with link delete, and pin a URL with weblink.
icon: material/link-variant
---

# :link: Linking issues

Record how issues relate to one another, and pin external URLs to them.
`link` creates an issue-to-issue link, `link list` and `link types` read what
exists, `link delete` removes one by id, and `weblink` attaches a web URL. The
JSON example below shows the `data` block only — the envelope wrapper and exit
codes live on [Output](../output.md), and each command links to its reference
page for the full flag and field tables.

## link

Connect two issues with a typed relationship — "blocks", "duplicates", "relates
to". Links show in the Jira sidebar, in reports, and in JQL via `linkedIssue`.
There is no `link add`: the bare `link` command is the create form. `KEY` is the
inward issue, `--to` is the outward issue, and `--type` is a link-type name from
[`link types`](#link-types). Pass a key range or several keys to link many issues
to the same target.

```sh
jira issue link PROJ-123 --to PROJ-456 --type Blocks
jira issue link PROJ-100..110 -p 4 --to PROJ-456 --type Relates
jira issue link PROJ-123 --to PROJ-456 --type Blocks --dry-run
```

1.  A key range or list links every issue to the same `--to` target; `-p` /
   `--parallelism` fans the writes out (default `1`, max `16`).

`--json-input` accepts the exact `POST /rest/api/3/issueLink` body — `type` by
name or id, `inwardIssue`, `outwardIssue`, and an optional `comment` block, the
only path to a link comment. The inward issue may come from the payload or the
positional `KEY`; setting both to different values is refused, and a body
without `inwardIssue` acts as a template applied to every positional key.

```json
{
  "type": { "name": "Blocks" },
  "inwardIssue": { "key": "PROJ-123" },
  "outwardIssue": { "key": "PROJ-456" },
  "comment": { "body": { "type": "doc", "version": 1, "content": [] } }
}
```

Human output confirms the relationship that was recorded:

```text
INF ℹ️ dry_run=false inward_issue=PROJ-123 outward_issue=PROJ-456 type=Blocks
```

The `data` carries `inward_issue`, `outward_issue`, `type`, and `dry_run`. A
multi-key run returns ordered `data.results[]`, each entry with its own `ok` and
either the created link or an `error`.

!!! warning "Direction follows the link type, not the argument order"
    `--to` only labels the outward issue; what the link *means* comes from the
    type. `Blocks` reads "outward: blocks / inward: is blocked by", so the
    relationship may not point the way the command order suggests. Run
    [`link types`](#link-types) and check the `inward`/`outward` labels before you
    create a link where direction matters. `--dry-run` previews the request
    locally and never contacts Jira.

[Full flags & output fields →](../reference/jira/issue/link/index.md)

## link list

Inspect the links on an issue before you add or remove one. The command flattens
Jira's inward/outward fork into a single direction-aware array, sorted by
direction, type name, then linked key, and carries the type definition inline so
consumers needn't round-trip to `link types`. Pass several keys (or a range) to
read many at once.

```sh
jira issue link list PROJ-123
jira issue link list PROJ-123 PROJ-124 -p 4 --output=json
```

Each `data.links[]` entry pairs the link's `direction` and `type` with the
`other_issue` it points at (key, summary, status):

```json
{
  "key": "PROJ-123",
  "count": 1,
  "links": [
    {
      "id": "10173",
      "self": "https://your-org.atlassian.net/rest/api/3/issueLink/10173",
      "type": { "id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks" },
      "direction": "outward",
      "other_issue": { "key": "PROJ-456", "summary": "Stand up staging DB", "status": "To Do" }
    }
  ]
}
```

A single key returns the `data.key` / `data.links` / `data.count` shape above.
Multiple keys return ordered `data.results[]`, each successful entry carrying its
own `key`, `links`, and `count`.

[Full flags & output fields →](../reference/jira/issue/link/list.md)

## link types

List the link types configured on the active site so you have the exact `--type`
value and its direction labels. The answer is cache-backed: it reads the
`linktypes` cache when fresh and only calls Jira on a miss, a stale entry, or
`--refresh`.

```sh
jira issue link types
jira issue link types --refresh
jira issue link types --output=json
```

```text
NAME       OUTWARD      INWARD
Blocks     blocks       is blocked by
Cloners    clones       is cloned by
Duplicate  duplicates   is duplicated by
Relates    relates to   relates to
```

!!! tip "Cache visibility"
    `data.from_cache` and the `cache_state` fields tell you whether the list came
    from the cache or a fresh fetch. `--ttl-minutes` overrides the freshness
    window; it tracks `jira cache linktypes`, so both commands agree on staleness.

[Full flags & output fields →](../reference/jira/issue/link/types.md)

## link delete

Remove a single link by its global id. Find the id with [`link list`](#link-list)
first. `KEY` is required for context and completion, but Jira deletes links by id
alone, so the id is all that goes on the wire.

```sh
jira issue link delete PROJ-123 10173 --dry-run
jira issue link delete PROJ-123 10173 --force
```

```text
INF ℹ️ deleted=true dry_run=false key=PROJ-123 link_id=10173
```

`data.link_id` echoes the id you passed verbatim. `--dry-run` previews the
deletion and never contacts Jira.

!!! danger "Deletes need --force when headless"
    A live delete prompts for confirmation in an interactive terminal. In
    headless, agent, or `--no-input` mode there is no prompt, so `--force` is
    required or the command fails.

[Full flags & output fields →](../reference/jira/issue/link/delete.md)

## weblink

Pin an external URL to an issue — a pull request, design doc, runbook, or
dashboard — as a Jira remote link. This is separate from issue-to-issue links and
uses a different endpoint. `--url` is required and must be an absolute `http`/
`https` URL; `--title` sets the display text. The CLI only adds web links; list,
edit, or remove them through the Jira web UI.

```sh
jira issue weblink PROJ-123 --url https://github.com/acme/app/pull/42 --title "Fix build"
jira issue weblink PROJ-123 --url https://example.com/design --dry-run
```

```text
INF ℹ️ dry_run=false issue=PROJ-123 title="Fix build" url=https://github.com/acme/app/pull/42
```

`--dry-run` validates the URL syntax locally and returns the same shape with
`dry_run: true` plus `url_remote_checked: false` — it neither fetches the target
nor contacts Jira. Pass several keys to attach the same link to each; that returns
ordered `data.results[]`.

[Full flags & output fields →](../reference/jira/issue/weblink.md)

## See also

*   [Reading issues](read.md) — find the issue and its existing links first
*   [Output](../output.md) — the JSON envelope and exit codes
*   [JQL](../jql.md) — query linked issues with `linkedIssue`
