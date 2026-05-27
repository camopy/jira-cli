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
- Note `--detail` is NOT accepted on `search jql` / `search saved` — that flag belongs to → `list_issues`.

# preview without calling Jira
- `jira issue list --as-jql --output=json` returns the builder output without an API call.

**Run**
- Hand JQL: `jira search jql 'assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC' --output=json`
- Saved query: `jira search saved my-open-bugs --output=json`
- Build then run:
  ```sh
  JQL=$(jira jql build --project KAN --assignee me --output=json | jq -r '.data.jql')
  jira search jql "$JQL" --output=json
  ```
- Builder examples:
  ```sh
  jira jql build --project KAN --status Done --assignee me --output=json
  # → {"jql": "project = KAN AND assignee = currentUser() AND status = Done ORDER BY updated DESC"}

  jira jql build --project KAN --label regression --label hotfix --type Bug --type Task --output=json
  # → {"jql": "project = KAN AND labels in (regression, hotfix) AND issuetype in (Bug, Task) ORDER BY updated DESC"}

  jira jql build --project KAN --order-by updated --desc --output=json
  # → {"jql": "project = KAN ORDER BY updated DESC"}
  ```

**Save**
> Requires `--output=json`.
- `data.issues[].key` [string, required] — feed to → `read_issue`, → `edit_issue`, → `transition_issue`, etc.
- `data.jql` [string, required on `jql build`] — the constructed JQL string; pipe to `search jql` or pass to `issue list --jql`.
- `meta.pagination.startAt` / `.maxResults` / `.total` / `.isLast` [int / int / int / bool] — paginate until `isLast=true`.

**Behavior**
- Builder flag translations (so you don't hand-quote):

| Flag                        | Translates to                          |
|-----------------------------|----------------------------------------|
| `--assignee me`             | `assignee = currentUser()`             |
| `--reporter me`             | `reporter = currentUser()`             |
| Repeated `--label X`        | `labels in (X, Y, Z)`                  |
| Repeated `--type X`         | `issuetype in (X, Y, Z)`               |
| Repeated `--status X`       | `status in (X, Y, Z)`                  |
| `--order-by F --desc`       | `ORDER BY F DESC` (default)            |
| `--order-by F --desc=false` | `ORDER BY F ASC`                       |
| no flags                    | `updated >= -365d ORDER BY updated DESC` |

- Saved queries live as files with optional YAML frontmatter:
  ```text
  ---
  name: my-open-bugs
  description: Bugs assigned to me, not done
  project: KAN
  ---
  project = KAN AND issuetype = Bug AND assignee = currentUser() AND statusCategory != Done
  ORDER BY priority DESC, updated DESC
  ```

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `3`, `invalid order-by field` | `--order-by` value not on the allow-list (or contains shell metachars like `'updated; DROP TABLE x'`) | Use a vetted field name; see → `jql_reference` |
| Exit `3` at flag-parse on `--label`/`--type`/`--status` | Unbalanced quotes in the value | Strip the bad quote before re-running |
| Exit `3`, Jira `400` on unknown function/field | Hand-authored JQL references a field/function this instance does not expose | Cross-check operators, keywords, functions in → `jql_reference` |
| Zero `data.issues[]` | Query is well-formed but matches nothing | Loosen the JQL; reconfirm `project`/`assignee` values |

**Next**
- Then: → `read_issue` on any captured key for the typed envelope.
- Then: → `edit_issue`, → `transition_issue`, → `add_comment` on captured keys.
- Alternative: → `list_issues` for the default-project / board-filtered convenience surface.
- Reference: → `jql_reference` for operators, keywords, functions, and high-yield recipes.
