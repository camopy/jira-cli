---
title: Creating & editing issues
description: Write issues with create, edit, clone, and delete — flags, payload shapes, and the dry-run preview.
icon: material/file-document-edit-outline
---

# :pencil: Creating & editing issues

Four write verbs: `create` files a new issue, `edit` changes an existing one,
`clone` copies one, `delete` removes one. JSON examples below show the `data`
block only — the envelope wrapper and exit codes live on
[Output](../output.md), and each command links to its reference page for the
complete flag and output-field tables. `--dry-run` is a local preview that
validates the payload's shape and never contacts Jira — it has no screen
schema, so unknown field names pass through. Add `--validate-remote` to a
`create` or `edit` dry-run for a read-only pre-flight that fetches the live
screen (createmeta / editmeta) and runs the same field-schema and custom-field
checks a real submit gets; the envelope confirms with
`data.validated_remotely: true`. A priority name is additionally checked
against the cached site priorities whenever that cache is present, on dry-run
and live alike.

## create

File a new issue. Common fields go on convenience flags — `--project`,
`--type`, `--summary`, `--assignee`, `--priority`, `--label`, `--parent` — and
anything heavier (an ADF description, custom fields) goes in `--json-input`.
Flags layer on top of the JSON file, so an explicit flag overrides the same key
in the payload. Under `--no-input` the project, type, and summary must all be
resolvable from flags, JSON, or profile defaults before the command will submit.

```sh
jira issue create --project PROJ --type Task --summary "Fix the build"
jira issue create --project PROJ --type Bug --summary "Crash on startup" --assignee me --label regression
jira issue create --json-input new-issue.json --dry-run
jira issue create --json-input new-issue.json --output=json
```

