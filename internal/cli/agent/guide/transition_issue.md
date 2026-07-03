## transition_issue
Goal: Move an issue to a new workflow state by picking an available transition ID for that issue.
When: an issue must move to a new workflow state (e.g. To Do → In Progress → Done); the available transition ids depend on the issue's current state and may not be known up front.

**Decide**

# step 1 — list
- Always list first. Transition IDs are **workflow-specific** — they vary per project and per workflow, so the IDs you saw on `<ISSUE_KEY>` are not necessarily valid on `<OTHER_ISSUE_KEY>`.
- List several issues/ranges with `jira issue transition KEY... -p N`; multi-key output uses `data.results[]`.

# step 2 — execute
- Pick an `id` from the listed transitions and pass it to `--transition`.
- Multi-key execution applies the same transition id to each issue and reports per-key failures when one issue cannot take that transition.

# guard
- `--dry-run` validates the request (issue exists, transition ID present on this issue) without changing state.

**Run**
- List available transitions for an issue: `jira issue transition KEY --output=json`
- Multi-key list: `jira issue transition <PROJECT_KEY>-1..10 -p 4 --output=json`
- Execute the chosen transition: `jira issue transition KEY --transition <id> --output=json`
- Bulk execute: `jira issue transition <PROJECT_KEY>-1..10 -p 4 --transition <id> --output=json`
- Preview only: `jira issue transition KEY --transition <id> --dry-run --output=json`
- With a comment, posted atomically: `jira issue transition KEY Done --markdown "released in v1.2.3" --output=json` (or `--markdown-file`)
- With transition-screen fields (e.g. resolution) and/or an ADF comment: `jira issue transition KEY Done --json-input payload.json` where the payload carries `fields` (either payload shape) and an optional ADF `comment` key
- Native REST body: `--json-input` also accepts the exact `POST /rest/api/3/issue/{key}/transitions` body — a `transition` section naming the target (`{"id": "31"}` or `{"name": "Done"}`), `fields`, and an `update` operation block forwarded verbatim. With a payload-named target no positional STATUS is needed; naming the target in both places with different values is refused. `update.comment` operations and the `comment` key are mutually exclusive — supply the comment once.
- Screen rule: fields and comments ride the transition only when the workflow's transition **has a screen**. On a screenless transition (the norm in team-managed projects) Jira accepts the request and silently discards the payload, so the CLI refuses with exit 3 instead — transition bare, then → `add_comment`.

**Save**
> Requires `--output=json`.
- `data.transitions[].id` [string, required] — pass to `--transition` to execute.
- `data.transitions[].name` [string, required] — human label (e.g. `"In Progress"`, `"Done"`).
- `data.transitions[].to.name` [string, optional] — target status when Jira includes it; some tenants only return `id` and `name`.
- `meta.command` [string] — `issue.transitions` on list; `issue.transition` on execute.
- Multi-key list/execute: `data.results[]` [array, required] — ordered by requested key; each successful entry has command-specific `data`.

**Preconditions**
- Headless minimum under `--no-input`: `--transition <id>` is required to execute.
- The transition must be currently valid for this issue's state — Jira rejects transitions that aren't reachable from the issue's current status, even if they exist elsewhere in the workflow.

**Behavior**
- Listing and executing are the same subcommand (`jira issue transition KEY`); the presence of `--transition <id>` switches modes.
- `--dry-run` runs validation but never sends the state-change request.
- `-p` / `--parallelism` is bounded to 1..16 and affects multi-key transition listing and execution.

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
