## list_issues
Goal: Page through issues for the active profile — by JQL, by board, or by the default project — capturing keys for downstream per-issue work.
When: a batch of issue keys is needed for downstream per-issue work and the filter set fits the flag surface; for hand-written JQL or stored queries, see → `search_jql`.

**Decide**

# target
- Default profile filter (whatever `default_project` / `default_board` resolves to): `jira issue list`.
- Ad-hoc JQL: `--jql 'JQL'`.
- Show the JQL that WOULD run without calling Jira: `--as-jql` (local-only preview, no API call).
- Restrict to one agile board: `--board <NAME>` (exact case-insensitive) or `--board-id <id>` (numeric escape when names collide).

# field set
- Default summary set per row: `key, summary, status, assignee, priority, updated`.
- Full field records for every row in the page: `--detail` (this flag is `issue list` only — `search jql` / `search saved` don't accept it).
- Wire-shape `fields:["*all"]`: `--full`.
- Explicit selector: `--fields key,summary,customfield_10010`.

# guard
- `--board` and `--board-id` are mutually exclusive — passing both exits 3.
- Explicit `--board ""` suppresses the configured `default_board` for one invocation.

**Run**
- Default: `jira issue list --output=json`
- With JQL: `jira issue list --jql 'project = KAN AND statusCategory != Done' --output=json`
- Preview JQL only: `jira issue list --as-jql --output=json`
- Full field records: `jira issue list --detail --output=json`
- Board filter (name): `jira issue list --board "Engineering Sprint" --output=json`
- Board filter (id): `jira issue list --board-id 42 --output=json`

**Save**
> Requires `--output=json`.
- `data.issues[].key` [string, required] — feed to → `read_issue`, → `edit_issue`, → `transition_issue`, → `add_comment`, etc.
- `data.issues[]` [object array] — summary set fields by default; full records under `--detail`.
- `meta.pagination.startAt` / `.maxResults` / `.total` / `.isLast` [int / int / int / bool] — paginate until `isLast=true`.

**Preconditions**
- `--board NAME` requires a primed boards cache. Empty cache → exit 3 with `boards cache is empty — run "jira cache boards"`. See → `cache_metadata` for the prime command and → `discover_board` for resolution semantics.
- `--board` resolution is **exact case-insensitive only** — no substring fallback. Ambiguous matches exit 3 with structured `candidates[]`; pass `--board-id` to disambiguate.

**Behavior**
- Board filtering emits `project in (P1, P2, …)` JQL built from the board's cached project keys — the board is not a server-side filter, it expands locally to a project list.
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
| Exit `3`, both `--board` and `--board-id` set | Mutex violation | Pass exactly one |
| Exit `3`, `default_board "X" not found in boards cache` | Stale or missing cache vs configured default | `jira cache boards --refresh`, or unset the default |

**Next**
- Then: → `read_issue` on any captured `key` for the full typed envelope.
- Then: → `edit_issue`, → `transition_issue`, → `add_comment` on captured keys.
- Alternative: → `search_jql` for direct JQL or saved-query execution.
- Composes: → `discover_board` to enumerate boards and → `cache_metadata` to prime the boards cache before `--board` use.