The `--json-input` file accepts flat CLI-alias keys or the Jira-native
`{"fields": {...}}` object interchangeably (`description` is an ADF
document; `description_markdown` takes Markdown instead — a lossy shortcut
that can't carry mentions, dates, panels, or status lozenges):

```json
{
  "project_key": "PROJ",
  "issue_type": "Task",
  "summary": "Fix the build",
  "description": {
    "type": "doc",
    "version": 1,
    "content": [
      { "type": "paragraph", "content": [{ "type": "text", "text": "body" }] }
    ]
  }
}
```

A live create returns `data.issue` with the new `id`, `key`, and `self`. A
`--dry-run` returns `data.preview` — the resolved fields the create *would*
submit (`project_key`, `issue_type`, `summary`, `description_adf`, and any
custom fields) with `dry_run: true`.

!!! tip "`create` and `edit` accept the same payload shapes"
    Both commands take either form: the Jira-native shape — a top-level
    `fields` object holding bare field names with wire nesting
    (`{"project": {"key": …}}`, `{"issuetype": {"name": …}}`) — or flat
    convenience keys at the top level (`project_key`, `issue_type`,
    `summary`, plus any bare Jira field name). The CLI normalises either
    into the same submit, so a payload written for one command works on
    the other.

    Object-valued system fields additionally accept a bare string in
    either form, lifted to one fixed identity key before submission:
    `project` and `parent` become `{"key": …}`; `issuetype` and
    `priority` become `{"name": …}`; `assignee` and `reporter` become
    `{"accountId": …}`; and string elements of `components`,
    `fixVersions`, and `versions` become `{"name": …}`. Explicit wire
    objects pass through untouched, and a numeric-looking string is
    never reinterpreted as an id — address by id with an explicit
    `{"id": …}` object.

[Full flags & output fields →](../reference/jira/issue/create.md)

## edit

Change one or more fields on an existing issue. Use `--summary`, `--assignee`,
or `--markdown` for single-field tweaks; pass `--json-input` for
everything else (an ADF description, custom fields, several fields at once). The
JSON payload follows
[Atlassian's `editIssue`](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-issueidorkey-put)
shape — a top-level `fields` object of bare field names. `edit` accepts issue-key
lists and ranges with `-p` / `--parallelism` for bulk field changes.

Bare `jira issue edit PROJ-123` (no field flags) opens `$EDITOR` on the
description. That works for a human at a terminal; under `--no-input` the CLI
refuses it rather than hang on a TTY prompt, and asks for `--summary`,
`--assignee`, `--markdown`, or `--json-input` instead.

```sh
jira issue edit PROJ-123 --summary "new title"
jira issue edit PROJ-123 --assignee me
jira issue edit PROJ-123 --markdown "## Steps\n\n1. Repro\n2. Fix"
jira issue edit PROJ-1..PROJ-10 -p 4 --summary "bulk title"
jira issue edit PROJ-123 --json-input fields.json --dry-run --output=json
```

`--markdown` is the headless way to replace the description without
the editor. It converts Markdown to ADF with the same lossy converter
[`create` uses](#create), so GFM features beyond the supported set degrade — in
the default strict mode a lossy conversion aborts before submission; add
`--adf-best-effort` to keep the converted document and surface a warning. The
same effect is available inside a `--json-input` payload through the
`description_markdown` key. For mentions, dates, panels, status, or tables —
constructs Markdown can't express — pass native ADF under `fields.description`
instead; that round-trips losslessly.

The `--json-input` file wraps the fields:

```json
{ "fields": { "summary": "new title", "labels": ["regression"] } }
```

```json
{ "fields": { "description_markdown": "## Steps\n\n1. Repro\n2. Fix" } }
```

The payload may also be the exact `PUT /rest/api/3/issue/{key}` body, including
a top-level `update` block of add/set/remove operations as a sibling of
`fields`. The block is forwarded verbatim — Jira validates the operation verbs —
and appears in the envelope as `data.update`:

```json
{ "update": { "labels": [{ "add": "triage" }] } }
```

A live edit returns `data.fields` (the validated submission), `data.result`
(Jira's response, usually empty on a 204), and `dry_run: false`. A `--dry-run`
returns the same `data.fields` with `dry_run: true` and no `result` — the call
never reaches Jira.

[Full flags & output fields →](../reference/jira/issue/edit.md)

## clone

Copy an issue into a new one. The clone keeps the source's editable fields
(summary, ADF description, type, priority, labels, components, custom fields)
and drops lifecycle and identity (key, status, created date, comments,
worklogs, links). Override any carried field through `--json-input` — including
`project.key` to land the clone in a different project. The source survives
untouched; reach for `jira issue move` when you want to relocate the original
instead.

A live clone is destructive: it needs `--force` in headless, agent, or
`--no-input` mode, and prompts an interactive terminal otherwise. `--dry-run`
needs no `--force`.

```sh
jira issue clone PROJ-123 --dry-run
jira issue clone PROJ-123 --force
jira issue clone PROJ-123 --json-input overrides.json --force
jira issue clone PROJ-1..PROJ-10 -p 4 --force
```

The `data.issue` field is the **source** key; `data.result` is the **new**
clone — that pairing is easy to misread:

```json
{
  "dry_run": false,
  "issue": "PROJ-123",
  "result": {
    "id": "10407",
    "key": "PROJ-145",
    "self": "https://example.atlassian.net/rest/api/3/issue/10407"
  }
}
```

A `--dry-run` drops `result`, sets `dry_run: true`, and adds
`data.payload.fields` echoing the would-be create body.

[Full flags & output fields →](../reference/jira/issue/clone.md)

## delete

Permanently remove the issue. The key is freed for the project's next issue. If
you want the record preserved, transition it to a terminal status (`Done`,
`Cancelled`) instead. Like `clone`, a live delete needs `--force` in
headless, agent, or `--no-input` mode; multi-key delete requires `--force` even
in an interactive terminal, to avoid a long confirmation loop.

```sh
jira issue delete PROJ-123 --dry-run
jira issue delete PROJ-123 --force
jira issue delete PROJ-123 --force --delete-subtasks
jira issue delete PROJ-1..PROJ-10 -p 4 --force
```

A live delete returns `data.issue` (the key) and `data.result: null` — Jira
answers a successful delete with 204 No Content. A `--dry-run` returns
`dry_run: true` and `data.payload.fields` (empty, since delete carries no field
payload).

!!! danger "Delete is irreversible"
    There is no undo and no recycle bin. The validation pipeline catches shape
    errors, but once the call lands the issue is gone. Confirm the key with
    [`view`](read.md) first, or run `--dry-run`.

!!! warning "Subtasks block delete unless `--delete-subtasks` is set"
    Jira refuses to delete an issue that has subtasks unless the caller asks
    for the cascade. `--delete-subtasks` is a `delete`-only flag.

[Full flags & output fields →](../reference/jira/issue/delete.md)

## See also

*   [Reading issues](read.md) — find the key before you change it
*   [Custom fields](../custom-fields.md) — the field IDs you put in a payload
*   [ADF](../adf.md) — the description document format
*   [Output](../output.md) — the JSON envelope and exit codes
