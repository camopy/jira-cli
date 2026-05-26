## clone_issue
Goal: Duplicate an existing issue (same project or different) by reading its fields, sanitizing lifecycle/timing noise, and POSTing a fresh issue — optionally with caller-supplied overrides.

**Decide**
- Same project, no field changes? Straight clone — `--force` only.
- Same project but want a different summary, assignee, or stripped fields? Merge a `--json-input` override; caller fields win over source fields.
- Different project? Override `project.key` (and any required-field changes the target project mandates).
- Clear an inherited field on the copy? Set it to `null` in the override.
- Need a preview that runs the full validation pipeline but never POSTs? `--dry-run` (still requires `--force` in agent context — see → `safe_mutation`).

**Run**
- Canonical: `jira issue clone KAN-1 --force --output=json`
- With overrides: `jira issue clone KAN-1 --force --json-input /tmp/over.json --output=json`
- Preview (full validation, no POST): `jira issue clone KAN-1 --force --dry-run --output=json`

Override file shape (override fields merge on top of carried source fields):

```json
{"fields": {"summary": "Triage copy of KAN-1", "assignee": {"accountId": "<your-id>"}}}
```

Different-project clone:

```json
{"fields": {"project": {"key": "OTHER"}, "summary": "Ported from KAN-1"}}
```

**Save**
> Requires `--output=json`.
- `data.key` [string, required] — key of the new issue (feed to → `read_issue`, → `edit_issue`, → `transition_issue`).
- `data.id` [string, required] — numeric id of the new issue.
- `data.self` [string] — REST URL of the new issue.

**Behavior**

`issue clone` is a GET → sanitize → POST round-trip. The set of fields it carries vs drops is fixed:

- **Carries**: `summary`, `description`, `issuetype`, `project`, `priority`, `assignee`, `labels`, `components`, `fixVersions`, `affectedVersions`, `duedate`, all `customfield_*` (except lexorank-shaped values — Jira Software's Rank field is auto-assigned on the new issue).
- **Drops**: identifiers (`id`, `key`, `self`), lifecycle (`status`, `statusCategory`, `statuscategorychangedate`, `resolution`, `resolutiondate`, `created`, `updated`, `creator`, `reporter`, `lastViewed`, `issuerestriction`), time-tracking (`timeestimate`, `timespent`, `timeoriginalestimate`, `workratio`, `progress`, `timetracking`, `aggregate*`), positioning (`rankBeforeIssue`, `rankAfterIssue`), and collections (`comment`, `worklog`, `subtasks`, `attachment`, `votes`, `watches`, `issuelinks`).

Override merge rule: caller `--json-input` fields overwrite the carried source value; explicit `null` strips a carried field.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `validation_error` requesting `--force` | Agent / non-TTY without `--force` | Add `--force`. See → `safe_mutation`. |
| `INVALID_INPUT (400)` after a clean `--dry-run` | Target project requires a customfield the source didn't carry | Add the missing field to the override, re-run. → `cache_metadata` to discover the field id. |
| Cloned issue is missing the Rank | Expected — Jira Software auto-assigns Rank on insert | No action; reorder via the board / `customfield_NNNN` if needed. |

**Next**
- Then: → `edit_issue` to tweak the new issue, → `link_issues` to relate it back to the source.
- Composes: → `safe_mutation` (destructive workflow contract).
- Alternative: → `move_issue` if you want to relocate the original rather than copy it.
