---
title: Comments
description: Read and write an issue's comment thread — add, list, edit, and delete, with ADF bodies and visibility controls.
icon: material/comment-text-outline
---

# :speech_balloon: Comments

Four verbs for the conversation on an issue: `add` writes one, `list` reads the
thread, `edit` rewrites one by id, `delete` removes one. Comment bodies are
[ADF](../adf.md) — pass Markdown for convenience or native ADF JSON for exact
control. JSON examples below show the `data` block only; the envelope wrapper and
exit codes live on [Output](../output.md), and each command links to its
reference page for the full flag and field tables.

## comment add

Write one comment to one or more issues. Supply the body as `--body-markdown`
(converted to ADF on the way out) or `--json-input` pointing at a native ADF JSON
file — the two are mutually exclusive. The body runs through the
validate-and-encode pipeline before submission; `--dry-run` previews the
validated ADF and never contacts Jira.

```sh
jira issue comment add PROJ-123 --body-markdown "Deployed to staging."
jira issue comment add PROJ-1..10 -p 4 --body-markdown "Release note"
jira issue comment add PROJ-123 --json-input ./comment.json
jira issue comment add PROJ-123 --body-markdown "Internal note." --visibility-role Developers
jira issue comment add PROJ-123 --body-markdown "Draft." --dry-run --output=json
```

1.  `add` takes multiple keys — separate arguments, comma lists, or `A..B`
   ranges. The same body and visibility are applied to each. `-p` / `--parallelism`
   fans the writes out (default `1`, max `16`).

The `data.comment` object carries the created comment with snake-case keys; the
ADF body comes back rendered to Markdown:

```json
{
  "issue": "PROJ-123",
  "comment": {
    "id": "10244",
    "body": "Deployed to staging.\n",
    "author": { "account_id": "712020:…", "display_name": "John Doe", "email_address": "john.doe@example.com" },
    "update_author": { "…": "same shape as author" },
    "visibility": null,
    "created": "2026-05-27T07:13:03.338-0400",
    "updated": "2026-05-27T07:13:03.338-0400"
  },
  "dry_run": false
}
```

A single key returns the `data` above; multiple keys return ordered
`data.results[]`, each with `ok` and either the per-issue `data` or an `error`.

!!! info "Legacy shorthand"
    `jira issue comment PROJ-123 --body-markdown "…"` (no subcommand) still works
    as an alias of `comment add` and builds an identical request.

[Full flags & output fields →](../reference/jira/issue/comment/add.md)

## comment list

Read the thread, or find a comment id before editing or deleting. `list` takes
the same key lists and ranges as [`issue view`](read.md#reading-many-at-once).
`--limit` sets the page size (default `50`); `--all` walks every page until Jira
reports the last one.

```sh
jira issue comment list PROJ-123
jira issue comment list PROJ-123 --all
jira issue comment list PROJ-1..10 -p 4
jira issue comment list PROJ-123 --output=json
```

Human output is one row per comment, newest pages last:

```text
Comments  (2 comments)
#10244    John Doe       2026-05-27T07:13:03.338-0400  Deployed to staging.
#10245    John Doe       2026-05-27T07:13:03.697-0400  Rollout complete.
```

The JSON `data` carries the rendered comments and a pagination block:

```json
{
  "comments": [
    { "id": "10244", "body": "Deployed to staging.\n", "author": { "…": "…" }, "update_author": { "…": "…" }, "visibility": null, "created": "…", "updated": "…" }
  ],
  "pagination": { "is_last": false, "max_results": 50, "next_page_token": "", "start_at": 0, "total": 2 }
}
```

Multiple keys return `data.results[]`; each successful entry's `data` holds that
issue's `comments`, `pagination`, and any per-key `warnings`.

!!! warning "Lossy ADF renders surface as warnings"
    Bodies are rendered to Markdown for human and JSON output. A comment whose
    ADF can't round-trip cleanly is listed under `warnings[]` (`type:
    adf-lossy-comment`) with the constructs that were dropped. If `--all` stops on a
    rate limit, `pagination.is_last` stays `false`, `next_page_token` carries the
    resume cursor, and a `rate-limit-during-paginate` warning is added.

[Full flags & output fields →](../reference/jira/issue/comment/list.md)

## comment edit

Rewrite one comment by id — `edit KEY COMMENT_ID` is single-target. Run
[`comment list`](#comment-list) first to find the id. The replacement body takes
the same `--body-markdown` / `--json-input` pair as `add`, and visibility can be
replaced (`--visibility-role` / `--visibility-group`) or removed
(`--clear-visibility`). `--dry-run` previews the validated body and visibility
change without contacting Jira.

```sh
jira issue comment edit PROJ-123 10042 --body-markdown "Updated: rollout complete."
jira issue comment edit PROJ-123 10042 --body-markdown "Now public." --clear-visibility
jira issue comment edit PROJ-123 10042 --json-input ./comment.json --dry-run
```

A live edit returns the updated comment under `data.comment` — the same shape as
[`comment add`](#comment-add).

[Full flags & output fields →](../reference/jira/issue/comment/edit.md)

## comment delete

Remove one comment by id — `delete KEY COMMENT_ID` is single-target. `--dry-run`
previews the deletion and never contacts Jira.

```sh
jira issue comment delete PROJ-123 10042 --dry-run
jira issue comment delete PROJ-123 10042 --force
```

The `data` block confirms the removal:

```json
{ "comment_id": "10042", "deleted": true }
```

!!! warning "Destructive — `--force` required when headless"
    A live delete in headless, agent, or `--no-input` mode must pass `--force`,
    or the command errors before calling Jira. An interactive terminal is
    prompted to confirm when `--force` is omitted.

[Full flags & output fields →](../reference/jira/issue/comment/delete.md)

## See also

*   [Reading issues](read.md) — view, list, and mine
*   [ADF](../adf.md) — the comment-body document format
*   [Output](../output.md) — the JSON envelope and exit codes
