## discover_board
Goal: Enumerate the agile boards visible to the active profile so `--board` filters in → `list_issues` and `jira jql build` resolve to a known id.
When: a `--board <name>` filter is needed and the board's numeric id is unknown, or the board cache returned no matches for a name the user expects to exist.

**Decide**
- First time on this profile (cache empty): prime it with `jira cache boards`. `boards list` also primes transparently on the first run when the cache is empty.
- Already primed, just want the listing: `jira boards list`.
- Cache exists but you suspect it's stale (new board, renamed board): `jira boards list --refresh` to force a re-prime.
- Need to start over: `jira cache clear boards` drops the cache file; the next call re-primes.
- Very large instance hitting the default safety bound: `--unbounded` to disable.

**Run**
- Explicit prime: `jira cache boards --output=json`
- Listing (envelope or table): `jira boards list --output=json`
- Force refresh: `jira boards list --refresh --output=json`
- Drop the cache: `jira cache clear boards`
- Remove the pagination bound: `jira boards list --unbounded --output=json`

**Save**
> Requires `--output=json`.
- `data.boards[].id` [int, required] — pass as `--board-id <id>` on → `list_issues` (unambiguous escape).
- `data.boards[].name` [string, required] — pass as `--board NAME` on → `list_issues` (exact case-insensitive match only).
- `data.boards[].type` [string, required] — verbatim Jira value: `scrum`, `kanban`, `simple`, `agility`, or any future Atlassian board type (round-trips through the cache without modification).
- `data.boards[].project_keys[]` [string array, required] — the project keys the board expands to in `project in (...)` JQL.
- `data.cache_state` [string] — `fresh`, `missing`, `stale`, `malformed`, `refresh`, or `empty`.
- `data.cache_source_state` [string] — the cache state before any fetch this call performed.
- `data.cache_empty` [bool] — true when the fetched or cached board list is empty.
- `data.from_cache` [bool] — true when the response came from disk.
- `data.fetched_at` [string] — RFC3339 timestamp of the most recent fetch.
- `data.truncated` [bool] / `data.truncated_reason` [string] — set when the safety bound fired.
- `meta.pagination.total` / `.start_at` / `.max_results` / `.is_last` / `.next_page_token` — standard pagination.

Envelope shape:

```json
{
  "data": {
    "boards": [
      {"id": 42, "name": "Engineering Sprint", "type": "scrum",
       "project_keys": ["ENG", "PLAT"]}
    ],
    "pagination": {
      "total": 12, "start_at": 0, "max_results": 12,
      "is_last": true, "next_page_token": null
    },
    "from_cache": true,
    "fetched_at": "2026-05-06T18:30:00Z",
    "truncated": false,
    "truncated_reason": "",
    "cache_state": "fresh",
    "cache_source_state": "fresh",
    "cache_empty": false
  }
}
```

**Behavior**
- Cache primer paginates the full set with safety bounds (default 100 pages / 10 000 boards). Truncation emits a `cache-truncated` warning naming the bound that fired and sets `data.truncated` / `data.truncated_reason`.
- Tab completion on `--board` reads the same cache:
  ```text
  $ jira issue list --board <TAB>
  Engineering Sprint  (scrum, ENG, PLAT)
  Platform Roadmap    (kanban, PLAT)
  ```
- The `default_board` profile setting (see → `list_issues`) is also resolved against this cache.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `3`, `boards cache is empty — run "jira cache boards"` | `--board NAME` used before priming | → `cache_metadata` then retry, or pass `--board-id` |
| Exit `3` with `candidates[]` listing matches | Ambiguous board name across projects | Re-run with `--board-id <id>` from the listed candidates (each entry carries `id`, `name`, `project_keys`) |
| `data.truncated=true`, `cache-truncated` warning | Default page / total bound fired on a very large instance | Re-run with `--unbounded` |
| `default_board "X" not found in boards cache` | Configured default doesn't resolve | `jira cache boards --refresh`, or unset with `jira config set profiles.<profile>.default_board ''` |

**Next**
- Then: → `list_issues` with `--board` or `--board-id`.
- Composes: → `cache_metadata` for the general cache-primer pattern (TTL, refresh, clear, concurrency).
