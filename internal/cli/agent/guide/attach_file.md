## attach_file
Goal: List, upload, download, or remove file attachments on a Jira issue with size-cap and clobber/force guards honored.
When: a binary or non-Jira artifact (log, screenshot, exported report) must travel with the issue, or an existing attachment needs to be pulled down or removed.

**Decide**

# direction
- List: `jira issue attachment list KEY` — oldest-first.
- Add: `jira issue attachment add KEY --file <path>` — multipart upload.
- Download: `jira issue attachment download KEY <id> --to <path>` — writes to disk, never stdout.
- Delete: `jira issue attachment delete KEY <id> --force` — force-gated under `--no-input`.

# guards
- Download is clobber-protected: an existing target file blocks the write unless explicitly overwritten.
- Delete requires `--force` in `--no-input` / agent mode.

**Run**
- List: `jira issue attachment list KEY --output=json`
- Add: `jira issue attachment add KEY --file ./trace.log --output=json`
- Download (named target): `jira issue attachment download KEY 10042 --to ./local.pdf --output=json`
- Download (current dir): `jira issue attachment download KEY 10042 --output=json`
- Delete: `jira issue attachment delete KEY 10043 --force --output=json`

**Save**
> Requires `--output=json`.

`attachment list` envelope:

```json
{
  "data": {
    "attachments": [
      {"id": "10042", "filename": "screenshot.png", "mime_type": "image/png", "size": 84211,
       "author": {"account_id": "...", "display_name": "Test User"},
       "created": "2026-05-04T18:30:00.000+0000"}
    ],
    "pagination": {"total": 1, "start_at": 0, "max_results": 50, "is_last": true, "next_page_token": null}
  }
}
```

`attachment add`:

```json
{
  "data": {
    "attachments": [
      {"id": "10043", "filename": "trace.log", "mime_type": "text/plain", "size": 4012,
       "author": {"account_id": "...", "display_name": "..."}, "created": "..."}
    ]
  }
}
```

`attachment download` reports the written path and bytes:

```json
{"data": {"attachment_id": "10042", "written_to": "./local.pdf", "bytes": 124521, "mode": "output"}}
```

`attachment delete`:

```json
{"data": {"attachment_id": "10043", "deleted": true}}
```

- `data.attachments[].id` [string, required] — feed to `attachment download` and `attachment delete`.
- `data.attachments[].filename` / `.mime_type` / `.size` [string / string / int, required] — server-side metadata.
- `data.attachments[].author` [object, required] — uploader.
- `data.pagination.is_last` / `data.pagination.next_page_token` [bool / string] — paginate `attachment list` until `is_last=true`.
- `data.attachment_id` [string, required on download/delete] — echo of the target id.
- `data.written_to` [string, required on download] — actual disk path written.
- `data.bytes` [int, required on download] — file size on disk after write.
- `data.mode` [string, required on download] — `output` when `--to PATH` was given, else `current-dir`.
- `data.deleted` [bool, required on delete] — `true` on success.

**Preconditions**
- The binary always writes downloads to a file; there is no stdout streaming mode.
- `attachment delete` under `--no-input` requires `--force` (else exit 3).

**Behavior**
- Download is clobber-protected — an existing file at the target path is not overwritten silently.
- Each project can pin its own upload size cap; the per-project cap is enforced by Jira, not the CLI.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 5, upstream 413 | Upload exceeded the per-project attachment size cap; upstream message is preserved verbatim in `errors[].message` | Shrink/split the file; cap is server-set, not CLI-configurable |
| exit 3 on delete | `--force` missing under `--no-input` | Re-run with `--force` |
| exit 3 on download | Target file already exists (clobber guard) | Choose a fresh `--to` path or remove the stale file first |
| exit 2 (`not_found`) | Wrong attachment id | Re-list and copy `data.attachments[].id` verbatim |

**Next**
- Then: → `add_comment` to reference the new attachment in the issue thread.
- Composes: → `read_issue` (attachments are part of the issue evidence trail).
