## search_jql
Goal: Run a JQL query — hand-authored, flag-built, or saved on disk — and capture matching issue keys.
When: a query goes beyond what `issue list` flags can express, a stored query has to be replayed by name, or a flag set needs to be previewed as JQL via `jira jql build`.

**Decide**

# query source
- Hand-authored string: `jira search jql 'JQL'`.
- File-saved query under `~/.config/jira-cli/queries/<name>.jql`: `jira search saved <name>`.
- Built from flags (no hand-quoting): `jira jql build <flags>` — emits the JQL string; pipe into `search jql` or use with `issue list --jql`.

# field set
- Default summary set per row.
- Wire-shape `fields:["*all"]`: `--full`.
- Explicit selector: `--fields key,summary,customfield_10010`.
- Jira may still omit fields the endpoint, token, project, or field screen does not expose. Always inspect the returned `data.issues[].fields` shape; do not infer that a missing requested field exists with an empty value.
- Note `--detail` is NOT accepted on `search jql` / `search saved` — that flag belongs to → `list_issues`.

# preview without calling Jira
- `jira issue list --as-jql --output=json` returns the builder output without an API call.

**Run**
- Hand JQL: `jira search jql 'assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC' --output=json`
- Active board-backed projects:
  ```sh
  jira search jql 'project in (<PROJECT_KEY>, <OTHER_PROJECT_KEY>) AND statusCategory != Done ORDER BY updated DESC' --output=json
  ```
- Saved query: `jira search saved my-open-bugs --output=json`
- Build then run:
  ```sh
  JQL=$(jira jql build --project <PROJECT_KEY> --assignee me --output=json | jq -r '.data.jql')
  jira search jql "$JQL" --output=json
  ```
- Builder examples:
  ```sh
  jira jql build --project <PROJECT_KEY> --status Done --assignee me --output=json
  # → {"jql": "project = <PROJECT_KEY> AND assignee = currentUser() AND status = Done ORDER BY updated DESC"}

  jira jql build --project <PROJECT_KEY> --label regression --label hotfix --type Bug --type Task --output=json
  # → {"jql": "project = <PROJECT_KEY> AND labels in (regression, hotfix) AND issuetype in (Bug, Task) ORDER BY updated DESC"}

  jira jql build --key <PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12 --output=json
  # → {"jql": "key in (<PROJECT_KEY>-1, ..., <PROJECT_KEY>-10, <OTHER_PROJECT_KEY>-1, ..., <OTHER_PROJECT_KEY>-12) ORDER BY updated DESC"}

  jira jql build --project <PROJECT_KEY> --order-by updated --desc --output=json
  # → {"jql": "project = <PROJECT_KEY> ORDER BY updated DESC"}
  ```

**Save**
> Requires `--output=json`.
- `data.issues[].key` [string, required] — feed to → `read_issue`, → `edit_issue`, → `transition_issue`, etc.
- `data.jql` [string, required on `jql build`] — the constructed JQL string; pipe to `search jql` or pass to `issue list --jql`.
- `meta.pagination.startAt` / `.maxResults` / `.total` / `.isLast` [int / int / int / bool] — paginate until `isLast=true`. Treat `isLast` as authoritative; newer Jira search responses can report `total=0` or omit a trustworthy total while still returning rows.

**Behavior**
- Builder flag translations (so you don't hand-quote):

| Flag                        | Translates to                          |
|-----------------------------|----------------------------------------|
| `--assignee me`             | `assignee = currentUser()`             |
| `--reporter me`             | `reporter = currentUser()`             |
| `--key <ISSUE_KEY>`         | `key = <ISSUE_KEY>`                    |
| `--key <PROJECT_KEY>-1:3,<OTHER_PROJECT_KEY>-1:2` | `key in (<PROJECT_KEY>-1, <PROJECT_KEY>-2, <PROJECT_KEY>-3, <OTHER_PROJECT_KEY>-1, <OTHER_PROJECT_KEY>-2)` |
| Repeated `--label X`        | `labels in (X, Y, Z)`                  |
| Repeated `--type X`         | `issuetype in (X, Y, Z)`               |
| Repeated `--status X`       | `status in (X, Y, Z)`                  |
| `--status '<Done'`          | `statusCategory in ("To Do", "In Progress")` |
| `--status '>=In Progress'`  | `statusCategory in ("In Progress", Done)` |
| `--status '!Abandoned'`     | `status != Abandoned`                  |
| `--order-by F --desc`       | `ORDER BY F DESC` (default)            |
| `--order-by F --desc=false` | `ORDER BY F ASC`                       |
| no flags                    | `updated >= -365d ORDER BY updated DESC` |

- Status comparators (`<`, `<=`, `>`, `>=`) operate on the three workflow
  *categories* (`To Do` < `In Progress` < `Done`), not on status names — so
  `<Done` excludes every done-category status (Closed, Resolved, Won't Do, …),
  and the operand must be one of those three category names. Plain names and
  comparators combine as alternatives (OR); `!Status` is AND-ed as an exclusion.
- Saved queries live as files with optional YAML frontmatter:
  ```text
  ---
  name: my-open-bugs
  description: Bugs assigned to me, not done
  project: <PROJECT_KEY>
  ---
  project = <PROJECT_KEY> AND issuetype = Bug AND assignee = currentUser() AND statusCategory != Done
  ORDER BY priority DESC, updated DESC
  ```
- `--key` values accept single keys, comma lists, repeated flags, and ranges using `:` or `..`. Each comma member expands independently, so `<PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12` is valid. One range cannot cross projects: `<PROJECT_KEY>-1:<OTHER_PROJECT_KEY>-100` exits 3. Whitespace inside a `--key` value is rejected. Expanded key sets are capped at 1000 keys and exit `3` before emitting JQL when exceeded.
- Key expansion is for known keys or deliberate sparse-range probes. For discovery questions like "what is active on this board?", start with project/board/JQL filters, then pass discovered keys to → `read_issue` if more detail is needed.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `3`, `invalid order-by field` | `--order-by` value not on the allow-list (or contains shell metachars like `'updated; DROP TABLE x'`) | Use a vetted field name; see → `jql_reference` |
| Exit `3` at flag-parse on `--label`/`--type`/`--status` | Unbalanced quotes in the value | Strip the bad quote before re-running |
| Exit `3`, `same project` on `--key` | One key range crosses projects, e.g. `<PROJECT_KEY>-1:<OTHER_PROJECT_KEY>-100` | Split it into separate ranges: `<PROJECT_KEY>-1:100,<OTHER_PROJECT_KEY>-1:100` |
| Exit `3`, `issue key expansion exceeds maximum of 1000 keys` | `--key` expanded past the local safety cap | Split the key set into smaller builder/list invocations, or use project/JQL filters for discovery |
| Exit `3`, Jira `400` on unknown function/field | Hand-authored JQL references a field/function this instance does not expose | Cross-check operators, keywords, functions in → `jql_reference` |
| Zero `data.issues[]` | Query is well-formed but matches nothing | Loosen the JQL; reconfirm `project`/`assignee` values |
| Requested field is absent/null | Jira did not return that field despite `--fields` / `--full` | Trust the live `fields` object; use available summary fields, or verify the field via Jira UI/API permissions before depending on it |

**Next**
- Then: → `read_issue` on any captured key for the typed envelope.
- Then: → `edit_issue`, → `transition_issue`, → `add_comment` on captured keys.
- Alternative: → `list_issues` for the default-project / board-filtered convenience surface.
- Reference: → `jql_reference` for operators, keywords, functions, and high-yield recipes.
