# Cache

`jira-cli` keeps a local copy of slow-changing Jira metadata so that
common operations (completion, validation, board scoping, ADF
field-name resolution) don't pay a round trip every time. Caches are
scoped to a `(profile, base_url, config_path)` tuple and live under:

```text
~/.cache/jira-cli/<profile>-<hash>/<resource>.json
```

The base directory resolves OS-natively:

| Platform | Cache directory |
|----------|-----------------|
| Linux/macOS | `$XDG_CACHE_HOME/jira-cli/...` if `XDG_CACHE_HOME` is set to an absolute path, else `~/.cache/jira-cli/...` |
| Windows | `%LocalAppData%\cache\jira-cli\...` (e.g. `C:\Users\You\AppData\Local\cache\jira-cli\...`); `XDG_CACHE_HOME` is honored when set to an absolute path |

Each `jira cache <resource>` command primes the matching `<resource>.json`,
reading from disk when fresh and refetching from Jira when missing,
stale, or `--refresh` is passed.

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

Every primer accepts the same two flags:

*   `--refresh` forces a fetch even when the cache is fresh.
*   `--ttl-minutes <n>` overrides the resource's default freshness
  window for this call.

The envelope shape is consistent across resources: an `ok: true` /
data block carrying `count`, `fetched_at`, `from_cache`,
`cache_state` (`missing` / `fresh` / `stale` / `malformed` / `refresh`
/ `empty`), `profile`, and the resource's payload field.

## Freshness and invalidation

Each resource has its own default freshness window — its TTL, listed in
the table above. The windows run long because completion never blocks on
a network call: it reads whatever is cached and falls back to empty. A
stale cache therefore only skews autocomplete — a board you just created
stays invisible, a label you just deleted lingers — until the next
refresh; completion never reaches for the network to correct it. (Create,
edit, and move validation is separate: it queries Jira's live create /
edit screens, so it is never served by this cache.) Resources that churn
(labels, epics) keep short windows; admin-managed schema (issue types,
statuses, link types, priorities) runs to weeks or months.

A cached resource counts as a miss — and is refetched on the next
freshness-sensitive read — when any of these hold:

*   the cache file is absent;
*   its JSON is malformed;
*   it is older than the resource's TTL, *and* the caller asked for
    fresh data (completion ignores age, so a long TTL never makes
    autocomplete reach for the network);
*   its on-disk schema version no longer matches the running binary — a
    CLI upgrade that changes the cached shape invalidates old entries
    rather than mis-parsing them;
*   you pass `--refresh` or `--force`, or run `cache clear`;
*   the `(profile, base_url, config_path)` tuple changes — a different
    profile or site reads a different cache namespace.

