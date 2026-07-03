## add_comment
Goal: Post a comment on a Jira issue, capturing the persisted comment shape (id, authors, timestamps, visibility) for follow-up edits.
When: an issue needs a status update, decision note, or human-readable annotation that doesn't change workflow state or fields.

**Decide**

# body shape
- Native ADF (preferred for agents — lossless): `--json-input <file>` or `--json-input -` with the ADF doc.
- Markdown convenience (lossy — see → `adf_reference` for what survives): `--markdown "<markdown>"`.
- `--json-input` and `--markdown` are mutually exclusive on `comment add`.

# guards
- `--no-input` is required in agent / non-TTY mode; with no body flags it would otherwise open an editor and exit 3.
- `--dry-run` validates and short-circuits before any Jira call.

**Run**
- ADF (file): `jira issue comment KEY --json-input adf.json --no-input --output=json`
- ADF (stdin): `cat adf.json | jira issue comment KEY --json-input - --no-input --output=json`
- Markdown: `jira issue comment KEY --markdown "**heads up**" --no-input --output=json`
- Bulk markdown: `jira issue comment add <PROJECT_KEY>-1..10 -p 4 --markdown "**heads up**" --no-input --output=json`

`adf.json` shape — either the full body wrapped in `{"body": {...}}` or just the ADF doc itself:

```json
{
  "body": {
    "type": "doc", "version": 1, "content": [
      {"type": "heading", "attrs": {"level": 3}, "content": [{"type": "text", "text": "Update"}]},
      {"type": "paragraph", "content": [
        {"type": "text", "text": "Status: "},
        {"type": "text", "text": "blocked", "marks": [{"type": "strong"}]}
      ]}
    ]
  }
}
```

**Save**
> Requires `--output=json`.

`comment add KEY` (and `comment edit KEY ID`) return the persisted comment shape. Multi-key `comment add KEY... -p N` returns ordered `data.results[]`; each successful entry carries the same persisted comment shape under `data`.

```json
{
  "data": {
    "comment": {
      "id": "10042",
      "body": "Updated body…",
      "author": {"account_id": "<original>", "display_name": "Alice"},
      "update_author": {"account_id": "<caller>", "display_name": "Matt"},
      "created": "2026-04-01T10:00:00.000+0000",
      "updated": "2026-05-05T11:22:33.000+0000",
      "visibility": {"type": "role", "value": "Developers"}
    }
  }
}
```

- `data.comment.id` [string, required] — feed to `comment edit KEY <id>` or → `list_comments` `comment delete KEY <id> --force`.
- `data.comment.body` [string, required] — rendered text after Jira persistence.
- `data.comment.author` / `data.comment.update_author` [object, optional] — original author vs the caller who last edited; `update_author` is `null` on initial create.
- `data.comment.created` / `data.comment.updated` [string, required] — RFC3339 timestamps.
- `data.comment.visibility` [object, optional] — `{type, value}` when role- or group-restricted; `null` for public.

**Preconditions**
- ADF doc shape must satisfy → `adf_reference`; unknown fields are rejected upstream by Jira.
- `--markdown` is converted client-side; constructs without an ADF mapping degrade (see → `adf_reference`).

**Behavior**
- The two body flags are mutually exclusive — passing both fails locally with exit 3 before any Jira call.
- ADF payloads round-trip without loss; markdown payloads are best-effort.
- `-p` / `--parallelism` is bounded to 1..16 and applies to multi-key comment add. `comment edit/delete` stay single-key because they also require one comment id.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, `code: required_flag_missing` | `--no-input` with no body flag (editor flow refused) | Re-run with `--json-input` or `--markdown` |
| exit 3, `code: flag_value_invalid` | Both `--json-input` and `--markdown` set | Pick one body flag |
| exit 4 from upstream | Invalid ADF doc (unknown field, bad structure) | Validate against → `adf_reference` and re-run |

**Next**
- Then: → `list_comments` to read the updated thread back, or `comment edit KEY <id>` with the returned `data.comment.id` to revise.
- Composes: → `read_issue` (most comment work happens inside an issue review loop), → `author_adf` (pre-flight a rich body with `adf convert` and pipe it into `--json-input`).
