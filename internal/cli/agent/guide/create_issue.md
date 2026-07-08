## create_issue
Goal: Create a Jira issue from a structured payload and capture the new key for follow-up mutations.
When: a brand-new issue is needed and the project key and issue type are known; for subtasks of an existing parent see → `create_subtask` instead.

**Decide**

# target
- Project: `project_key` (alias) or `project.key` — required unless the profile carries a default.
- Type: `issue_type` (alias) or `issuetype.name` — required unless the profile carries a default.
- There are no `--project`, `--project-key`, or `--issue-type` flags on `issue create`. For any non-default project or issue type, use `--json-input`.
- If you do not know the available type names (`Bug`, `Epic`, `Task`, etc.), run `jira cache issuetypes --output=json`.

# body
- Recommended: `--json-input payload.json` with native ADF for `description` (round-trips losslessly).
- Default-backed one-shots: `--summary "..."` (optionally `--assignee me|none|<accountId>|<email>`) — only when the active profile already supplies the project and issue type. An `--assignee` value containing `@` must be a bare, valid email; it is resolved to an account id via a live `/user/search` (no match → not-found, many → ambiguous), so it is rejected under `--dry-run`.
- Lossy human shortcut: `description_markdown` in the payload (converted to ADF; GFM features beyond the supported set degrade).

# guard
- A bare `--dry-run` validates parsing, ADF compatibility, and a cached-priority mismatch, then stops. It has NO screen schema: unknown field names, invalid issue types, and customfield types pass through unchecked — it is a shape check, not proof Jira accepts the payload.
- `--dry-run --validate-remote` fetches createmeta (read-only) and runs the same field-schema and customfield checks a live submit gets: unknown fields fail strict with exit 3, an issue type missing from the project's create screen errors, and known customfield types validate. `data.validated_remotely: true` confirms the fetch ran.
- A `--validate-remote` pass is field-SHAPE validation only, NOT proof of referential validity: Jira can still silently drop a label not in the project's scheme, a parent the hierarchy rejects, or an option a context excludes — while returning 2xx. To confirm application, pass `--verify` on the live write (or read the issue back yourself).
- `--verify` (live writes only; rejected with `--dry-run`) re-fetches the issue after a successful create and diffs the requested fields against what the server applied. Drops surface as `warnings[]` entries of type `field_not_applied` plus a `data.verification` block (`applied` values, `dropped` list, and `unverified` for requested fields the diff cannot observe, e.g. `duedate`). `ok` stays `true` — the write itself succeeded.
- `--adf-strict` rejects any lossy step with exit 3; `--adf-best-effort` degrades silently with warnings.

**Run**
- Canonical: `jira issue create --no-input --json-input payload.json --output=json`
- Verified write: `jira issue create --no-input --json-input payload.json --verify --output=json` (one extra read; check `warnings[]` for `field_not_applied`)
- Server-validated preview: `jira issue create --no-input --json-input payload.json --dry-run --validate-remote --output=json`
- Stdin variant: `cat payload.json | jira issue create --no-input --json-input - --output=json`
- Default-backed one-shot: `jira issue create --no-input --summary "Refactor auth middleware" --assignee me --output=json`
- Preview only: `jira issue create --dry-run --no-input --json-input payload.json --output=json`

Minimal payload:

```json
{
  "summary": "Refactor auth middleware",
  "issue_type": "Task",
  "project_key": "<PROJECT_KEY>",
  "description": {
    "type": "doc", "version": 1, "content": [
      {"type": "paragraph", "content": [{"type": "text", "text": "Description body."}]}
    ]
  }
}
```

Both payload shapes are accepted interchangeably on create and edit: the flat convenience keys shown here, or the Jira-native `{"fields": {...}}` object with wire spellings (`project`, `issuetype`). Prefer the native shape when the payload originates from Jira's API docs or another REST client — the exact `POST /rest/api/3/issue` body works with zero translation, and the identity wire fields (`project`, `issuetype`) are never subject to screen validation. Object-valued system fields additionally accept a bare string in either shape, lifted to one fixed identity key before submission: `project`/`parent` → `{"key": ...}`, `issuetype`/`priority` → `{"name": ...}`, `assignee`/`reporter` → `{"accountId": ...}`, and string elements of `components`/`fixVersions`/`versions` → `{"name": ...}`. Explicit wire objects pass through untouched, and there is no digits-means-id guessing — address by id with an explicit `{"id": ...}` object. Richer payload (every key past the aliases is forwarded verbatim into Jira's `fields` object):

