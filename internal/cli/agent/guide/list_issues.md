## list_issues
Goal: Page through issues for the active profile — by key set, JQL, board, or the default project — capturing keys for downstream per-issue work.
When: a batch of issue keys is needed for downstream per-issue work and the filter set fits the flag surface; for hand-written JQL or stored queries, see → `search_jql`.

**Decide**

# target
- Default profile filter (whatever `default_project` / `default_board` resolves to): `jira issue list`.
- Ad-hoc JQL: `--jql 'JQL'`.
- Known key set: `--key <ISSUE_KEY>,<OTHER_ISSUE_KEY>` or `--key <PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12`.
- Show the JQL that WOULD run without calling Jira: `--as-jql` (local-only preview, no API call).
- Restrict to one agile board: `--board <NAME>` (exact case-insensitive) or `--board-id <id>` (numeric escape when names collide).
- Restrict by date: `--updated` / `--created` / `--resolved`. Value is a signed relative duration (`-7d`), an absolute `YYYY-MM-DD`, a comparator form (`>=2026-01-01`), or an inclusive `..` range (`2026-01-01..2026-02-01`); bare = lower bound. See → `search_jql` for the full grammar.
- Active tickets on one or more boards: discover board project keys first, then query those projects with `statusCategory != Done`. Use key expansion only after discovery, when you already have a known key set or a deliberate sparse-range probe.

