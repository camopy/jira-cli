## cache_metadata
Goal: Prime, refresh, or clear the per-profile local caches so repeated reads and client-side validation don't hit Jira every call.

**Decide**

# what to prime
- `labels` — autocomplete `--label` values and validate client-side without a round-trip.
- `projects` — validate `project_key` in payloads; list project options without GET-ing every issue.
- `epics` — set `parent.key` to an epic without listing issues to find one.
- `fields` — **required** before authoring custom-field values; this is how you discover `customfield_10010` is "Story Points" and what type it expects.
- `issuetypes` — validate `issue_type` in payloads; tells you which types are subtasks.
- `linktypes` — drives `--type` completion on `jira issue link` and pins the canonical names per instance.
- `boards` — drives `--board` completion; resolves names to project lists for the `project in (...)` JQL clause (see → `discover_board`).

# when to use cache vs live API
- Multiple writes / repeated reads in the same session → cache.
- One-shot read, or you specifically need fresh-from-server data (you just created a label and want to see it) → skip the cache, hit live.

# refresh signal
- Cache is **never auto-refreshed**. Force with `--refresh`, age-gate with `--ttl-minutes N`, or wipe with `jira cache clear`.

**Run**
- Per-resource prime: `jira cache labels --output=json`, `jira cache projects --output=json`, `jira cache epics --output=json`, `jira cache fields --output=json`, `jira cache issuetypes --output=json`, `jira cache linktypes --output=json`, `jira cache boards --output=json`.
- Force refresh: `jira cache fields --refresh --output=json`
- TTL gate (refetch if older than N minutes): `jira cache fields --ttl-minutes 5 --output=json`
- Wipe one: `jira cache clear labels`
- Wipe everything for the active profile: `jira cache clear`
- Recommended once-per-session prime for agents:
  ```sh
  jira cache fields     --refresh --output=json   # so you can map customfield_NNNN → name
  jira cache projects   --refresh --output=json   # so you can validate project keys
  jira cache issuetypes --refresh --output=json   # so you can validate issue_type
  ```
- Re-use without spending tokens on Jira:
  ```sh
  jira cache labels --output=json | jq -r '.data.labels[]'
  ```

**Save**
> Requires `--output=json`.
- `data.<resource>[]` [array, required] — the cached list (`data.labels[]`, `data.projects[]`, `data.fields[]`, `data.issuetypes[]`, `data.epics[]`, `data.link_types[]`, `data.boards[]`).
- `data.from_cache` [bool] — true when read from disk, false when this call hit Jira.
- `data.fetched_at` [string] — RFC3339 timestamp of the most recent fetch.
- `data.count` [int, where applicable] — number of items in the cached list.
- `data.cache_state` [string] — `fresh`, `missing`, `stale`, `malformed`, `refresh`, or `empty`.
- `data.cache_source_state` [string] — the cache state before any fetch this call performed.
- `data.cache_empty` [bool] — true when the fetched or cached resource list is empty.
- `data.profile` [string] — emitted on cache-primer envelopes to confirm which profile's cache was touched.

Envelope shape (using `linktypes` as an example):

```json
{
  "data": {
    "link_types": [
      {"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
      {"id": "10001", "name": "Cloners", "inward": "is cloned by", "outward": "clones"},
      {"id": "10002", "name": "Relates", "inward": "relates to", "outward": "relates to"}
    ],
    "from_cache": true,
    "fetched_at": "2026-05-05T12:00:00Z",
    "count": 3,
    "cache_state": "fresh",
    "cache_source_state": "fresh",
    "cache_empty": false
  }
}
```

**Preconditions**
- Per-profile cache lives under `${XDG_CACHE_HOME:-~/.cache}/jira-cli/<profile>/`. Each subcommand prints the data AND writes it to disk.
- `jira cache fields --output=json` is the canonical way to discover `customfield_NNNN` IDs on a Jira instance — agents should run this once per session before authoring custom-field values.

**Behavior**
- Refresh after these events:

| Event                                                | Refresh        |
|------------------------------------------------------|----------------|
| You just created / renamed / deleted a label         | `labels`       |
| You created / renamed / archived a project           | `projects`     |
| Admin added a new custom field or changed a schema   | `fields`       |
| Admin added / renamed / disabled an issue type       | `issuetypes`   |
| You created / closed an epic                         | `epics`        |
| First call of a fresh session (recommended for `fields`) | as needed   |
| You hit a "not found" on something you know exists   | the relevant resource — your cache is stale |

- Concurrency: both the config/site/profile cache namespace and the config TOML use atomic temp-file + rename writes. Two `jira` invocations running in parallel against the same profile will not corrupt each other's state — concurrent writes serialize cleanly at the filesystem level.
- The `boards` cache primer paginates with safety bounds; see → `discover_board` for `--unbounded` and truncation details.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `not found` on a label / project / type you know exists | Cache is stale | Re-run the relevant `jira cache <resource> --refresh` |
| `data.cache_empty=true` after a fresh prime | The instance genuinely has none of that resource for this profile | Verify with a different profile or via the Jira UI |
| `cache-truncated` warning on `boards` | Default 100-page / 10 000-board bound fired | See → `discover_board` (re-run with `--unbounded`) |
| Custom-field author errors referencing unknown `customfield_NNNN` | `fields` cache never primed this session | `jira cache fields --refresh --output=json`, then re-author |

**Next**
- Then: → `list_issues` and → `discover_board` consume the `boards` cache for `--board` filtering.
- Then: → `create_issue` and → `edit_issue` consume the `fields` / `projects` / `issuetypes` caches for client-side validation.
- Composes: → `core_contract` (cache envelopes follow the same `ok`/`meta`/`data` shape).
