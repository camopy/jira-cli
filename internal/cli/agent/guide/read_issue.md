## read_issue
Goal: Fetch one issue's typed JSON envelope so downstream workflows have a stable shape to parse.

**Decide**
- Single issue, known key: `jira issue view KEY`.
- Need the comment thread, attachments, links, or worklog for the same key? → `list_comments`, → `attach_file`, → `link_issues`, → `log_work` instead — `view` does not project those collections.

**Run**
- Canonical: `jira issue view KEY --output=json`

**Save**
> Requires `--output=json`.
- `data` [object, required] — the typed issue envelope (`key`, `summary`, `status`, `assignee`, `priority`, `updated`, plus other projected fields).

**Behavior**
- The typed projection covers the common fields. A few fields are not yet mapped into the envelope shape and there is no raw REST passthrough mode to recover them — closing the gap means extending the typed projection.

| Field                              | Typed JSON     |
|------------------------------------|----------------|
| `parent`                           | not projected  |
| `subtasks`                         | not projected  |
| `issuetype.name` on `issue view`   | may be `null`  |

- Because `subtasks` is not projected, there is no CLI-side verification of the subtask list after a → `create_subtask` call.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `2` (`not_found`) | Wrong key, or the active profile cannot see this issue | Verify the key with → `search_jql` or → `list_issues` under the right project/profile |
| `parent` / `subtasks` absent from JSON | Known typed-output gap (not an error) | Treat as unavailable from the CLI for now; do not assume "no subtasks" |
| `issuetype.name` is `null` | Known typed-output gap on some issue types | Cross-check with → `list_issues` (richer projection per row) |

**Next**
- Then: → `list_comments` to read the discussion thread on the same key.
- Then: → `transition_issue` to advance workflow state.
- Then: → `edit_issue` to patch fields in place.
- Alternative: → `list_issues` or → `search_jql` when you don't already have the key.
