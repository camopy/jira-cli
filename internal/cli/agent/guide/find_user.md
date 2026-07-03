## find_user
Goal: Resolve a person's display name or email to their Jira `accountId` — the value an ADF `mention` node requires.
When: authoring a comment or description that @-mentions someone, assigning by identity, or any flow that needs a deterministic person → accountId step. Read-only; never mutates.

**Run**
- By email (deterministic where the address is unique): `jira user search "dev@example.com" --output=json`
- By display name (may return several candidates): `jira user search "Sam" --output=json`

**Read**
- `data.users[]` — every active match, each with `account_id`, `display_name`, `email_address`. Inactive and deleted accounts are excluded.
- `data.count` — branch on it: `1` means resolved; `0` means no match (still `ok: true`, exit 0 — an empty list is a successful search); `2+` means disambiguate on `email_address` before using an id.

**Mention round-trip (email → accountId → mention node)**

1. `jira user search "dev@example.com" --output=compact` → take `users[0].account_id`.
2. Splice it into a mention node inside the comment body:

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "cc "},
  {"type": "mention", "attrs": {"id": "<account_id from step 1>", "text": "@Dev Name"}}
]}
```

3. Submit via `--json-input` on → `add_comment` / → `edit_issue` (mentions have no Markdown spelling — see → `adf_reference`).
4. The reverse direction also holds: → `list_comments` returns native ADF bodies, so an existing comment's mention nodes carry their `attrs.id` intact for reuse.

**Recover**
- `data.count` is `0` for a known-real person: the query matches Jira's user search semantics (prefix/name parts), not substrings — try the email, or a shorter name fragment.
- Ambiguous name (`count` 2+): re-query with the email, or pick by `email_address` from the returned candidates; never guess between ids.