```json
{
  "labels": ["regression", "stress-test"],
  "priority": {"name": "Highest"},
  "duedate": "2026-06-01",
  "customfield_10015": "2026-05-15",
  "environment": {"type": "doc", "version": 1, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "production / linux-amd64"}]}]},
  "components": [{"name": "ui"}],
  "fixVersions": [{"name": "1.1.0"}],
  "assignee_account_id": "712020:00000000-0000-0000-0000-000000000000"
}
```

**Save**
> Requires `--output=json`.
- `data.issue.key` [string, required] — the new issue key (e.g. `<ISSUE_KEY>`); feed into `→ ` `read_issue`, `→ ` `edit_issue`, `→ ` `add_comment`, `→ ` `transition_issue`. The key is nested under `data.issue`, so a top-level `jq -r '.data.key'` extraction returns empty even though the create succeeded — read `.data.issue.key`.
- `data.issue.id` [string] — numeric issue id; `data.issue.self` [string] — REST URL of the new issue.
- `data.dry_run` [bool] — `false` on a real create. On `--dry-run` the payload is validated and no Jira call is made: `data` carries `dry_run: true` and a `preview` object instead of `issue`, so no key is returned.
- `meta.command` [string] — `issue.create`.

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
- `jira cache issuetypes` discovers visible type names and IDs, but it is instance/visibility scoped, not project-create-screen scoped. It cannot answer whether `Story` is available on project `<PROJECT_KEY>`'s create screen; there is no `cache issuetypes --project` or standalone createmeta command yet.

**Behavior**
- Detection of ADF in the payload is **by value shape, not key suffix** — the CLI walks the payload, finds any value whose root matches `{type: "doc", version: N, content: [...]}`, and validates it. Strict mode rejects with the offending node/mark name; best-effort preserves and emits `unknown_adf_node` / `unknown_adf_mark` warnings.
- `--markdown` and `description_markdown` are human convenience layers and are lossy — use them only when you can tolerate the loss.
- For the full ADF document shape and supported nodes/marks, see → `adf_reference`.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `issue type Story not found on the create screen for project <PROJECT_KEY>` (exit 3, `code=issue_type_unknown`) | The `--type` value names no issue type on that project's create screen — resolved against the fetched list in-code, so it's validation, not a 404 | Pick one of the valid type names in `errors[0].suggestions` (or from Jira's create dialog) |
| `screen schema unavailable: project or issue type not found` (exit 3) | The project key is unknown, or there is no live connection to resolve the create screen | Check the project with `cache projects`; confirm connectivity |
| `Operation value must be an Atlassian Document` on `environment` | Passed `environment` as a plain string; on most modern Jira instances it is an ADF field | Re-run with a full ADF doc value for `environment` (same shape as `description`) |
| `Operation value must be an Atlassian Document` on `description` | Plain string for `description` | Wrap in `{type: "doc", version: 1, content: [...]}` or use `description_markdown` |
| `unknown_adf_node` / `unknown_adf_mark` warning | Best-effort run kept an unsupported node | Re-run with `--adf-strict` to surface and fix, or accept the degradation |
| exit 3 with missing-fields error | Required field absent under `--no-input` | Add `summary` + `project_key` + `issue_type` (or rely on profile defaults) |
| `unknown key` rejection from Jira | Used a `*_adf` suffix or any other non-Jira key name | Drop the suffix — pass `description`/`environment`/`customfield_NNNN` bare |

**Next**
- Then: → `read_issue` to confirm rendered fields, or → `transition_issue` to move it off the initial state.
- Subtask of an existing parent? → `create_subtask`.
- Adding context: → `add_comment`, → `attach_file`, → `link_issues`.
- Composes: → `author_adf` (pre-flight a rich description with `adf convert` before submitting).
