## list_comments
Goal: Walk an issue's comment thread oldest-first, paginating through every page, and detect lossy-ADF surfaces before quoting comment bodies.

**Decide**

# scope
- One page (default `max_results`): `comment list KEY`.
- Specific page size: `--limit N`.
- All pages drained in one call: `--all`.

# delete (force-gated under `--no-input`)
- `comment delete KEY <id> --force --output=json` removes a specific comment; the `<id>` comes from `data.comments[].id` of this workflow or from → `add_comment` `data.comment.id`.

**Run**
- Single page: `jira issue comment list KEY --output=json`
- Sized page: `jira issue comment list KEY --limit 50 --output=json`
- Drain all: `jira issue comment list KEY --all --output=json`
- Delete: `jira issue comment delete KEY 10042 --force --no-input --output=json`

**Save**
> Requires `--output=json`.

`comment list KEY` envelope (oldest-first):

```json
{
  "data": {
    "comments": [
      {"id": "10101", "body": "Markdown rendered text…",
       "author": {"account_id": "...", "display_name": "Alice"},
       "update_author": null,
       "created": "2026-04-01T10:00:00.000+0000",
       "updated": "2026-04-01T10:00:00.000+0000",
       "visibility": null}
    ],
    "pagination": {"total": 142, "start_at": 0, "max_results": 50, "is_last": false, "next_page_token": "50"}
  },
  "warnings": [
    {"type": "adf-lossy-comment", "comment_id": "10103", "lossy_constructs": ["inlineCard", "panel:custom"]}
  ]
}
```

- `data.comments[].id` [string, required] — pass to `comment edit` or `comment delete`.
- `data.comments[].body` [string, required] — markdown rendering; check `warnings[]` for any lossy entries before treating as canonical.
- `data.comments[].author` / `data.comments[].update_author` [object, optional] — `update_author` is `null` when the comment has never been edited.
- `data.comments[].visibility` [object, optional] — `null` for public, otherwise `{type, value}` (role/group restriction).
- `data.pagination.is_last` [bool, required] — stop when `true`.
- `data.pagination.next_page_token` [string, optional] — feed back as paging cursor until `is_last=true`.
- `warnings[].comment_id` + `warnings[].lossy_constructs[]` [array, optional] — if a comment id appears here, its `body` is a degraded markdown projection — re-read with native ADF tooling when fidelity matters.

`comment delete KEY ID --force`:

```json
{"data": {"comment_id": "10042", "deleted": true}}
```

- `data.comment_id` [string, required] — echo of the deleted id.
- `data.deleted` [bool, required] — `true` on success.

**Preconditions**
- Comments are returned oldest-first; agent loops that want "latest" must take the last entry of the last page, not the first.
- `comment delete --force` is mandatory under `--no-input`; without `--force` the command exits 3.

**Behavior**
- `warnings[]` does not change the exit code — the response is still successful; treat lossy markers as a signal to switch to ADF reads, not as failure.
- Pagination is cursor-based via `next_page_token`; do not assume `start_at + max_results` is enough.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `warnings[].type = adf-lossy-comment` | Comment contained an ADF construct without a markdown mapping | Re-fetch with ADF tooling (see → `adf_reference`) before quoting |
| exit 3 on delete | `--no-input` without `--force` | Re-run with `--force` |
| exit 2 (`not_found`) on delete | Wrong issue key or comment id | Re-list and copy `data.comments[].id` verbatim |

**Next**
- Then: → `add_comment` to reply, or `comment edit KEY <id>` with a captured id.
- Composes: → `read_issue` (full-issue review loops fold in this thread walk).
