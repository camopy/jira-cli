## create_issue
Goal: Create a Jira issue from a structured payload and capture the new key for follow-up mutations.
When: a brand-new issue is needed and the project key and issue type are known; for subtasks of an existing parent see → `create_subtask` instead.

**Decide**

# target
- Project: `project_key` (alias) or `project.key` — required unless the profile carries a default.
- Type: `issue_type` (alias) or `issuetype.name` — required unless the profile carries a default.

# body
- Recommended: `--json-input payload.json` with native ADF for `description` (round-trips losslessly).
- Convenience one-shots: `--summary "..."` (optionally `--assignee me|none|<accountId>`) — bypasses `--json-input`.
- Lossy human shortcut: `description_markdown` in the payload (converted to ADF; GFM features beyond the supported set degrade).

# guard
- `--dry-run` runs every validation stage (parse → ADF compat → field schema → customfield encoding) but stops before the API call.
- `--adf-strict` rejects any lossy step with exit 3; `--adf-best-effort` degrades silently with warnings.

**Run**
- Canonical: `jira issue create --no-input --json-input payload.json --output=json`
- Stdin variant: `cat payload.json | jira issue create --no-input --json-input - --output=json`
- Quick one-shot: `jira issue create --no-input --summary "Refactor auth middleware" --assignee me --output=json`
- Preview only: `jira issue create --dry-run --no-input --json-input payload.json --output=json`

Minimal payload:

```json
{
  "summary": "Refactor auth middleware",
  "issue_type": "Task",
  "project_key": "KAN",
  "description": {
    "type": "doc", "version": 1, "content": [
      {"type": "paragraph", "content": [{"type": "text", "text": "Description body."}]}
    ]
  }
}
```

Richer payload (every key past the aliases is forwarded verbatim into Jira's `fields` object):

```json
{
  "labels": ["regression", "stress-test"],
  "priority": {"name": "Highest"},
  "duedate": "2026-06-01",
  "customfield_10015": "2026-05-15",
  "environment": {"type": "doc", "version": 1, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "production / linux-amd64"}]}]},
  "components": [{"name": "ui"}],
  "fixVersions": [{"name": "1.1.0"}],
  "assignee_account_id": "712020:ff38cf6b-faa6-42ae-aa4b-20a2108cfc0f"
}
```

**Save**
> Requires `--output=json`.
- `data.key` [string, required] — the new issue key (e.g. `KAN-104`); feed into `→ ` `read_issue`, `→ ` `edit_issue`, `→ ` `add_comment`, `→ ` `transition_issue`.
- `data.self` [string, optional] — REST URL of the new issue.
- `meta.command` [string] — `issue.create`; on `--dry-run` the payload is validated and no Jira call is made.

**Preconditions**
- Native ADF is the canonical wire shape. Use the **bare Jira field name** (`description`, `environment`, `customfield_NNNN`) when passing an ADF document — there is no `*_adf` convention; the CLI does not rename keys, and Jira rejects unknown keys.
- Aliases the CLI translates server-side:

  | Alias                  | Translates to               |
  |------------------------|-----------------------------|
  | `project_key`          | `project.key`               |
  | `issue_type`           | `issuetype.name`            |
  | `description_markdown` | `description` (ADF, lossy)  |
  | `assignee_account_id`  | `assignee.accountId`        |

- Headless minimum under `--no-input`: `summary` + `project_key` + `issue_type` (or defaults from the profile).
- Prime custom-field metadata before authoring values: → `cache_metadata` (`cache fields`, `cache projects`, `cache issuetypes`).

**Behavior**
- Detection of ADF in the payload is **by value shape, not key suffix** — the CLI walks the payload, finds any value whose root matches `{type: "doc", version: N, content: [...]}`, and validates it. Strict mode rejects with the offending node/mark name; best-effort preserves and emits `unknown_adf_node` / `unknown_adf_mark` warnings.
- `--body-markdown` and `description_markdown` are human convenience layers and are lossy — use them only when you can tolerate the loss.
- For the full ADF document shape and supported nodes/marks, see → `adf_reference`.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `screen schema could not be resolved in strict mode: pipeline: project/issue-type schema unknown` | Sent the wire-envelope shape (`{"fields": {"project": {"key": "..."}, "issuetype": {"name": "..."}, ...}}`) — that is the edit_issue shape, not create_issue | Rewrite with flat top-level alias keys: `project_key`, `issue_type`, `summary`, `description`. No `fields` wrapper |
| `Operation value must be an Atlassian Document` on `environment` | Passed `environment` as a plain string; on most modern Jira instances it is an ADF field | Re-run with a full ADF doc value for `environment` (same shape as `description`) |
| `Operation value must be an Atlassian Document` on `description` | Plain string for `description` | Wrap in `{type: "doc", version: 1, content: [...]}` or use `description_markdown` |
| `unknown_adf_node` / `unknown_adf_mark` warning | Best-effort run kept an unsupported node | Re-run with `--adf-strict` to surface and fix, or accept the degradation |
| exit 3 with missing-fields error | Required field absent under `--no-input` | Add `summary` + `project_key` + `issue_type` (or rely on profile defaults) |
| `unknown key` rejection from Jira | Used a `*_adf` suffix or any other non-Jira key name | Drop the suffix — pass `description`/`environment`/`customfield_NNNN` bare |

**Next**
- Then: → `read_issue` to confirm rendered fields, or → `transition_issue` to move it off the initial state.
- Subtask of an existing parent? → `create_subtask`.
- Adding context: → `add_comment`, → `attach_file`, → `link_issues`.
