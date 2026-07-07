---
title: Caching
description: The local metadata cache — where it lives, per-resource TTLs, freshness and invalidation, the priming commands, and fleet-safe use.
icon: material/database-outline
---

# :card_file_box: Caching

`jira-cli` keeps a local copy of slow-changing Jira metadata so common
operations (completion, validation, board scoping, ADF field-name resolution)
don't pay a round trip every time. Caches are scoped to a
`(profile, base_url, config_path)` tuple and live under:

```text
~/.cache/jira-cli/<profile>-<hash>/<resource>.json
```

The base directory resolves OS-natively:

| Platform | Cache directory |
|----------|-----------------|
| Linux/macOS | `$XDG_CACHE_HOME/jira-cli/...` if `XDG_CACHE_HOME` is an absolute path, else `~/.cache/jira-cli/...` |
| Windows | `%LocalAppData%\cache\jira-cli\...` (e.g. `C:\Users\You\AppData\Local\cache\jira-cli\...`); `XDG_CACHE_HOME` is honoured when set to an absolute path |

Each `jira cache <resource>` command primes the matching `<resource>.json`,
reading from disk when fresh and refetching from Jira when missing, stale, or
`--refresh` is passed.

| Resource | What it powers | Default TTL |
|----------|----------------|-------------|
| `labels` | `--label` completion | 1 hour |
| `epics` | `--epic` completion in the issue create flow | 4 hours |
| `projects` | `--project` completion | 7 days |
| `fields` | `customfield_*` resolution in the ADF field map, `--field` completion | 14 days |
| `boards` | `--board` completion, board-scoped JQL in [`issue list`](issue/read.md#list) and [`jql build`](jql.md#build) | 28 days |
| `issuetypes` | `--type` completion (the instance-wide type list) | 30 days |
| `statuses` | `--status` completion | 30 days |
| `linktypes` | `--type` completion for [`issue link`](issue/links.md#link) | 90 days |
| `priorities` | `--priority` completion | 90 days |
| `resolutions` | resolution names for resolve transitions | 90 days |

Every primer accepts `--refresh` (force a fetch even when fresh) and
`--ttl-minutes <n>` (override the resource's freshness window for this call).
Each resource page under [Reference › cache](reference/jira/cache/index.md)
carries its full flag and output-field table. JSON examples below show the `data`
block only — the envelope wrapper and exit codes live on [Output](output.md).

## The shared envelope

Every flat-list primer returns the same `data` fields — only the payload field
name and its items change per resource:

```json
{
  "cache_empty": false,
  "cache_state": "fresh",
  "cache_source_state": "fresh",
  "count": 3,
  "fetched_at": "…",
  "from_cache": true,
  "profile": "default",
  "projects": [
    { "id": "10034", "key": "PROJ", "name": "Example Project", "project_type": "software" }
  ]
}
```

`cache_state` is one of `missing`, `fresh`, `stale`, `malformed`, `refresh`, or
`empty`. Human output collapses this to a single `INF ℹ️` log line of
`key=value` pairs, with arrays rendered as `field="[N items]"` rather than the
structured payload — reach for `--output=json` when a consumer needs per-item
detail. Add `-d` / `--debug` to print the HTTP trace to stderr (token redacted);
see [Output › Debug](output.md#debug).

## Freshness and invalidation

Each resource has its own default freshness window — its TTL, listed above. The
windows run long because completion never blocks on a network call: it reads
whatever is cached and falls back to empty. A stale cache therefore only skews
autocomplete — a board you just created stays invisible, a label you just
deleted lingers — until the next refresh; completion never reaches for the
network to correct it. (Create, edit, and move validation is separate: it
queries Jira's live create / edit screens, so it is never served by this cache.)
Resources that churn (labels, epics) keep short windows; admin-managed schema
(issue types, statuses, link types, priorities) runs to weeks or months.

A cached resource counts as a miss — and is refetched on the next
freshness-sensitive read — when any of these hold:

*   the cache file is absent;
*   its JSON is malformed;
*   it is older than the resource's TTL, *and* the caller asked for fresh data
    (completion ignores age, so a long TTL never makes autocomplete reach for
    the network);
*   its on-disk schema version no longer matches the running binary — a CLI
    upgrade that changes the cached shape invalidates old entries rather than
    mis-parsing them;
*   you pass `--refresh` or `--force`, or run `cache clear`;
*   the `(profile, base_url, config_path)` tuple changes — a different profile
    or site reads a different cache namespace.

Age alone never triggers a *background* refetch — there is no daemon, and
completion never makes a surprise network call. Refetching happens only when you
run a command that reads with freshness intent: any primer (or
[`cache refresh`](#refresh)) refetches when its cached read misses, and
`--refresh` / `--force` refetches even when the cache is still fresh.

## The list primers

These flat-list primers share the [envelope above](#the-shared-envelope); each differs only
in its payload field and item shape:

```sh
jira cache projects --output=json
jira cache fields --refresh --output=json
```

| Resource (`jira cache …`) | Payload field | Item shape |
|---------------------------|---------------|------------|
| `projects` | `projects` | `{ id, key, name, project_type }` |
| `fields` | `fields` | `{ id, name, type }` — the system + custom field map for `customfield_*` resolution |
| `issuetypes` | `issuetypes` | `{ id, name, subtask }` — every type across visible projects |
| `labels` | `labels` | flat strings, not objects |
| `epics` | `epics` | `{ key, summary, status }` — maps `--epic <key>` to a parent link |
| `linktypes` | `link_types` (snake_case) | `{ id, name, inward, outward, self }` |
| `statuses` | `statuses` | `{ id, name, status_category }` from `GET /status`, instance-wide (not per-project); the same display name recurs under different ids across workflows |
| `priorities` | `priorities` | `{ id, name }` from `GET /priority` |
| `resolutions` | `resolutions` | `{ id, name }` from `GET /resolution` — the tenant's resolution names for resolve transitions |

## boards

`cache boards` primes the on-disk board list but does **not** return the boards
array — use [`boards list`](#boards-list) to read what was cached. `--unbounded`
walks every page; the default caps at 100 pages / 10 000 boards and sets
`truncated: true` if the cap was hit.

```sh
jira cache boards
jira cache boards --refresh --unbounded --output=json
```

Its `data` carries metadata only — `boards_count`, `primed`, `truncated`,
`truncated_reason`, and `ttl_seconds` — alongside the shared cache fields.

## boards list

`jira boards list` reads the cached `boards.json` and prints the board array,
with the same `--refresh` / `--unbounded` re-priming semantics as `cache boards`.

```sh
jira boards list
jira boards list --refresh --output=json
```

```json
{
  "boards": [
    { "id": 1, "name": "Example board", "project_keys": ["PROJ"], "type": "simple" }
  ],
  "cache_state": "fresh",
  "from_cache": true,
  "truncated": false
}
```

The pagination block rides in `meta.pagination` (`startAt`, `maxResults`,
`total`, `isLast`), the same shape every paginated command emits. `--limit`
windows the cached set (default `50`) and `--all` returns everything.

[Full flags & output fields →](reference/jira/boards/list.md)

### Using a default board

Pin a board to the active profile so `issue list` and `jql build` scope by it
without an explicit `--board` flag (matched case-insensitively against the
cache):

```sh
jira config set profiles.default.default_board "Example board"
```

## refresh

`cache refresh` primes several resources in one pass. With no argument it covers
every resource; pass names to limit it. It is TTL-gated by default — a resource
inside its window is reported `fresh` and left untouched — and `--force`
refetches everything.

!!! note "`cache refresh` vs `cache <resource> --refresh`"
    They both refetch, but they're different tools. `cache <resource> --refresh`
    refetches **one** resource — always, even when fresh — and returns its
    payload (the list itself). `cache refresh [resource…]` is the batch
    maintainer: one, many, or all resources, TTL-gated so it **skips** anything
    still fresh (`--force` to override), with bounded `-p` concurrency, and it
    returns per-resource status in `results[]`, not the lists. Reach for the
    primer when you want the data; reach for `refresh` when you want to warm or
    maintain caches.

```sh
jira cache refresh
jira cache refresh --force
jira cache refresh boards labels
jira cache refresh -p 4 --output=json
```

Resources fetch with bounded concurrency: `-p` / `--parallelism` defaults to `1`
(sequential — the rate-limit-safe default); raise it (up to 16) to fetch in
parallel. `--ttl-minutes <n>` overrides every window for the run, and
`--unbounded` lifts the boards page cap. One resource failing doesn't abort the
rest — successes stay in `data.results`, failures go to `errors[]`, and the
command exits with the highest per-resource failure code.

The output is the shared multi-key shape: a `results[]` keyed by resource name,
each carrying `status` (`fresh` / `refreshed`), `from_cache`, `count`,
`fetched_at`, and `duration_ms`, plus `succeeded` / `failed` totals. A partial
failure sets `ok: false`, replaces the failing resource's `data` with an
`error`, and mirrors it into top-level `errors[]`:

```json
{
  "results": [
    { "key": "statuses", "ok": true, "data": { "count": 3, "from_cache": false, "status": "refreshed" } },
    { "key": "fields", "ok": false, "error": { "type": "not_found", "code": "jira_not_found", "http_status": 404 } }
  ],
  "succeeded": 1,
  "failed": 1
}
```

Unknown resource names are rejected up front with `code=arg_value_invalid`
(exit 3), before any fetch.

[Full flags & output fields →](reference/jira/cache/refresh.md)

## clear

```sh
jira cache clear
jira cache clear projects
```

With no argument, every cache file for the active profile is deleted and the
count comes back in `data.removed` (an integer). With a resource argument, only
that file is removed and `data.removed` is a boolean — `true` when a file existed
and was deleted, `false` when there was nothing to remove:

```json
{ "profile": "default", "removed": true, "resource": "projects" }
```

Valid resources: `labels`, `projects`, `epics`, `fields`, `issuetypes`,
`linktypes`, `boards`, `statuses`, `priorities`, `resolutions`. An unknown name
is rejected up front with `code=arg_value_invalid` (exit 3); the error message
lists the valid set.

`--dry-run` reports what a live clear would remove — the same `data.removed`
shape with `dry_run: true` — without touching any file. In a script, agent, or
CI context (non-TTY, or `--no-input`) a live clear requires `--force`; an
interactive terminal proceeds without a prompt, since a cleared cache is
rebuilt by the next prime:

```sh
jira cache clear --dry-run --output=json
jira cache clear projects --force --output=json
```

[Full flags & output fields →](reference/jira/cache/clear.md)

## Fleet operations

Multiple agents or CI runners hitting the same Jira tenant should not all
`cache <resource> --refresh` at once. Jira rate-limits per-token, and a
fleet-wide synchronised refresh is the fastest way to trip a 429. Within a single
run, prefer [`cache refresh`](#refresh) over a shell loop of per-resource
primers: it keeps concurrency bounded and reports per-resource status in one
envelope.

!!! warning "A cold `cache clear` in CI is a 429 magnet"
    `jira cache clear` in a CI prelude on every job — combined with parallel
    jobs — turns every cache miss into a synchronised Jira fetch. Cold-start a
    shared step once at workflow level, then let per-job invocations read the
    warm cache. CI is a non-TTY context, so a live clear there also needs
    `--force`.

*   Warm the cache once in a setup step, then share the cache directory (e.g.
    GitHub Actions `actions/cache` keyed on profile + base URL) across the matrix.
*   Stagger explicit `--refresh` calls; the default TTL behaviour already
    amortises across short-lived calls.
*   Honour `errors[0].retry_after_seconds` from a 429 envelope — don't retry in
    a tight loop.
*   Avoid `cache clear` (no resource) on shared infrastructure unless you
    control every consumer of the cache directory.

See [Troubleshooting](troubleshooting.md) for the rate-limit diagnostic flow.

## See also

*   [`issue list`](issue/read.md#list) — filter flags rely on the cached
    `projects`, `issuetypes`, `labels`, and `boards` resources
*   [`jql build`](jql.md#build) — `--board` resolution reads `boards.json`
*   [`issue link`](issue/links.md#link) — `--type` completion reads `link_types`
*   [Custom fields](custom-fields.md) — the `fields` cache backs `customfield_*` resolution
*   [Output](output.md) — the JSON envelope and exit codes
</content>
