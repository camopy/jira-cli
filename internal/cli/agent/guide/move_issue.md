## move_issue
Goal: Swap an existing issue's `project` and/or `issuetype` in place, providing any new required fields the destination demands — without creating a new issue or changing the issue key history.
When: the project or issue type on an existing issue is wrong and the team needs to preserve the key, history, comments, and link graph.

**Decide**
- Just project change? Override `project.key`.
- Just type change? Override `issuetype.name` (or `.id`).
- Both? Pass both in the same override.
- Target project / type requires fields the source doesn't have? Include them in the same override (e.g. `customfield_10010`).
- Want to confirm the override is well-formed before submitting? `--dry-run`.

**Run**
- Canonical: `jira issue move <ISSUE_KEY> --force --json-input /tmp/move.json --output=json`
- Bulk move: `jira issue move <PROJECT_KEY>-1..10 -p 4 --force --json-input /tmp/move.json --output=json`
- Preview: `jira issue move <ISSUE_KEY> --force --json-input /tmp/move.json --dry-run --output=json`

Minimum override shape:

```json
{"fields": {"project": {"key": "<TARGET_PROJECT_KEY>"}, "issuetype": {"name": "Story"}}}
```

**Save**
> Requires `--output=json`.
- Single-key live submit returns the moved issue under `data.result`; multi-key move returns ordered `data.results[]`, with each successful entry carrying `data.result`.

**Preconditions**
- `--json-input` is required — `move` has no field flags. The destination shape is the entire contract.
- `--force` (or `--dry-run`) is required in agent context — see → `safe_mutation`.

**Behavior**
- No new issue is created; the original key (or its remapped successor on instances that renumber across projects) carries forward, along with comments, worklogs, and attachments.
- Required-field changes between projects / issuetypes must appear in the override. If the target project mandates a field the source didn't have, the submit fails with `INVALID_INPUT (400)`.
- `-p` / `--parallelism` is bounded to 1..16 and applies to multi-key move. Multi-key live move requires `--force`.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `INVALID_INPUT (400)` after `--dry-run` was clean | Target project requires a field not in the override | Add the missing field; → `cache_metadata` (`fields` cache) to discover its id. |
| `validation_error` requesting `--force` | Agent / non-TTY without `--force` | Add `--force`. See → `safe_mutation`. |

**Next**
- Then: → `read_issue` to confirm the post-move shape, → `transition_issue` if the new project's workflow needs a status sync.
- Composes: → `safe_mutation`.
- Alternative: → `clone_issue` if you want a copy in the new project while leaving the original in place.