# field set
- Default summary set per row: `key, summary, status, assignee, priority, updated`.
- Full field records for every row in the page: `--detail` (this flag is `issue list` only — `search jql` / `search saved` don't accept it).
- Explicit field selector or wire-shape `fields:["*all"]`: use → `search_jql`; `issue list` does not accept `--fields` or `--full`.

# guard
- `--board` and `--board-id` are mutually exclusive — passing both exits 3.
- Explicit `--board ""` suppresses the configured `default_board` for one invocation.

**Run**
- Default: `jira issue list --output=json`
- With JQL: `jira issue list --jql 'project = <PROJECT_KEY> AND statusCategory != Done' --output=json`
- Active tickets for several board-backed projects:
  ```sh
  jira boards list --output=json
  jira issue list --jql 'project in (<PROJECT_KEY>, <OTHER_PROJECT_KEY>) AND statusCategory != Done ORDER BY updated DESC' --output=json
  ```
- With issue-key ranges: `jira issue list --key <PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12 --output=json`
- With large sparse issue-key ranges: `jira issue list --key <PROJECT_KEY>-1:100,<OTHER_PROJECT_KEY>-1:200 -p 15 --output=json`
- Preview JQL only: `jira issue list --as-jql --output=json`
- Changed in a window: `jira issue list --project <PROJECT_KEY> --updated=-7d --output=json` (use `=` so the leading-dash value is not read as a flag)
- Recently resolved: `jira issue mine --status Done --resolved=-7d --order-by resolved --output=json`
- Full field records: `jira issue list --detail --output=json`
- Board filter (name): `jira issue list --board "Engineering Sprint" --output=json`
- Board filter (id): `jira issue list --board-id 42 --output=json`

**Save**
> Requires `--output=json`.
- `data.issues[].key` [string, required] — feed to → `read_issue`, → `edit_issue`, → `transition_issue`, → `add_comment`, etc.
- `data.issues[]` [object array] — summary set fields by default; full records under `--detail`.
- `meta.pagination.startAt` / `.maxResults` / `.total` / `.isLast` [int / int / int / bool] — paginate until `isLast=true`. Treat `isLast` as authoritative; some Jira search responses report `total=0` or omit a reliable total even when rows are present.

**Preconditions**
- `--board NAME` requires a primed boards cache. Empty cache → exit 3 with `boards cache is empty — run "jira cache boards"`. See → `cache_metadata` for the prime command and → `discover_board` for resolution semantics.
- `--board` resolution is **exact case-insensitive only** — no substring fallback. Ambiguous matches exit 3 with structured `candidates[]`; pass `--board-id` to disambiguate.

**Behavior**
- Board filtering emits `project in (P1, P2, …)` JQL built from the board's cached project keys — the board is not a server-side filter, it expands locally to a project list.
- To find active tickets across multiple boards, use → `discover_board` / `jira boards list` to map board names or ids to project keys, then run JQL with `project in (...) AND statusCategory != Done`. A single `--board` flag scopes one board; it is not the right primitive for combining several boards with an active-status predicate.
- `--key` emits `key = KEY` or `key in (...)`. Single keys, comma lists, repeated flags, and ranges (`<PROJECT_KEY>-1:10`, `<PROJECT_KEY>-1..10`) are accepted. Lists may mix projects (`<ISSUE_KEY>,<OTHER_ISSUE_KEY>`) and separate ranges may mix projects (`<PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12`), but one range may not cross projects (`<PROJECT_KEY>-1:<OTHER_PROJECT_KEY>-100` exits 3). Do not put spaces inside a `--key` value. Expanded key sets are capped at 1000 keys and exit `3` before any Jira request when exceeded.
- `-p N` / `--parallelism N` applies to `--key` lists and ranges. Large key sets are split into bounded search chunks and up to `N` chunks run concurrently. `issue list` returns visible existing issues from sparse ranges in requested-key order; use → `read_issue` (`issue view KEY... -p N`) when every missing key must appear as a per-key error.
- If one `--key` search chunk fails, successful chunks are retained in the error envelope, `data.failed_key_chunks[]` describes failed chunks, and the command exits non-zero.
- `default_board` (profile config, see below) applies implicitly to `issue list` whenever `--board`/`--board-id` is omitted. The flag wins over the default; the default wins exclusively over `default_project` on commands that consume `--board` (no intersection, no union).
- `default_board` is validated **at use-time only** — `config set` accepts any string without checking the cache (which may not exist yet). When the configured `default_board` doesn't resolve, you get `default_board "X" not found in boards cache — run "jira cache boards --refresh" or unset with "jira config set profiles.<profile>.default_board ''"`.

# `default_board` profile config
- Set: `jira config set profiles.default.default_board "Engineering Sprint"`
- Inspect: `jira config get profiles.default.default_board`
- Unset: `jira config set profiles.default.default_board ""`

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `3`, `boards cache is empty` | First-time board use without a primed cache | → `cache_metadata` (run `jira cache boards`) then retry |
| Exit `3`, `candidates[]` of board matches | Ambiguous `--board NAME` across projects | Re-run with `--board-id <id>` from the candidates list |
| Exit `3`, `same project` on `--key` | One range crosses projects, e.g. `<PROJECT_KEY>-1:<OTHER_PROJECT_KEY>-100` | Split it into separate same-project ranges, e.g. `<PROJECT_KEY>-1:100,<OTHER_PROJECT_KEY>-1:100` |
| Exit `3`, `issue key expansion exceeds maximum of 1000 keys` | `--key` expanded past the local safety cap | Split into smaller same-project ranges, or use project/JQL filters instead of probing a huge sparse range |
| Exit `3`, `parallelism must be between 1 and 16` | `-p` / `--parallelism` was outside the supported bound | Re-run with `-p 1` through `-p 16` |
| Exit `3`, both `--board` and `--board-id` set | Mutex violation | Pass exactly one |
| Exit `3`, `default_board "X" not found in boards cache` | Stale or missing cache vs configured default | `jira cache boards --refresh`, or unset the default |

**Next**
- Then: → `read_issue` on any captured `key` for the full typed envelope.
- Then: → `edit_issue`, → `transition_issue`, → `add_comment` on captured keys.
- Alternative: → `search_jql` for direct JQL or saved-query execution.
- Composes: → `discover_board` to enumerate boards and → `cache_metadata` to prime the boards cache before `--board` use.