Age alone never triggers a *background* refetch — there is no daemon, and
completion never makes a surprise network call. Refetching happens only
when you run a command that reads with freshness intent: any primer (or
[`cache refresh`](#refresh)) refetches when its cached read misses —
absent, malformed, or past TTL — and `--refresh` / `--force` refetches
even when the cache is still fresh.

!!! note "Human output shape"

    Cache primers emit a single `INF ℹ️` log line with every envelope
    field as a `key=value` pair. Arrays render as `field="[N items]"`
    rather than embedding the structured payload; reach for
    `--output=json` when a consumer needs the per-item detail.
    `cache boards` is the exception: it emits metadata only, no
    array field.

Add `-d` / `--debug` to print the HTTP request/response trace on stderr
(token redacted); stdout keeps the clean envelope. See
[Output](output.md#debug).

## projects

```sh
jira cache projects
jira cache projects --refresh --output=json
```

=== "Human"

    ```text
    INF ℹ️ cache_empty=false cache_source_state=fresh cache_state=fresh count=3 fetched_at=… from_cache=true profile=default projects="[3 items]"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.projects", "timestamp": "…", "request_id": "…" },
      "data": {
        "cache_empty": false,
        "cache_state": "fresh",
        "cache_source_state": "fresh",
        "count": 3,
        "fetched_at": "…",
        "from_cache": true,
        "profile": "default",
        "projects": [
          { "id": "10034", "key": "<PROJECT_KEY>", "name": "Example Project", "project_type": "software" }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

## fields

```sh
jira cache fields
jira cache fields --refresh --output=json
```

`cache fields` populates the system + custom field map used to resolve
`customfield_*` keys when assembling ADF payloads. Each entry carries
`id`, `name`, and a `type` hint (where Jira reports one).

=== "Human"

    ```text
    INF ℹ️ cache_empty=false cache_source_state=fresh cache_state=fresh count=53 fetched_at=… fields="[53 items]" from_cache=true profile=default
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.fields", "timestamp": "…", "request_id": "…" },
      "data": {
        "cache_empty": false,
        "cache_state": "fresh",
        "cache_source_state": "fresh",
        "count": 53,
        "fetched_at": "…",
        "from_cache": true,
        "profile": "default",
        "fields": [
          { "id": "summary", "name": "Summary", "type": "string" },
          { "id": "customfield_10017", "name": "Issue color", "type": "string" }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

## issuetypes

```sh
jira cache issuetypes --output=json
```

Returns every issue type Jira surfaces across visible projects, with
`subtask: true` on the sub-task variants.

=== "Human"

    ```text
    INF ℹ️ cache_empty=false cache_source_state=fresh cache_state=fresh count=15 fetched_at=… from_cache=true issuetypes="[15 items]" profile=default
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.issuetypes", "timestamp": "…", "request_id": "…" },
      "data": {
        "cache_empty": false,
        "cache_state": "fresh",
        "cache_source_state": "fresh",
        "count": 15,
        "fetched_at": "…",
        "from_cache": true,
        "profile": "default",
        "issuetypes": [
          { "id": "10001", "name": "Task", "subtask": false },
          { "id": "10006", "name": "Subtask", "subtask": true }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

## labels

```sh
jira cache labels --output=json
```

Returns the global label list (a flat string array, not objects).

=== "Human"

    ```text
    INF ℹ️ cache_empty=false cache_source_state=fresh cache_state=fresh count=8 fetched_at=… from_cache=true labels="[8 items]" profile=default
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.labels", "timestamp": "…", "request_id": "…" },
      "data": {
        "cache_empty": false,
        "cache_state": "fresh",
        "cache_source_state": "fresh",
        "count": 2,
        "fetched_at": "…",
        "from_cache": true,
        "profile": "default",
        "labels": ["example-label-one", "example-label-two"]
      },
      "errors": [],
      "warnings": []
    }
    ```

## epics

```sh
jira cache epics --output=json
```

Returns visible epics with `key`, `summary`, and `status`. Used by the
issue create flow to map `--epic <key>` to a parent link.

=== "Human"

    ```text
    INF ℹ️ cache_empty=false cache_source_state=fresh cache_state=fresh count=1 epics="[1 items]" fetched_at=… from_cache=true profile=default
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.epics", "timestamp": "…", "request_id": "…" },
      "data": {
        "cache_empty": false,
        "cache_state": "fresh",
        "cache_source_state": "fresh",
        "count": 1,
        "fetched_at": "…",
        "from_cache": true,
        "profile": "default",
        "epics": [
          { "key": "<PROJECT_KEY>-1", "summary": "Example epic", "status": "To Do" }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

## linktypes

```sh
jira cache linktypes --output=json
```

The field is `link_types` (snake_case) in the envelope, mirroring the
Jira REST representation.

=== "Human"

    ```text
    INF ℹ️ cache_empty=false cache_source_state=fresh cache_state=fresh count=4 fetched_at=… from_cache=true link_types="[4 items]" profile=default
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.linktypes", "timestamp": "…", "request_id": "…" },
      "data": {
        "cache_empty": false,
        "cache_state": "fresh",
        "cache_source_state": "fresh",
        "count": 4,
        "fetched_at": "…",
        "from_cache": true,
        "profile": "default",
        "link_types": [
          {
            "id": "10000",
            "name": "Blocks",
            "inward": "is blocked by",
            "outward": "blocks",
            "self": "https://your-site.atlassian.net/rest/api/3/issueLinkType/10000"
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

## boards

`cache boards` primes the on-disk board list but does **not** return
the boards array in the envelope. Use [`boards list`](#boards-list) to
read what was cached.

```sh
jira cache boards
jira cache boards --refresh --unbounded --output=json
```

`--unbounded` walks every page; the default caps at 100 pages /
10 000 boards and sets `truncated: true` if the cap was hit.

=== "Human"

    ```text
    INF ℹ️ boards_count=3 cache_empty=false cache_source_state=fresh cache_state=fresh fetched_at=… from_cache=true primed=true profile=default truncated=false ttl_seconds=2419200
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.boards", "timestamp": "…", "request_id": "…" },
      "data": {
        "boards_count": 3,
        "cache_empty": false,
        "cache_state": "fresh",
        "cache_source_state": "fresh",
        "fetched_at": "…",
        "from_cache": true,
        "primed": true,
        "profile": "default",
        "truncated": false,
        "truncated_reason": "",
        "ttl_seconds": 2419200
      },
      "errors": [],
      "warnings": []
    }
    ```

## boards list

`jira boards list` reads the cached `boards.json` and prints the
board array, with the same `--refresh` / `--unbounded` semantics as
`cache boards` for re-priming when needed.

```sh
jira boards list
jira boards list --refresh --output=json
jira boards list --unbounded --output=json
```

=== "Human"

    ```text
    Boards  (3 boards, source: cache, fetched_at: …)
          1  Example board A           simple  <PROJECT_KEY>
          2  Example board B           simple  <OTHER_PROJECT_KEY>
         35  Example board C           simple  <ANOTHER_PROJECT_KEY>
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "boards.list", "timestamp": "…", "request_id": "…" },
      "data": {
        "boards": [
          { "id": 1, "name": "Example board", "project_keys": ["<PROJECT_KEY>"], "type": "simple" }
        ],
        "cache_empty": false,
        "cache_state": "fresh",
        "fetched_at": "…",
        "from_cache": true,
        "pagination": { "is_last": true, "max_results": 1, "next_page_token": null, "start_at": 0, "total": 1 },
        "truncated": false,
        "truncated_reason": ""
      },
      "errors": [],
      "warnings": []
    }
    ```

### Using a default board

Pin a board to the active profile so `issue list` and `jql build` scope
by it without an explicit `--board` flag:

```sh
jira config set profiles.default.default_board "Example board"
```

The board name is matched case-insensitively against the cache.

## statuses

```sh
jira cache statuses --output=json
```

Returns the instance's workflow statuses (`id`, `name`) from
`GET /rest/api/3/status`. Drives `--status` completion. The list is
instance-wide, not scoped to a project's workflow.

## priorities

```sh
jira cache priorities --output=json
```

Returns the instance's issue priorities (`id`, `name`) from
`GET /rest/api/3/priority`. Drives `--priority` completion.

## refresh

`cache refresh` primes several resources in one pass. With no argument it
covers every resource; pass names to limit it. By default it is
TTL-gated — a resource still inside its window is reported `fresh` and
left untouched — and `--force` refetches everything.

```sh
jira cache refresh                       # every stale resource
jira cache refresh --force               # everything, ignoring freshness
jira cache refresh boards labels         # just these two
jira cache refresh -p 4 --output=json    # up to four at a time
```

Resources are fetched with bounded concurrency: `-p` / `--parallelism`
defaults to `1` (sequential — the rate-limit-safe default); raise it
(up to 16) to fetch in parallel. `--ttl-minutes <n>` overrides every
resource's window for the run, and `--unbounded` lifts the boards page
cap. One resource failing does not abort the rest — the envelope keeps
the successes in `data.results`, lists the failures in `errors[]`, and
exits with the highest per-resource failure code.

The output is the shared multi-key shape used by the batch issue
commands: a per-resource `results[]` keyed by resource name, each
carrying `status` (`fresh` / `refreshed`), `from_cache`, `count`,
`fetched_at`, and `duration_ms`, plus `succeeded` / `failed` totals.

=== "Human"

    A summary line, then one block per successful resource — including
    the `fresh` ones it skipped:

    ```text
    INF ℹ️ total=2 succeeded=2 failed=0
    statuses
    INF ℹ️ count=3 duration_ms=0 fetched_at=… from_cache=false status=refreshed
    priorities
    INF ℹ️ count=3 duration_ms=0 fetched_at=… from_cache=true status=fresh
    ```

    On partial failure, successes stay on stdout and a bounded
    failed-key summary goes to stderr (exit non-zero):

    ```text
    INF ℹ️ total=2 succeeded=1 failed=1
    statuses
    INF ℹ️ count=3 duration_ms=0 fetched_at=… from_cache=false status=refreshed
    ERR ❌ Failed keys total=2 succeeded=1 failed=1 reason="jira not found" keys=fields shown=1 hint="use --output=json for full per-key errors"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.refresh", "timestamp": "…", "request_id": "…" },
      "data": {
        "results": [
          { "key": "statuses", "ok": true, "data": { "count": 3, "duration_ms": 0, "fetched_at": "…", "from_cache": false, "status": "refreshed" } },
          { "key": "priorities", "ok": true, "data": { "count": 3, "duration_ms": 0, "fetched_at": "…", "from_cache": true, "status": "fresh" } }
        ],
        "succeeded": 2,
        "failed": 0
      },
      "errors": [],
      "warnings": []
    }
    ```

    A partial failure sets `ok: false`, drops the failing resource's
    `data` for an `error`, mirrors it into top-level `errors[]`, and
    carries `meta.exit_code`:

    ```json
    {
      "ok": false,
      "meta": { "command": "cache.refresh", "exit_code": 2, "timestamp": "…", "request_id": "…" },
      "data": {
        "results": [
          { "key": "statuses", "ok": true, "data": { "count": 3, "duration_ms": 0, "fetched_at": "…", "from_cache": false, "status": "refreshed" } },
          { "key": "fields", "ok": false, "error": { "type": "not_found", "code": "jira_not_found", "http_status": 404, "provider": "jira", "retryable": false } }
        ],
        "succeeded": 1,
        "failed": 1
      },
      "errors": [
        { "type": "not_found", "code": "jira_not_found", "http_status": 404, "provider": "jira", "retryable": false }
      ],
      "warnings": []
    }
    ```

Unknown resource names are rejected up front with
`code=arg_value_invalid` (exit 3), before any fetch — the same
validation as [`cache clear`](#unknown-resource).

## clear

```sh
jira cache clear                    # remove every cached resource for the profile
jira cache clear projects           # remove just one resource
```

Valid resource names: `labels`, `projects`, `epics`, `fields`,
`issuetypes`, `linktypes`, `boards`, `statuses`, `priorities`.

### clear all

With no argument, every cache file for the active profile is deleted
and the count is returned in `data.removed`.

=== "Human"

    ```text
    INF ℹ️ profile=default removed=2
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.clear", "timestamp": "…", "request_id": "…" },
      "data": { "profile": "default", "removed": 2 },
      "errors": [],
      "warnings": []
    }
    ```

### clear one

With a resource argument, only that file is removed. `data.removed`
is a boolean: `true` when a cached file existed and was deleted,
`false` when there was nothing to remove.

=== "Human"

    ```text
    INF ℹ️ profile=default removed=true resource=projects
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "cache.clear", "timestamp": "…", "request_id": "…" },
      "data": { "profile": "default", "removed": true, "resource": "projects" },
      "errors": [],
      "warnings": []
    }
    ```

### Unknown resource

`cache clear <unknown>` rejects the argument up front with
`code=arg_value_invalid` and exits 3. The error message lists the
valid set:

```json
{
  "ok": false,
  "meta": { "command": "cache.clear", "exit_code": 3, "timestamp": "…", "request_id": "…" },
  "data": null,
  "errors": [
    {
      "type": "validation",
      "code": "arg_value_invalid",
      "message": "unknown cache resource \"bogus\"; valid resources: labels, projects, epics, fields, issuetypes, linktypes, boards, statuses, priorities",
      "hint": "Pass one of the documented positional argument values; run the command with --help for valid choices.",
      "retryable": false
    }
  ],
  "warnings": []
}
```

## Fleet operations

Multiple agents or CI runners hitting the same Jira tenant should not
all `cache <resource> --refresh` at once. Jira rate-limits per-token,
and a fleet-wide synchronised refresh is the fastest way to trip a 429.
Within a single run, prefer [`cache refresh`](#refresh) over a shell
loop of per-resource primers: it keeps concurrency bounded (`-p`,
default sequential) and reports per-resource status in one envelope.

!!! warning "Common mistake"

    `jira cache clear` in a CI prelude on every job — combined with
    parallel jobs — converts every cache miss into a synchronised Jira
    fetch. Cold-start a shared step once at workflow level, then let
    per-job invocations read the warm cache.

Recipes:

*   Warm the cache once in a setup step, then share the cache
    directory (e.g. GitHub Actions `actions/cache` keyed on profile +
    base URL) across the matrix.
*   Stagger explicit `--refresh` calls; the default TTL behaviour
    already amortises across short-lived calls.
*   Honour `errors[0].retry_after_seconds` from a 429 envelope. Don't
    immediately retry in a loop.
*   Avoid `cache clear` (no resource) on shared infrastructure unless
    you control every consumer of the cache directory.

See [Troubleshooting](troubleshooting.md) for the rate-limit
diagnostic flow.

## See also

*   [`issue list`](issue/read.md#list): filter flags rely on the cached
  `projects`, `issuetypes`, `labels`, and `boards` resources for
  completion and validation.
*   [`jql build`](jql.md#build): `--board` resolution reads from the
  cached `boards.json`.
*   [`issue link`](issue/links.md#link): `--type` completion reads
  from the cached `link_types`.
*   [Custom fields](custom-fields.md): the `fields` cache backs
  `customfield_*` resolution when building ADF payloads.
