---
title: Attachments
description: Upload, list, download, and delete issue attachments — multi-key uploads, single-target fetches, and the dry-run preview.
icon: material/paperclip
---

# :paperclip: Attachments

Four verbs under `jira issue attachment`: `add` to upload, `list` to find the
ids Jira assigns, then `download` and `delete` to act on one file by that id.
Use it for logs, screenshots, and exports — the binary evidence that doesn't
belong in a comment. JSON examples below show the `data` block only; the
envelope wrapper and exit codes live on [Output](../output.md).

## attachment add

Upload one or more local files to one or more issues. Pass paths positionally
for a quick upload, or use repeated `--file` flags when a path holds
shell-sensitive characters (a comma, a space). `add` takes `KEY...`, so the
same files fan out to every issue you name; add `-p` / `--parallelism` to upload
to several issues at once.

```sh
jira issue attachment add PROJ-123 ./screenshot.png
jira issue attachment add PROJ-123 --file ./report,final.pdf --file ./logs.txt
jira issue attachment add PROJ-1..10 -p 4 --file ./release-notes.md
jira issue attachment add PROJ-123 ./report.pdf --dry-run
```

1.  Each `--file` is exactly one path — unlike a comma-separated list, it won't
   split `report,final.pdf` into two bogus filenames.
2.  Same file, every issue in the range, four uploads in flight at a time.

Every path is checked for existence, regular-file type, and size before any
HTTP call. `--dry-run` runs only that local preflight and uploads nothing — the
`data` carries `dry_run: true` and a `files` array of `{ path, size,
mime_inferred }` instead of the uploaded `attachments`.

A successful upload returns the stored attachments, each with the id you'll need
later:

```json
{
  "key": "PROJ-123",
  "dry_run": false,
  "attachments": [
    {
      "id": "10500",
      "filename": "screenshot.png",
      "mime_type": "image/png",
      "size": 4096,
      "created": "2026-06-01T22:00:29.281-0400",
      "author": { "account_id": "712020:…", "display_name": "John Doe" }
    }
  ]
}
```

!!! warning "100 MB cap"
    Each file — and the running total of a multi-file upload — must stay under
    100 MB, or the command fails validation before contacting Jira. Your tenant
    may also enforce a smaller server-side limit.

[Full flags & output fields →](../reference/jira/issue/attachment/add.md)

## attachment list

List what's attached to an issue and, crucially, the numeric ids Jira assigns —
you need one before you can `download` or `delete`. It accepts `KEY...` (lists
and `PROJ-1..10` ranges); a single key returns `data.attachments`, multiple keys
return ordered `data.results[]`, each entry carrying that issue's `attachments`
and `pagination`.

```sh
jira issue attachment list PROJ-123
jira issue attachment list PROJ-123 --all
jira issue attachment list PROJ-1..10 -p 4 --output=json
```

`--limit` (default `50`) is applied client-side — Jira returns attachments as
part of the issue, so there's no server page to fetch. `--all` returns every
attachment regardless of `--limit`. Each row matches the attachment shape shown
under [`add`](#attachment-add); the pagination block rides in `meta.pagination`
with `total` always known and `isLast: false` signalling a truncated window
(re-run with `--all` — there is no cursor for a client-side window):

```json
{
  "data": {
    "attachments": [ { "id": "10500", "filename": "screenshot.png", "…": "…" } ]
  },
  "meta": {
    "pagination": { "startAt": 0, "maxResults": 50, "total": 2, "isLast": true }
  }
}
```

[Full flags & output fields →](../reference/jira/issue/attachment/list.md)

## attachment download

Fetch one attachment by id and write it to a file. This is single-target — it
takes `KEY ATTACHMENT_ID`, not a key list — because the id identifies exactly
one file. Binary content is **always** written to a file, never stdout, so a
JSON or compact consumer never has bytes spliced into its stream.

```sh
jira issue attachment download PROJ-123 10500
jira issue attachment download PROJ-123 10500 --to ./report.pdf --force
jira issue attachment download PROJ-123 10500 --to ./report.pdf --dry-run
```

Without `--to`, the file lands in the current directory under the server-supplied
filename (falling back to `attachment-<id>` if the response omits one). With
`--to PATH` it writes exactly there. Either way an existing target is protected:
the command fails unless you pass `--force` to overwrite. `--dry-run` validates
the target path and prints the planned write — `data.mode` and `data.target` —
without contacting Jira. A live download returns `written_to` and the `bytes`
count.

[Full flags & output fields →](../reference/jira/issue/attachment/download.md)

## attachment delete

Remove one attachment by id. Like `download` this is single-target
(`KEY ATTACHMENT_ID`); run [`list`](#attachment-list) first to find the id.

```sh
jira issue attachment delete PROJ-123 10500 --dry-run
jira issue attachment delete PROJ-123 10500 --force
```

`--dry-run` previews the deletion and never contacts Jira. Because the delete is
destructive, a live run needs confirmation: in headless, agent, or `--no-input`
mode it requires `--force`, and an interactive terminal prompts before deleting
when `--force` is omitted. Success returns `{ "attachment_id": "10500",
"deleted": true }`.

[Full flags & output fields →](../reference/jira/issue/attachment/delete.md)

## See also

*   [Reading issues](read.md) — `view` shows the attachment count on an issue
*   [Output](../output.md) — the JSON envelope and exit codes
*   [Commenting](comments.md) — the other place files and context get attached
