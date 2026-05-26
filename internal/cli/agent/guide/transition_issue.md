## transition_issue
Goal: Move an issue to a new workflow state by picking an available transition ID for that issue.

**Decide**

# step 1 — list
- Always list first. Transition IDs are **workflow-specific** — they vary per project and per workflow, so the IDs you saw on `KAN-104` are not necessarily valid on `OTHER-1`.

# step 2 — execute
- Pick an `id` from the listed transitions and pass it to `--transition`.

# guard
- `--dry-run` validates the request (issue exists, transition ID present on this issue) without changing state.

**Run**
- List available transitions for an issue: `jira issue transition KEY --output=json`
- Execute the chosen transition: `jira issue transition KEY --transition <id> --output=json`
- Preview only: `jira issue transition KEY --transition <id> --dry-run --output=json`

**Save**
> Requires `--output=json`.
- `data.transitions[].id` [string, required] — pass to `--transition` to execute.
- `data.transitions[].name` [string, required] — human label (e.g. `"In Progress"`, `"Done"`).
- `data.transitions[].to.name` [string, optional] — target status the transition lands in.
- `meta.command` [string] — `issue.transition` on execute; the list call returns the same envelope shape with `data.transitions[]` populated.

**Preconditions**
- Headless minimum under `--no-input`: `--transition <id>` is required to execute.
- The transition must be currently valid for this issue's state — Jira rejects transitions that aren't reachable from the issue's current status, even if they exist elsewhere in the workflow.

**Behavior**
- Listing and executing are the same subcommand (`jira issue transition KEY`); the presence of `--transition <id>` switches modes.
- `--dry-run` runs validation but never sends the state-change request.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Empty `data.transitions[]` from list | Issue has no valid transitions from its current state (terminal status or permissions) | → `read_issue` to confirm status; check permissions on the project workflow |
| `Transition <id> is not available` on execute | ID was correct for a different issue / project / workflow | Re-list with `jira issue transition KEY --output=json` and pick from this issue's actual set |
| exit 3 with missing `--transition` | Tried to execute under `--no-input` without `--transition <id>` | Add `--transition <id>` from a prior list call |

**Next**
- Then: → `read_issue` to confirm the new status.
- Then: → `add_comment` to document why the state changed.
- Composes: → `safe_mutation` (`--dry-run` validates without mutating, matching the rest of the mutation surface).
