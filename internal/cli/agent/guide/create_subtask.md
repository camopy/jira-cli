## create_subtask
Goal: Create a subtask under an existing parent issue and capture the new child key.
When: an existing parent issue needs a child task and the destination project supports a Subtask-typed child.

**Decide**

# parent
- Parent issue key (e.g. `<PARENT_ISSUE_KEY>`) — must already exist; the CLI does not verify it client-side.

# body
- Same shape and options as → `create_issue` — same `--json-input`, same ADF rules, same alias table, same convenience flags.

# guard
- `--dry-run` validates the payload (including the `parent.key` field schema) without calling Jira.

**Run**
- Canonical: `jira issue create --no-input --json-input subtask.json --output=json`
- Preview only: `jira issue create --dry-run --no-input --json-input subtask.json --output=json`

Subtask payload (note `issue_type: "Subtask"` and `parent.key`):

```json
{
  "summary": "REL: Subtask 1 of <PARENT_ISSUE_KEY>",
  "issue_type": "Subtask",
  "project_key": "<PROJECT_KEY>",
  "parent": {"key": "<PARENT_ISSUE_KEY>"},
  "description": {"type": "doc", "version": 1, "content": [
    {"type": "paragraph", "content": [{"type": "text", "text": "Detail of subtask 1."}]}
  ]}
}
```

**Save**
> Requires `--output=json`.
- `data.issue.key` [string, required] — the new subtask key; feed into `→ ` `read_issue` or downstream mutations. The key is nested under `data.issue` (read `.data.issue.key`, not a top-level field).
- `data.issue.id` [string] — numeric issue id; `data.issue.self` [string] — REST URL of the new subtask.
- `meta.command` [string] — `issue.create` (subtasks share the create envelope).

**Preconditions**
- `issue_type` must be a subtask-style type in the target project (typically `"Subtask"`; some projects rename it). If unsure, prime → `cache_metadata` (`cache issuetypes`) first.
- `parent.key` is required for subtask-style types and must reference an issue in the same project.
- All `create_issue` preconditions apply — same alias table, same bare-field-name rule for ADF.

**Behavior**
- There is **no CLI-side verification of the subtask list** today. The typed `issue view` envelope does not project `subtasks`, so after creation you can confirm the child exists with → `read_issue` on the new key, but you cannot list a parent's subtasks via the typed envelope.
- ADF detection, alias translation, dry-run stages, and warning behavior all match → `create_issue` exactly.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Jira rejects `parent.key` as unknown field | Parent type not configured for subtasks in this project | Verify the project workflow allows subtasks; pick a different `issue_type` |
| `Parent issue does not exist` | Bad `parent.key` (typo, deleted, or wrong project) | Re-run → `read_issue` against the intended parent first |
| Any ADF / alias / required-field error | Same as → `create_issue` | See the `create_issue` Recover table |

**Next**
- Then: → `read_issue` on the returned `data.issue.key` (the parent's subtask list is not exposed via the typed envelope).
- Then: → `link_issues` to add non-parent/child relationships.
- Composes: → `create_issue` (subtask creation is a specialization of the create workflow).
