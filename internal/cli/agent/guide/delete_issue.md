## delete_issue
Goal: Permanently remove an issue (and optionally its subtasks) from Jira after confirming nothing downstream depends on it.
When: a duplicate, mis-filed, or test issue must be removed and the team accepts that the key is gone forever — for project/issue-type fixes that preserve the key, see → `move_issue`.

**Decide**
- Issue has subtasks? You MUST pass `--delete-subtasks` — Jira refuses to delete a parent otherwise. The flag drains parent + every subtask atomically.
- Want to confirm the call is shaped right without hitting Jira? `--dry-run` (still requires `--force` in agent context).
- Operating in a TTY without `--force`? Expect a `huh` prompt that requires typing `Yes, delete` verbatim.
- Operating in agent / `--no-input` mode? `--force` is mandatory — see → `safe_mutation`.

**Run**
- Canonical (agent): `jira issue delete KAN-1 --force --output=json`
- With subtasks: `jira issue delete KAN-1 --force --delete-subtasks --output=json`
- Preview (no Jira mutation): `jira issue delete KAN-1 --force --dry-run --output=json`

**Save**
> Requires `--output=json`.
- `data.key` [string, required] — echo of the deleted key (use as evidence).
- `data.deleted` [bool, required] — `true` once the delete returned 204.
- `data.deleted_subtasks` [array of strings] — keys removed alongside the parent when `--delete-subtasks` is set.

**Preconditions**
- `--force` is mandatory in agent / non-TTY / `--no-input` mode. Omitting it exits `3` with `validation_error`.
- The caller must have permission to delete in the project; otherwise Jira returns `FORBIDDEN (403)`.

**Behavior**
- Deletion is irreversible. There is no undo.
- ⚠ **Subtasks block deletion** — Jira refuses to delete a parent with subtasks unless `--delete-subtasks` is set. Without it, the call fails server-side; with it, the parent + every subtask are removed atomically.
- `--dry-run` is always allowed (TTY or not) and never touches Jira.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `validation_error` (exit `3`) requesting `--force` | Agent / non-TTY without `--force` | Add `--force`. See → `safe_mutation`. |
| `INVALID_INPUT (400)` with subtask reference | Parent has subtasks and `--delete-subtasks` wasn't set | Re-run with `--delete-subtasks`. |
| `NOT_FOUND (404)` | Already deleted, or key doesn't exist | Treat as already-clean in bulk loops. |
| `FORBIDDEN (403)` | Caller lacks delete permission in the project | Switch profile (→ `auth_setup`) or accept the issue stays. |

**Next**
- Then: nothing — the issue is gone. Update any cached search results that referenced the key.
- Composes: → `safe_mutation`.
- Alternative: → `transition_issue` to a terminal `Done` / `Cancelled` state if you want the record preserved, → `edit_issue` to clear sensitive fields without deleting.
