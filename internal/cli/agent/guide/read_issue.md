## read_issue
Goal: Fetch one or more issue JSON envelopes so downstream workflows can inspect Jira issue objects without leaving the CLI contract.
When: known issue keys need full payloads for downstream reasoning — transitions, comment review, customfield extraction, or batch inspection.

**Decide**
- Single issue, known key: `jira issue view KEY`.
- Several known keys or a range: `jira issue view <ISSUE_KEY> <OTHER_ISSUE_KEY>` or `jira issue view <PROJECT_KEY>-1:5`.
- Safe fan-out for several keys: add `-p N` / `--parallelism N`; default is `1`, maximum is `16`.
- Need the comment thread, attachments, links, or worklog for the same key? → `list_comments`, → `attach_file`, → `link_issues`, → `log_work` instead — `view` does not project those collections.

**Run**
- Canonical: `jira issue view KEY --output=json`
- Multi-key: `jira issue view KEY OTHER-2 --output=json`
- Bounded fan-out: `jira issue view <PROJECT_KEY>-1:5 -p 4 --output=json`
- Try a field-specific lookup when `view` omits a needed field: `jira search jql 'key = KEY' --fields key,issuetype,created --output=json`
- Try full search payload when an explicit field selector is not enough: `jira search jql 'key = KEY' --full --output=json`

**Save**
> Requires `--output=json`.
- Single key: `data.issue` [object, required] — the Jira issue object returned by the API, preserved under the CLI envelope.
- Multi-key: `data.results[]` [array, required] — ordered like the requested keys. Each entry has `key`, `ok`, and either `issue` or `error`.
- Multi-key: `data.succeeded` / `data.failed` [int, required] — per-key summary counts.
- Common single-key values are nested under `data.issue.fields.*`, for example `data.issue.fields.summary`, `data.issue.fields.status.name`, `data.issue.fields.assignee.accountId`, and `data.issue.fields.priority.name`.
- Common multi-key values are nested under `data.results[].issue.fields.*` for entries with `ok: true`.
- Jira custom fields keep their raw IDs under `fields.customfield_NNNNN`.

**Behavior**
- The CLI still emits its standard envelope (`ok`, `meta`, `data`, `errors`, `warnings`). Within each issue object, field names follow Jira's JSON shape, including camelCase keys such as `accountId`.
- Single-key reads preserve the existing `data.issue` shape. Multi-key reads switch to `data.results[]`; do not parse `data.issue` after passing more than one key.
- Multi-key reads do not fail fast. One missing or unauthorized key produces `ok: false`, a non-empty `error` in that result entry, a top-level error envelope, and retained successes for the other keys.
- In `--output=json`, partial-failure envelopes follow the core contract for failures: the whole envelope, including retained `data.results[]` successes, is written to stderr and stdout is empty. Parse stderr on non-zero exits.
- Human output keeps successful rows on stdout. Failed-key diagnostics are emitted on stderr and are bounded; use `--output=json` for the full per-key failure list.
- Issue-key lists and ranges expand locally to at most 1000 keys. Larger expansions exit `3` before credentials, network, or dry-run mutation work starts.
- Other issue-key commands also accept lists/ranges plus `-p` when the same operation can be applied independently per issue: comment add/list, attachment add/list, link create/list, web links, watcher add/remove/list, worklog add/list, edit with explicit fields, clone/move/delete, epic add/remove, and transition list/execute.
- Commands with a single secondary id remain single-key: comment edit/delete, attachment download/delete, and link delete.
- `issue view` does not have a separate raw REST passthrough mode; the command's normal JSON payload is already the Jira issue object wrapped by the CLI envelope.
- To get an issue's browse URL without fetching the whole object, `jira open KEY` (or `jira issue view KEY --web`) reports it at `data.url`, built offline from the profile base URL with no Jira call. The browser is launched only in an interactive session — never for an agent or piped stdin — so the URL stays usable headless.
- `issue view` and `issue list` do not have a `--fields` flag. Their default field sets can omit `issuetype`; absence means the field was not returned, not that the issue has no type.
- For the type catalog (`Bug`, `Epic`, `Task`, etc.), use → `cache_metadata` (`jira cache issuetypes`). Use `search jql --fields issuetype` only when you need the actual type of one existing issue, and verify the returned `fields` object because Jira may omit requested fields.

| Field                              | Typed JSON     |
|------------------------------------|----------------|
| `parent`                           | `data.issue.fields.parent` when Jira returns it |
| `subtasks`                         | `data.issue.fields.subtasks` when Jira returns it |
| `issuetype.name` from `search jql --fields issuetype` | `data.issues[].fields.issuetype.name` when Jira returns it |

- Because `issue view` preserves Jira's issue shape, absence of a key means Jira did not return it for the requested field set/token, not that the CLI projected it away.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `2` (`not_found`) | Wrong key, or the active profile cannot see this issue | Verify the key with → `search_jql` or → `list_issues` under the right project/profile |
| Multi-key envelope has `data.failed > 0` | At least one requested key failed; successes are still present in `data.results[]` | Iterate entries by `ok`; retry or inspect only the failed keys |
| Exit `3`, `issue key expansion exceeds maximum of 1000 keys` | A list or range expanded past the local safety cap | Split the range into smaller invocations or discover keys with → `list_issues` / → `search_jql` first |
| `parallelism must be between 1 and 16` | `-p` / `--parallelism` was outside the supported bound | Re-run with `-p 1` through `-p 16` |
| `parent` / `subtasks` absent from JSON | Jira did not include that field in the returned issue object | Cross-check field visibility/scopes or use → `search_jql` with explicit fields |
| `issuetype.name` / `created` absent/null | The default field set did not include it, or Jira did not expose it even after an explicit selector | Try → `search_jql` with `--fields` / `--full`; if the returned `fields` object is still empty, report the field as unavailable instead of guessing |

**Next**
- Then: → `list_comments` to read the discussion thread on the same key.
- Then: → `transition_issue` to advance workflow state.
- Then: → `edit_issue` to patch fields in place.
- Alternative: → `list_issues` or → `search_jql` when you don't already have the key.
