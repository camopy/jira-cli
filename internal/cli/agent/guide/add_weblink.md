## add_weblink
Goal: Attach a remote URL (with display title) to a Jira issue via the remote-link endpoint, rejecting non-web schemes client-side.
When: external context (a PR, design doc, dashboard) belongs on the issue without uploading the file itself.

**Decide**

# inputs
- `--url <http(s) URL>` — required.
- `--title <text>` — optional display label; what the user sees in the issue UI.

# scheme guard
- `--url` must use `http://` or `https://` (case-insensitive). Other schemes are rejected before any HTTP call.

**Run**
- `jira issue weblink KEY --url "https://example.com/spec" --title "Spec doc" --output=json`

**Save**
> Requires `--output=json`.
- Standard envelope; `data` carries the persisted remote-link record. Use the response only to confirm the call succeeded — agents typically don't need to dereference the remote-link id.

**Preconditions**
- Calls `POST /rest/api/3/issue/{KEY}/remotelink` — a different endpoint from → `link_issues`. Do not confuse with issue-to-issue links.
- `--url` is required.

**Behavior**
- The CLI rejects any non-`http(s)` scheme client-side before reaching Jira. Disallowed schemes include: `javascript:`, `file:`, `ftp:`, `data:`, `mailto:` (and any other non-web scheme).
- If you need a non-web link target, use a regular issue comment instead (see → `add_comment`).

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, `code: flag_value_invalid` on `--url` | Non-`http(s)` scheme | Re-run with a `http://` or `https://` URL, or use → `add_comment` to record the non-web reference |
| exit 3, `code: required_flag_missing` | Missing `--url` | Provide `--url` |

**Next**
- Then: → `read_issue` to confirm the remote link surface, or → `add_comment` to call attention to it in the thread.
- Alternative: → `link_issues` when the target is another Jira issue (different endpoint).
