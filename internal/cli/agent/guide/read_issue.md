## read_issue
Goal: Fetch one issue's JSON envelope so downstream workflows can inspect Jira's issue object without leaving the CLI contract.
When: a single issue's full payload is the input to downstream reasoning — transitions, comment review, customfield extraction — and the issue key is already known.

**Decide**
- Single issue, known key: `jira issue view KEY`.
- Need the comment thread, attachments, links, or worklog for the same key? → `list_comments`, → `attach_file`, → `link_issues`, → `log_work` instead — `view` does not project those collections.

**Run**
- Canonical: `jira issue view KEY --output=json`

**Save**
> Requires `--output=json`.
- `data.issue` [object, required] — the Jira issue object returned by the API, preserved under the CLI envelope.
- Common values are nested under `data.issue.fields.*`, for example `data.issue.fields.summary`, `data.issue.fields.status.name`, `data.issue.fields.assignee.accountId`, and `data.issue.fields.priority.name`.
- Jira custom fields keep their raw IDs under `data.issue.fields.customfield_NNNNN`.

**Behavior**
- The CLI still emits its standard envelope (`ok`, `meta`, `data`, `errors`, `warnings`). Within `data.issue`, field names follow Jira's JSON shape, including camelCase keys such as `accountId`.
- `issue view` does not have a separate raw REST passthrough mode; the command's normal JSON payload is already the Jira issue object wrapped by the CLI envelope.

| Field                              | Typed JSON     |
|------------------------------------|----------------|
| `parent`                           | `data.issue.fields.parent` when Jira returns it |
| `subtasks`                         | `data.issue.fields.subtasks` when Jira returns it |
| `issuetype.name` on `issue view`   | `data.issue.fields.issuetype.name` when Jira returns it |

- Because `issue view` preserves Jira's issue shape, absence of a key means Jira did not return it for the requested field set/token, not that the CLI projected it away.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `2` (`not_found`) | Wrong key, or the active profile cannot see this issue | Verify the key with → `search_jql` or → `list_issues` under the right project/profile |
| `parent` / `subtasks` absent from JSON | Jira did not include that field in the returned issue object | Cross-check field visibility/scopes or use → `search_jql` with explicit fields |
| `issuetype.name` is `null` | Jira did not include or expose the issue type name | Cross-check with → `list_issues` or → `search_jql` |

**Next**
- Then: → `list_comments` to read the discussion thread on the same key.
- Then: → `transition_issue` to advance workflow state.
- Then: → `edit_issue` to patch fields in place.
- Alternative: → `list_issues` or → `search_jql` when you don't already have the key.
