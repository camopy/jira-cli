# Jira CLI Agent Guide

Comprehensive steering document for AI coding assistants and developers
using `jira-cli`. This file is embedded in the binary and surfaced via
`jira agent guide`. Pair with `jira agent schema --compact` for the
machine-readable command tree, `jira agent adf-matrix --json` for the
ADF support matrix, and `jira agent fieldtypes --json` for the
customfield encoder registry.

The guide assumes you are interacting through the JSON envelope path
(`--json` or auto-detected agent mode); flags and recipes still apply
to TTY humans, just the output rendering differs.

## Identity & profiles

Resolve identity once per profile:

```sh
jira auth whoami --save
```

This calls `/rest/api/3/myself` and persists the resolved `account_id`
to the active profile's TOML entry. After this, `--assignee me`, the
TUI `A` key, and "in my epics" JQL all work without further setup.

Quick checks:

```sh
jira me              # short identity card (uses cached account_id)
jira config profile  # list every configured profile, mark the active one
jira auth status     # per-profile credential health (keyring/1password/env)
```

Switch profile per call:

```sh
jira <cmd> --profile work
```

The default profile is set by `default_profile = "..."` in
`~/.config/jira-cli/config.toml`. `--profile` overrides per call;
`JIRA_PROFILE` is **not** read (config is the source of truth).

## Output modes & envelope

Mode is auto-detected:

| Context           | Default mode       |
|-------------------|--------------------|
| TTY human         | `plain`  (clog rich text) |
| TTY + `--json`    | `json`  (full envelope) |
| Non-TTY (pipe)    | `json`  (full envelope) |
| Detected agent    | `compact`  (envelope, single line, jq-friendly) |

Agents detected via env vars: `CLAUDE_CODE`, `CURSOR_TERMINAL`,
`CURSOR_AGENT`, `COPILOT_CLI`, `COPILOT`, `GITHUB_COPILOT`,
`CODEX_*`, `OPENAI_CODEX`, `GEMINI_CLI`, `OPENCODE`. First match wins
in the order amp → codex → gemini → copilot → opencode → cursor →
claude. `AI_AGENT=<name>` is an explicit override.

Mode flags (mutually exclusive — combining them returns exit 3):

| Flag         | Effect |
|--------------|--------|
| `--json`     | Force the structured envelope on stdout |
| `--compact`  | Force jq-friendly compact JSON |
| `--plain`    | Force human-friendly clog rich text |
| `--raw`      | Emit the underlying Jira REST JSON verbatim (no envelope) |

Every JSON envelope:

```json
{
  "meta": {
    "command": "issue.create",
    "profile": "default",
    "timestamp": "2026-05-04T22:48:55Z",
    "request_id": "...",
    "pagination": { "startAt": 0, "maxResults": 50, "total": 12, "isLast": true }
  },
  "data":     { ... command-specific ... },
  "errors":   [],
  "warnings": []
}
```

`warnings[]` carries non-fatal best-effort diagnostics. Common warning
types you'll see:

| `type`                       | Meaning                                       |
|------------------------------|-----------------------------------------------|
| `unknown_adf_node`           | ADF node outside the MVP set, preserved opaquely |
| `unknown_adf_mark`           | ADF mark outside the MVP set, preserved opaquely |
| `customfield_unknown_type`   | `customfield_NNNN` key forwarded with no registry schema (Jira handles the type) |
| `lossy_adf_conversion`       | markdown → ADF or roundtrip dropped detail   |

In `--plain` mode warnings mirror to stderr as clog `WRN` lines so
stdout stays clean for piping.

### `--compact` and errors

Under `--compact`, the success path strips `meta` and `errors` for
jq-friendliness. **Error paths still emit the full envelope** so
failures stay parseable regardless of mode flags — stripping `errors`
on a failure would leave the failure invisible.

Exit codes (stable contract — never reused for new categories):

| Exit | Meaning                                  |
|------|------------------------------------------|
| 0    | Success                                  |
| 1    | Authentication failure                   |
| 2    | Not found                                |
| 3    | Validation error / mutex flags / no-input gap |
| 4    | Rate limited                             |
| 5    | Server error                             |

## Discovery surface

Three commands let agents introspect the CLI without reading prose:

```sh
jira agent schema [--compact]    # full command tree + flag signatures + per-command JSON output schemas
jira agent guide  [<section>]    # this guide; section is a slug like "issues" or "jql"
jira agent adf-matrix [--json]   # ADF node/mark support matrix
jira agent fieldtypes [--json]   # customfield encoder registry
```

Both `adf-matrix --json` and `fieldtypes --json` emit arrays of the
same envelope shape:

```json
{
  "kind": "node|mark|field-type",
  "name": "paragraph",
  "status": "mvp|preserve-only",
  "capabilities": { "author": true, "render": true, "preserve": true, "validate": true, "submit": true },
  "input_shape": { /* JSON Schema 2020-12 fragment */ },
  "output_shape": { /* JSON Schema 2020-12 fragment */ },
  "warnings": [],
  "official_url": "https://developer.atlassian.com/...",
  "notes": "...",
  "submit_description": "ADF: included in a Jira rich-text field payload after ADF validation passes."
}
```

A single agent parser handles both surfaces.

## Reading issues

```sh
jira issue view KEY            --json
jira issue list                --json
jira issue list --jql 'JQL'    --json
jira issue list --as-jql       --json    # show what JQL would run; no API call
jira search jql 'JQL'          --json
jira search saved <name>       --json    # ~/.config/jira-cli/queries/<name>.jql
```

### `--detail` on `issue list` only

`jira issue list --detail` requests full field records for every issue
in the page. Without it, list returns the summary set
(`key, summary, status, assignee, priority, updated`).

`search jql` does NOT take `--detail` — it always requests
`fields:["*all"]` server-side, returning the full Jira shape.

### `--raw` is the safety valve

Two known **typed-output drops** in the current `view` / `list`
transformations — use `--raw` to recover full fidelity:

| Field          | typed JSON | --raw |
|----------------|-----------|-------|
| `parent`       | dropped   | present (full nested issue shape) |
| `subtasks`     | dropped   | present |
| `issuetype.name` on `issue view` | reported as `null` | present |

Any time you need parent/subtask awareness, use `--raw` and parse
`fields.parent.key` / `fields.subtasks[].key` directly.

Example pattern for parent-aware code:

```sh
jira issue view KEY --raw 2>/dev/null | jq -r '.fields.parent.key // "none"'
```

## Creating issues

**Native ADF is the canonical path** — it's the wire shape Jira
expects, and the CLI round-trips it without lossy conversions.
`--body-markdown` and `description_markdown` exist as
human convenience layers and are lossy — use them only when you can
tolerate the loss (the language attr on fenced code blocks, for one,
*used to* drop silently and is now preserved, but other GFM features
beyond the supported set still degrade).

Recommended invocation:

```sh
jira issue create --no-input --json-input payload.json --json
```

Minimal payload:

```json
{
  "summary": "Refactor auth middleware",
  "issue_type": "Task",
  "project_key": "KAN",
  "description": {
    "type": "doc", "version": 1, "content": [
      {"type": "paragraph", "content": [{"type": "text", "text": "Description body."}]}
    ]
  }
}
```

Aliases the CLI translates server-side:

| Alias                  | Translates to               |
|------------------------|-----------------------------|
| `project_key`          | `project.key`               |
| `issue_type`           | `issuetype.name`            |
| `description_markdown` | `description` (ADF, lossy)  |
| `assignee_account_id`  | `assignee.accountId`        |

**Use the bare Jira field name** (`description`, `environment`,
`customfield_NNNN`) when writing an ADF document. ADF is the canonical
representation Jira's API accepts — pass the ADF doc directly as the
value:

```json
{
  "summary": "...",
  "description": {"type": "doc", "version": 1, "content": [...]}
}
```

The CLI walks the payload, detects any value whose root shape is an
ADF document (`{type: "doc", version: N, content: [...]}`), and runs
client-side validation on each: strict mode rejects with the offending
node/mark name; best-effort preserves with an `unknown_adf_node` /
`unknown_adf_mark` warning. Detection is by value shape, not by key
suffix — there is no `*_adf` convention.

Every other key in the JSON is forwarded verbatim into Jira's
`fields` object. Common ones:

```json
{
  "labels": ["regression", "stress-test"],
  "priority": {"name": "Highest"},
  "duedate": "2026-06-01",
  "customfield_10015": "2026-05-15",
  "environment": {"type": "doc", "version": 1, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "production / linux-amd64"}]}]},
  "components": [{"name": "ui"}],
  "fixVersions": [{"name": "1.1.0"}],
  "assignee_account_id": "712020:ff38cf6b-faa6-42ae-aa4b-20a2108cfc0f"
}
```

⚠ **`environment` is an ADF field** on most modern Jira instances — pass
the full ADF doc, not a plain string. Same for `description`. Plain
strings will be rejected with "Operation value must be an Atlassian
Document".

Convenience flags (good for quick one-shots, bypass `--json-input`):

```sh
jira issue create --no-input --summary "..." --json
jira issue create --no-input --summary "..." --assignee me --json
```

Always start with `--dry-run` if you're not sure about the payload:

```sh
jira issue create --dry-run --no-input --json-input payload.json --json
```

The dry-run runs every validation stage (parse → ADF compat → field
schema → customfield encoding) but stops before the API call.

## Editing issues

**kubectl-style default**: bare `jira issue edit KEY` (no field flags,
no `--json-input`) opens your `$EDITOR` on the description.

Field flags (single-shot edits, no editor):

```sh
jira issue edit KEY --summary "New title" --json
jira issue edit KEY --assignee me --json                 # or --assignee none / accountId
jira issue edit KEY --json-input fields.json --json     # bulk JSON edit
```

Bulk edit payload shape:

```json
{
  "fields": {
    "summary": "New title",
    "labels": ["updated", "v2"],
    "priority": {"name": "Lowest"},
    "duedate": "2026-07-15",
    "description": { "type": "doc", "version": 1, "content": [...] }
  }
}
```

⚠ **`issuelinks` cannot be set via bulk edit** — Jira refuses
`"Field does not support update 'issuelinks'"`. Use `jira issue link`
instead (below).

⚠ **`--no-input` requires at least one field** — empty edits are
validation errors (exit 3), never silent successes:

```sh
jira issue edit KEY --no-input             # ❌ exit 3
jira issue edit KEY --no-input --summary X # ✓ ok
```

## Comments

Native ADF (preferred for agents):

```sh
jira issue comment KEY --json-input adf.json --no-input --json
```

`adf.json` shape — either the full body wrapped in `{"body": {...}}`
or just the ADF doc itself:

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

Markdown convenience (lossy — see the ADF Reference section for what
survives):

```sh
jira issue comment KEY --body-markdown "**heads up**" --no-input --json
```

The two flags are mutually exclusive.

### Comment lifecycle envelopes

`comment list KEY [--all] [--limit N]` → paginated thread, oldest-first.
Lossy ADF surfaces under `warnings[]`:

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

`comment add KEY` (and `comment edit KEY ID`) return the persisted
comment shape:

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

`comment delete KEY ID --force` (force-gated under `--no-input`):

```json
{"data": {"comment_id": "10042", "deleted": true}}
```

## Attachments

```sh
jira issue attachment list KEY --json                                # oldest-first
jira issue attachment add KEY --file ./trace.log --json              # multipart upload
jira issue attachment download KEY 10042 --output ./local.pdf --json # clobber-protected
jira issue attachment delete KEY 10043 --force --json                # force-gated
```

`attachment list` envelope:

```json
{
  "data": {
    "attachments": [
      {"id": "10042", "filename": "screenshot.png", "mime_type": "image/png", "size": 84211,
       "author": {"account_id": "...", "display_name": "Matt Craven"},
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

`attachment download` reports written path + bytes (`mode` in `output`,
`current-dir`, or `stdout`; piped stdout is raw bytes, no envelope):

```json
{"data": {"attachment_id": "10042", "written_to": "./local.pdf", "bytes": 124521, "mode": "output"}}
```

`attachment delete`:

```json
{"data": {"attachment_id": "10043", "deleted": true}}
```

a 413 (per-project upload-size cap) maps to exit 5; the upstream
message is preserved verbatim under `errors[].message`.

## Watchers

```sh
jira issue watchers list KEY --json
jira issue watchers add KEY --user me --json          # alias: jira issue watch KEY
jira issue watchers remove KEY --user me --json       # alias: jira issue unwatch KEY
```

`watchers list` envelope — `is_watching`/`watch_count` are additive
metadata to mirror Atlassian's native shape:

```json
{
  "data": {
    "watchers": [
      {"account_id": "...", "display_name": "Alice", "email_address": "alice@example.com", "active": true}
    ],
    "is_watching": true,
    "watch_count": 3
  }
}
```

`watchers add` / `watch` (default — readback ):

```json
{
  "data": {
    "watchers": [{"account_id": "...", "display_name": "Alice", "active": true}],
    "was_already_watching": false
  }
}
```

Ambiguous user resolution → exit 3 with structured candidates so the
agent can re-run with `--user accountId:<id>`:

```json
{
  "data": {},
  "errors": [
    {
      "type": "validation",
      "message": "ambiguous user 'alice' — 3 candidates",
      "candidates": [
        {"account_id": "1", "display_name": "Alice Smith", "email_address": "alice.smith@example.com"},
        {"account_id": "2", "display_name": "Alice Jones", "email_address": "alice.jones@example.com"},
        {"account_id": "3", "display_name": "Alice Brown", "email_address": null}
      ]
    }
  ]
}
```

Zero matches → exit 2 (`not_found`); the input string is echoed in
`errors[0].message` so the agent knows what failed to resolve.

## Issue links

`KEY` is the inward issue, `--to` the outward. To read "KAN-72
blocks KAN-73" pass `KEY=KAN-73 --to KAN-72 --type Blocks`.

```sh
# A blocks B (B is blocked by A)
jira issue link <BLOCKED> --to <BLOCKER> --type Blocks --json

# A and B are related (no direction)
jira issue link KAN-73 --to KAN-72 --type Relates --json

# A is a duplicate of canonical B
jira issue link <DUP> --to <CANONICAL> --type Duplicate --json

# A is a clone of B
jira issue link <CLONE> --to <ORIGINAL> --type Cloners --json

# Preview without writing
jira issue link KAN-73 --to KAN-72 --type Blocks --dry-run --json
```

Discover the link types your instance has configured (admins add
custom ones):

```sh
jira issue view ANY-KEY --raw | jq -r '.fields.issuelinks[].type.name' | sort -u
```

Read back links on an issue with the typed envelope:

```sh
jira issue link list KEY --json
```

`link list` flattens Atlassian's wire shape — direction-aware
`other_issue` instead of inward/outward branching at the call site:

```json
{
  "data": {
    "links": [
      {"id": "9001",
       "type": {"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
       "direction": "outward",
       "other_issue": {"key": "KAN-200", "summary": "downstream service work", "status": "In Progress"}},
      {"id": "9002",
       "type": {"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
       "direction": "inward",
       "other_issue": {"key": "KAN-100", "summary": "upstream API contract", "status": "Done"}}
    ]
  }
}
```

`link delete LINK_ID --force` (force-gated under `--no-input`):

```sh
jira issue link delete 9001 --force --json
```

```json
{"data": {"link_id": "9001", "deleted": true}}
```

`link types` lists every link type the instance has configured. Cached
locally; `cache linktypes` primes the cache (and adds `data.profile`
per the cache-primer convention):

```sh
jira issue link types --json
jira cache linktypes --json
```

```json
{
  "data": {
    "link_types": [
      {"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
      {"id": "10001", "name": "Cloners", "inward": "is cloned by", "outward": "clones"},
      {"id": "10002", "name": "Relates", "inward": "relates to", "outward": "relates to"}
    ],
    "from_cache": true,
    "fetched_at": "2026-05-05T12:00:00Z",
    "count": 3
  }
}
```

Unknown link type → exit 3, error message names the missing type.
Bulk edit cannot update `issuelinks` — Jira refuses with
`"Field does not support update 'issuelinks'"`. This command is the
only path.

## Boards

`jira boards list` shows the agile boards visible to the active
profile. Prime the cache with `jira cache boards`; `boards list`
also primes transparently on first run when the cache is empty.

```sh
jira cache boards               # explicit prime
jira boards list --json         # listing (envelope or table)
jira boards list --refresh      # force re-prime
jira cache clear boards         # drop the cache file
```

`boards list` envelope:

```json
{
  "data": {
    "boards": [
      {"id": 42, "name": "Engineering Sprint", "type": "scrum",
       "project_keys": ["ENG", "PLAT"]}
    ],
    "pagination": {
      "total": 12, "start_at": 0, "max_results": 12,
      "is_last": true, "next_page_token": null
    },
    "from_cache": true,
    "fetched_at": "2026-05-06T18:30:00Z",
    "truncated": false,
    "truncated_reason": ""
  }
}
```

`type` is pass-through verbatim — `scrum` / `kanban` / `simple` are
the common values; `agility` and any future Atlassian board type
round-trip through the cache without modification.

The cache primer paginates the full set with safety bounds (default
100 pages / 10 000 boards); pass `--unbounded` to disable for very
large instances. Truncation emits a `cache-truncated` warning naming
the bound that fired.

### Filtering issues by board

```sh
jira issue list --board "Engineering Sprint"     # name resolution
jira issue list --board-id 42                    # numeric escape
jira jql build --board "Engineering Sprint" --as-jql
```

The emitted JQL is `project in (P1, P2, …)` built from the board's
cached project keys.

Resolution is **exact case-insensitive only** — no substring fallback.
Tab completion is the convenience layer:

```text
$ jira issue list --board <TAB>
Engineering Sprint  (scrum, ENG, PLAT)
Platform Roadmap    (kanban, PLAT)
```

`--board-id` is the unambiguous escape when a name collides across
projects. `--board` and `--board-id` are mutually exclusive — passing
both exits 3.

Empty cache + `--board NAME` exits 3 with `boards cache is empty —
run "jira cache boards"`. Ambiguous-name match exits 3 with
`candidates[]` listing every match's id, name, and project keys.

### `default_board` profile config

```sh
jira config set profiles.default.default_board "Engineering Sprint"
jira config get profiles.default.default_board
jira config set profiles.default.default_board ""    # unset
```

When set, `default_board` applies implicitly to `issue list` and
`jql build` whenever `--board`/`--board-id` is omitted. Explicit
`--board ""` suppresses the default for one invocation. The flag
wins over the default; the default wins exclusively over
`default_project` on commands that consume `--board` (no
intersection, no union).

Validation happens at use-time only — `config set` accepts any string
without checking the cache (which may not exist yet). When the
configured `default_board` doesn't resolve, you get:

```text
default_board "X" not found in boards cache — run "jira cache boards --refresh"
or unset with "jira config set profiles.<profile>.default_board ''"
```

## Web links (remote URL attachments)

```sh
jira issue weblink KEY --url "https://example.com/spec" --title "Spec doc" --json
```

Goes through `POST /rest/api/3/issue/{KEY}/remotelink`. Different
endpoint from issue-to-issue links. `--title` is what the user sees;
`--url` is required.

`--url` must use `http://` or `https://` (case-insensitive). Other
schemes (`javascript:`, `file:`, `ftp:`, `data:`, `mailto:`, etc.)
are rejected client-side before any HTTP call. If you need a
non-web link target, use a regular issue comment instead.

## Subtasks

Subtasks are regular `jira issue create` calls with
`issue_type: "Subtask"` and `parent.key` set:

```json
{
  "summary": "REL: Subtask 1 of KAN-104",
  "issue_type": "Subtask",
  "project_key": "KAN",
  "parent": {"key": "KAN-104"},
  "description": { "type": "doc", "version": 1, "content": [
    {"type": "paragraph", "content": [{"type": "text", "text": "Detail of subtask 1."}]}
  ]}
}
```

Verify with `jira issue view PARENT --raw | jq '.fields.subtasks'` —
recall that the typed view drops `subtasks`.

## Transitions (workflow state changes)

List available transitions for an issue (these are workflow-specific):

```sh
jira issue transition KEY --json
```

Returns `data.transitions[]` — pick an `id` and execute:

```sh
jira issue transition KEY --transition <id> --json
```

Transition IDs are workflow-specific — they vary per project and per
workflow. **Always list first to discover the IDs available for
the issue you're acting on.**

`--dry-run` validates the request without changing state.

## Worklog

```sh
jira worklog add KEY --time-spent 1h30m  --json
jira worklog add KEY --time-spent 2h     --started 2026-05-04T09:00:00.000+0000 --json
jira worklog add KEY --time-spent 45m    --comment-markdown "fixed bug X" --json
jira worklog add KEY --json-input wl.json  # full payload, ADF comment supported
```

`--time-spent` accepts `1d 2h 30m`-style durations; days resolve via
the per-profile `workday_seconds` (default 28,800 = 8h).

`worklog list KEY --json` reads, `worklog list KEY --raw` for the
full Jira shape.

## Destructive operations (clone / move / delete)

Refuses to run without one of:

| Mode           | Required          |
|----------------|-------------------|
| TTY human      | confirmation prompt (huh) — type "Yes, delete" |
| TTY + `--force` | no prompt        |
| Non-TTY / agent / `--no-input` | `--force` MUST be present (else exit 3) |
| `--dry-run`    | always allowed; never touches Jira |

Examples:

```sh
jira issue delete KAN-1 --force --json
jira issue delete KAN-1 --force --delete-subtasks --json   # drains subtasks atomically
jira issue clone  KAN-1 --force --json
jira issue move   KAN-1 --force --json-input move.json  --json
```

⚠ **Subtasks block deletion** — Jira refuses to delete a parent
without `--delete-subtasks` when the parent has subtasks. Pass
`--delete-subtasks` to remove the parent + every subtask atomically.

### Clone recipes

`issue clone` does GET → sanitize → POST. Carries: `summary`,
`description`, `issuetype`, `project`, `priority`, `assignee`,
`labels`, `components`, `fixVersions`, `affectedVersions`, `duedate`,
all `customfield_*` (except lexorank-shaped values — Jira Software's
Rank field is auto-assigned on the new issue). Drops: identifiers
(`id`, `key`, `self`), lifecycle (`status`, `statusCategory`,
`statuscategorychangedate`, `resolution`, `resolutiondate`, `created`,
`updated`, `creator`, `reporter`, `lastViewed`, `issuerestriction`),
time-tracking (`timeestimate`, `timespent`, `timeoriginalestimate`,
`workratio`, `progress`, `timetracking`, `aggregate*`), positioning
(`rankBeforeIssue`, `rankAfterIssue`), and collections (`comment`,
`worklog`, `subtasks`, `attachment`, `votes`, `watches`, `issuelinks`).

```sh
# Straight clone — same project, same fields
jira issue clone KAN-1 --force --json

# Clone with overrides — caller fields win over source fields
cat > /tmp/over.json <<'EOF'
{"fields": {"summary": "Triage copy of KAN-1", "assignee": {"accountId": "<your-id>"}}}
EOF
jira issue clone KAN-1 --force --json-input /tmp/over.json --json

# Preview without creating
jira issue clone KAN-1 --force --dry-run --json
```

To clone into a different project, override `project`:

```json
{"fields": {"project": {"key": "OTHER"}, "summary": "Ported from KAN-1"}}
```

To strip an inherited field (e.g. clear the assignee), set it to
`null` in the override.

### Move recipes

`issue move` swaps project and/or issuetype on an existing issue
(no new issue created). Required override is the destination shape:

```sh
cat > /tmp/move.json <<'EOF'
{"fields": {"project": {"key": "OTHER"}, "issuetype": {"name": "Story"}}}
EOF
jira issue move KAN-1 --force --json-input /tmp/move.json --json
```

Required-field changes between projects/types must also appear in
the override (e.g. if the target project mandates `customfield_10010`,
include it). Preview with `--dry-run` first.

## Cache primers

Per-profile JSON cache under `${XDG_CACHE_HOME:-~/.cache}/jira-cli/<profile>/`.
Each subcommand prints the data AND writes it to disk.

### What's cached and why

| Resource     | What it gives you                                | Why cache it |
|--------------|--------------------------------------------------|--------------|
| `labels`     | Every label in use across the workspace          | Lets you autocomplete labels and validate them client-side without a round-trip per `--label` flag |
| `projects`   | Every project key/name visible to you            | Validates `project_key` in payloads before submit; lists project options without GET-ing every issue |
| `epics`      | Every visible epic (key, summary, status)        | Lets you set `parent.key` to an epic without listing issues to find one |
| `fields`     | Every field on the instance, including `customfield_NNNN` IDs and their schema types | **Required** before you can author custom-field values — this is how you discover `customfield_10010` is "Story Points" and what type it expects |
| `issuetypes` | Every issue type (id, name, subtask flag)        | Validates `issue_type` in payloads; tells you which types are subtasks |
| `linktypes`  | Every issue-link type (Blocks, Cloners, Relates) | Drives `--type` completion on `jira issue link` and pins the canonical names per instance |
| `boards`     | Every agile board (id, name, type, project keys) | Drives `--board` completion on `jira issue list` / `jql build`; resolves board names to project lists for the `project in (...)` JQL clause |

### When to use it (vs reading inline)

Use the cache when you're going to make multiple writes or repeated
reads in the same session — the first call hits Jira and writes the
file, every subsequent call reads from disk in microseconds.

Skip the cache (just hit the live API) for one-shot reads or when
you specifically need fresh-from-server data (e.g. you just created
a new label and want to see it in the list).

### When to refresh

The cache is **never auto-refreshed**. It stays valid until you
either:

- Run with `--refresh` (force re-fetch and rewrite)
- Run with `--ttl-minutes N` (re-fetch if older than N minutes)
- Wipe with `jira cache clear [<resource>]`

Refresh after these events:

| Event                                                | Refresh   |
|------------------------------------------------------|-----------|
| You just created/renamed/deleted a label             | `labels`  |
| You created/renamed/archived a project               | `projects`|
| Admin added a new custom field or changed a schema   | `fields`  |
| Admin added/renamed/disabled an issue type           | `issuetypes` |
| You created/closed an epic                           | `epics`   |
| First call of a fresh session (recommended for `fields`) | as needed |
| You hit a "not found" on something you know exists   | the relevant resource — your cache is stale |

### Recommended session pattern for agents

```sh
# Once per session, prime the high-value caches:
jira cache fields     --refresh --json   # so you can map customfield_NNNN → name
jira cache projects   --refresh --json   # so you can validate project keys
jira cache issuetypes --refresh --json   # so you can validate issue_type

# Use cached data for the rest of the session:
jira cache labels --json | jq -r '.data.labels[]'   # cheap, reads disk
```

### Commands

```sh
jira cache labels      --json
jira cache projects    --json
jira cache epics       --json
jira cache fields      --json
jira cache issuetypes  --json

# Refresh / TTL:
jira cache fields --refresh         --json
jira cache fields --ttl-minutes 5   --json

# Wipe:
jira cache clear              # everything for the active profile
jira cache clear labels       # just the labels file
```

`jira cache fields --json` is the canonical way to discover
`customfield_xxxxx` IDs on a Jira instance — agents should run this
once per session before authoring custom-field values.

### Concurrency

Both the per-profile cache and the config TOML use atomic temp-file +
rename writes. Two `jira` invocations running in parallel against the
same profile will not corrupt each other's state — concurrent writes
serialize cleanly at the filesystem level.

## ADF reference

ADF is canonical. The official spec is at
[developer.atlassian.com/cloud/jira/platform/apis/document](https://developer.atlassian.com/cloud/jira/platform/apis/document/).
The CLI's MVP support set is mirrored in
`jira agent adf-matrix --json` (per-row `official_url` points to the
Atlassian docs page for that node/mark).

Every ADF doc starts with the root:

```json
{ "type": "doc", "version": 1, "content": [ /* block nodes */ ] }
```

### Block nodes

```json
// paragraph (the simplest body)
{"type": "paragraph", "content": [{"type": "text", "text": "hello"}]}

// heading (level 1-6)
{"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "Section"}]}

// blockquote (wraps any block content)
{"type": "blockquote", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "quoted"}]}]}

// bulletList / orderedList (content is listItem[])
{"type": "bulletList", "content": [
  {"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "first"}]}]}
]}

// codeBlock (attrs.language is the syntax-highlight hint)
{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "func main() {}"}]}

// rule (horizontal divider, no content)
{"type": "rule"}

// panel (panelType = info | warning | error | success | note)
{"type": "panel", "attrs": {"panelType": "info"}, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "info panel"}]}]}

// table (content is tableRow[]; tableRow content is tableHeader[] / tableCell[])
{"type": "table", "attrs": {"isNumberColumnEnabled": false, "layout": "default"}, "content": [
  {"type": "tableRow", "content": [
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Col"}]}]}
  ]},
  {"type": "tableRow", "content": [
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "val"}]}]}
  ]}
]}

// expand (collapsible — non-MVP but Jira accepts it)
{"type": "expand", "attrs": {"title": "Click to expand"}, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "hidden"}]}]}

// nestedExpand (only valid INSIDE tableCell or tableHeader)
```

### Inline nodes (live inside a paragraph's content[])

```json
// text — the only required attr is .text
{"type": "text", "text": "plain"}

// hardBreak — line break inside a paragraph
{"type": "text", "text": "line one"}, {"type": "hardBreak"}, {"type": "text", "text": "line two"}

// mention — id MUST be the user's accountId
{"type": "mention", "attrs": {"id": "712020:ff38cf6b-...", "text": "@Matt Craven"}}

// emoji — shortName + id (unicode codepoint) + text (the actual unicode)
{"type": "emoji", "attrs": {"shortName": ":rocket:", "id": "1f680", "text": "🚀"}}

// date — timestamp is epoch milliseconds AS A STRING
{"type": "date", "attrs": {"timestamp": "1769817600000"}}

// status — text is the label, color is named (green/red/yellow/blue/purple/...)
{"type": "status", "attrs": {"text": "READY", "color": "green"}}

// inlineCard — smart link
{"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/KAN-1"}}
```

### Marks (annotate text nodes)

```json
{"type": "text", "text": "bold",       "marks": [{"type": "strong"}]}
{"type": "text", "text": "italic",     "marks": [{"type": "em"}]}
{"type": "text", "text": "struck",     "marks": [{"type": "strike"}]}
{"type": "text", "text": "underlined", "marks": [{"type": "underline"}]}
{"type": "text", "text": "code()",     "marks": [{"type": "code"}]}
{"type": "text", "text": "link",       "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]}
{"type": "text", "text": "red",        "marks": [{"type": "textColor", "attrs": {"color": "#ff0000"}}]}
{"type": "text", "text": "highlight",  "marks": [{"type": "backgroundColor", "attrs": {"color": "#fffacd"}}]}
{"type": "text", "text": "2",          "marks": [{"type": "subsup", "attrs": {"type": "sub"}}]}
```

Multiple marks on one text node compose:

```json
{"type": "text", "text": "loud", "marks": [{"type": "strong"}, {"type": "underline"}, {"type": "textColor", "attrs": {"color": "#ff0000"}}]}
```

### Composition recipes

Drop these straight into `--json-input` for `issue create`, `issue edit`,
or `issue comment`. As substructures of an `issue create` payload,
assign them to the bare Jira field name (`description`, `environment`,
the relevant `customfield_NNNN`) — that is what Jira's API accepts and
what the CLI's ADF validator detects.

**Heading + paragraph + link:**

```json
{"type": "doc", "version": 1, "content": [
  {"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "Investigation"}]},
  {"type": "paragraph", "content": [
    {"type": "text", "text": "See "},
    {"type": "text", "text": "PR #482", "marks": [{"type": "link", "attrs": {"href": "https://github.com/org/repo/pull/482"}}]},
    {"type": "text", "text": " for the fix."}
  ]}
]}
```

**Bulleted list with mixed marks per item:**

```json
{"type": "bulletList", "content": [
  {"type": "listItem", "content": [{"type": "paragraph", "content": [
    {"type": "text", "text": "DB write path: ", "marks": [{"type": "strong"}]},
    {"type": "text", "text": "blocked"}
  ]}]},
  {"type": "listItem", "content": [{"type": "paragraph", "content": [
    {"type": "text", "text": "Inline code "},
    {"type": "text", "text": "user.last_login", "marks": [{"type": "code"}]},
    {"type": "text", "text": " not updating."}
  ]}]}
]}
```

**Numbered list — same shape, swap `bulletList` for `orderedList`:**

```json
{"type": "orderedList", "attrs": {"order": 1}, "content": [/* listItem[]... */]}
```

**Code block with language:**

```json
{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "func main() {\n  fmt.Println(\"hi\")\n}"}]}
```

**Panel for callouts (info / warning / error / success / note):**

```json
{"type": "panel", "attrs": {"panelType": "warning"}, "content": [
  {"type": "paragraph", "content": [
    {"type": "text", "text": "Don't forget to bump the schema version."}
  ]}
]}
```

**Inline mention of a user:**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "cc "},
  {"type": "mention", "attrs": {"id": "712020:ff38cf6b-...", "text": "@Matt Craven"}},
  {"type": "text", "text": " — heads up"}
]}
```

The `id` MUST be the user's `accountId` (get it from
`jira me --json` for yourself, or the assignee field on any issue
they own). The `text` is the display label and can be anything.

**Status pill (named color: green / red / yellow / blue / purple / grey / neutral):**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "Build: "},
  {"type": "status", "attrs": {"text": "GREEN", "color": "green"}}
]}
```

**Inline date (epoch milliseconds, as a string):**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "Target: "},
  {"type": "date", "attrs": {"timestamp": "1769817600000"}}
]}
```

**Smart link (Jira renders as a card if the URL is recognised):**

```json
{"type": "paragraph", "content": [
  {"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/KAN-72"}}
]}
```

**Quote block (any block content can nest inside):**

```json
{"type": "blockquote", "content": [
  {"type": "paragraph", "content": [
    {"type": "text", "text": "From the postmortem: "},
    {"type": "text", "text": "the migration ran twice", "marks": [{"type": "em"}]},
    {"type": "text", "text": "."}
  ]}
]}
```

**Two-column table with header row:**

```json
{"type": "table", "attrs": {"isNumberColumnEnabled": false, "layout": "default"}, "content": [
  {"type": "tableRow", "content": [
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Field"}]}]},
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Value"}]}]}
  ]},
  {"type": "tableRow", "content": [
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "duration"}]}]},
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "2h30m"}]}]}
  ]}
]}
```

**Horizontal divider between sections:**

```json
{"type": "rule"}
```

### ADF gotchas

- **Marks live on `text` nodes only** — putting `marks` on a
  `paragraph` or `heading` is invalid; strict mode rejects with
  the path of the offending node.
- **`bulletList` / `orderedList` content is `listItem[]`** — and
  every `listItem` content is `paragraph` (or another list to
  nest). Putting raw `text` directly inside a `listItem` is invalid.
- **`mention.attrs.id` is the accountId, not the email or display name.**
  Wrong id → mention renders as plain text in Jira.
- **`date.attrs.timestamp` is a string of milliseconds**, not a
  number and not seconds. `1769817600000` not `1769817600`.
- **`codeBlock` content is a single `text` node** containing the
  whole code with embedded `\n`s — not a list of lines.
- **`emoji.attrs.id` is the unicode codepoint** (e.g. `1f680`),
  `shortName` is the `:rocket:`-style alias, `text` is the actual
  unicode glyph (`🚀`). All three should be set for portability.
- **`tableCell` / `tableHeader` content must be wrapped in
  `paragraph`** — bare text inside a cell is invalid.

### ADF strict vs best-effort

| Path                   | Default mode |
|------------------------|--------------|
| Read / render          | best-effort  |
| `--plain` extract      | best-effort  |
| `--raw` emit           | n/a (passthrough, no validation) |
| Mutation submit        | strict       |
| `--dry-run` preview    | strict       |

Override per call:

```sh
jira issue create ... --adf-strict        # any lossy step → exit 3
jira issue create ... --adf-best-effort   # degrade silently with warnings
```

Or globally: `JIRA_ADF_STRICT=true` env, or `adf_strict = true` in the
profile TOML. Precedence: flag > env > profile > per-path default.

### Opaque preservation

Unknown ADF nodes/marks (anything outside the CLI's MVP set) round-trip
through the CLI byte-equivalently — the typed model preserves them via
opaque passthrough. **However**: Jira's create endpoint validates the
posted document against its own ADF schema and will reject truly unknown
node types with `INVALID_INPUT (400)`. The opaque path is for
preserving fidelity on read; submit paths are bounded by what Jira
itself accepts.

## JQL reference

Authoritative Atlassian docs (cite these — link rendering is honest):

- [JQL fields](https://support.atlassian.com/jira-service-management-cloud/docs/jql-fields/)
- [JQL operators](https://support.atlassian.com/jira-service-management-cloud/docs/jql-operators/)
- [JQL keywords](https://support.atlassian.com/jira-service-management-cloud/docs/jql-keywords/)
- [JQL functions](https://support.atlassian.com/jira-service-management-cloud/docs/jql-functions/)
- [JQL developer status](https://support.atlassian.com/jira-service-management-cloud/docs/jql-developer-status/)
- [JQL advanced-roadmap fields](https://support.atlassian.com/jira-service-management-cloud/docs/search-for-advanced-roadmaps-custom-fields-in-jql/)

### Common operators

| Operator                | Meaning                                  |
|-------------------------|------------------------------------------|
| `=`  /  `!=`            | exact match                              |
| `in (a, b, c)`          | match any of                             |
| `not in (a, b, c)`      | match none of                            |
| `~` / `!~`              | text match (string fields)               |
| `>`  `>=`  `<`  `<=`    | numeric / date comparison                |
| `is empty` / `is not empty` | null check (some fields use `EMPTY`) |
| `was`                   | historical value (combined with `during(...)`) |
| `changed`               | value transitioned (combined with `from`/`to`/`by`/`during`) |

### Common keywords

| Keyword | Meaning                              |
|---------|--------------------------------------|
| `AND`   | all conditions must match            |
| `OR`    | any condition may match              |
| `NOT`   | invert the condition                 |
| `ORDER BY <field> ASC|DESC` | sort the result set      |

### Common functions

```text
currentUser()              # the calling user's accountId
now()                      # current timestamp
startOfDay() / endOfDay()  # boundary helpers (also Week/Month/Year)
membersOf("group-name")    # members of a Jira group
componentsLeadByUser()     # components led by current user
projectsLeadByUser()
linkedIssues(KEY [, "blocks"])   # find issues linked to KEY by a link type
issuesWithText("phrase")
```

### High-yield recipes

```sh
# Everything assigned to me, not done
jira search jql 'assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC' --json

# In-flight issues in a specific project
jira search jql 'project = KAN AND status = "In Progress"' --json

# Bugs reported in the last sprint
jira search jql 'project = KAN AND issuetype = Bug AND created > startOfMonth()' --json

# Issues in any of my epics
jira search jql 'project = KAN AND parent in (linkedIssues(currentUser()))' --json

# Recently updated, with a specific label
jira search jql 'project = KAN AND labels = "regression" AND updated > -7d' --json

# Issues blocked by a specific issue
jira search jql 'issue in linkedIssues("KAN-72", "is blocked by")' --json

# Subtasks of a parent
jira search jql 'parent = KAN-69' --json

# Status-history check (was = 'In Progress' some time recently)
jira search jql 'status was "In Progress" during ("2026-04-01", "2026-05-01")' --json
```

### JQL builder — flag-driven query construction

When you don't want to hand-author JQL, build it from flags. Useful
in pipelines and for agents avoiding string-quoting bugs.

```sh
jira jql build --project KAN --status Done --assignee me --json
# → {"jql": "project = KAN AND assignee = currentUser() AND status = Done ORDER BY updated DESC"}

jira jql build --project KAN --label regression --label hotfix --type Bug --type Task --json
# → {"jql": "project = KAN AND labels in (regression, hotfix) AND issuetype in (Bug, Task) ORDER BY updated DESC"}

jira jql build --project KAN --order-by updated --desc --json
# → {"jql": "project = KAN ORDER BY updated DESC"}
```

Translations the builder applies for you:

| Flag                  | Translates to                          |
|-----------------------|----------------------------------------|
| `--assignee me`       | `assignee = currentUser()`             |
| `--reporter me`       | `reporter = currentUser()`             |
| Repeated `--label X`  | `labels in (X, Y, Z)`                  |
| Repeated `--type X`   | `issuetype in (X, Y, Z)`               |
| Repeated `--status X` | `status in (X, Y, Z)`                  |
| `--order-by F --desc` | `ORDER BY F DESC` (default ASC)        |
| no flags              | `updated >= -365d ORDER BY updated DESC` |

Validation:

- `--order-by <field>` is allow-listed; arbitrary field names with
  shell metachars (e.g. `'updated; DROP TABLE x'`) are rejected
  with `invalid order-by field` (exit 3, parseable envelope)
- `--label`/`--type`/`--status` values containing unbalanced quotes
  are rejected at flag-parse time before any string concat

Pipe builder output straight into search:

```sh
JQL=$(jira jql build --project KAN --assignee me --json | jq -r '.data.jql')
jira search jql "$JQL" --json
```

`jira issue list --as-jql --json` returns the same builder output
without making the API call — useful for previewing what
`issue list` would run.

### Saved JQL queries

Files under `~/.config/jira-cli/queries/<name>.jql` with optional YAML
frontmatter:

```text
---
name: my-open-bugs
description: Bugs assigned to me, not done
project: KAN
---
project = KAN AND issuetype = Bug AND assignee = currentUser() AND statusCategory != Done
ORDER BY priority DESC, updated DESC
```

Run:

```sh
jira search saved my-open-bugs --json
```

## Auth

### Setting up authentication

End state: a profile in `~/.config/jira-cli/config.toml` with a base
URL + email + auth-type, and a credential reachable from one of the
supported backends. The TOML never holds the secret.

#### Step 1 — pick a backend

Backends are tried in this priority order on every API call. Pick the
one you intend to use; the others stay as fallbacks.

| Backend | Pick when |
|---------|-----------|
| **OS keyring** (default) | Single workstation, you want zero extra setup, your OS provides Secret Service / Keychain / Credential Manager |
| **1Password (Go SDK)** | Team uses 1Password, you have a service account token (`OP_SERVICE_ACCOUNT_TOKEN` env var) OR the desktop app is signed in (`onepassword_account` set) |
| **1Password (`op` CLI fallback)** | You're already signed in via `op` CLI but don't want to wire the SDK |
| **Env var** | CI / containers / ephemeral runners. Read from `JIRA_TOKEN_<PROFILE>` |

The first backend that returns a credential wins. You don't have to
use only one — set up keyring locally, fall through to env in CI.

#### Step 2 — create the profile + credential

**Interactive (TTY, recommended for first time):**

```sh
jira auth login
# walks through: profile name → base URL → email → auth type → backend
# → credential prompt (reads from stdin without echoing)
```

**Headless (CI, scripted, agent):**

```sh
echo "$JIRA_TOKEN" | jira auth login --no-input \
  --profile-name work \
  --base-url https://company.atlassian.net \
  --email dev@example.com \
  --auth-type token \
  --backend keyring \
  --secret-stdin
```

For 1Password backend, use `--vault` + `--item` instead of
`--secret-stdin`:

```sh
jira auth login --no-input \
  --profile-name work \
  --base-url https://company.atlassian.net \
  --email dev@example.com \
  --auth-type token \
  --backend 1password \
  --vault Engineering \
  --item jira-cli-work
```

Auth types accepted: `token`, `basic`, `pat`, `mtls`. Anything else
returns exit 3 — no fake authenticated profile is stored.

#### Step 3 — verify

```sh
jira auth status              # per-profile credential health
jira auth whoami --save       # /myself + persist account_id to the profile
jira me                       # short identity card
```

`whoami --save` is what enables `--assignee me`, the TUI `A` key, and
"in my epics" JQL — run it once per profile after first login.

### Managing existing profiles

```sh
jira auth switch <profile>          # change active profile (writes default_profile)
jira auth refresh                    # re-resolve the credential from the backend
jira auth migrate --backend 1password  # move credential between backends
jira auth logout <profile>           # remove credential from the backend (TOML metadata stays)
jira auth token --json               # REDACTED token diagnostics (length, prefix, backend)
```

### Partial updates merge — they don't replace

`auth login --no-input` with **partial flags merges** into the existing
profile. Fields not supplied retain their current values. To replace
cleanly, pass every field. This protects against mistyped one-flag
updates wiping unrelated fields like `email` or `account_id`.

### When auth fails

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Exit 1 on every call | Credential missing or expired | `jira auth status` → check which backend is failing → re-run `auth login` for that profile |
| `unsupported auth type "X"` | Typo in `--auth-type` | Use one of `token`, `basic`, `pat`, `mtls` |
| `credential not found` | Backend has no entry for this profile | `jira auth login --profile-name <name>` |
| `OP_SERVICE_ACCOUNT_TOKEN not set` (1P backend) | Service-account env missing | Either export it, or fall back to keyring backend |
| 401 from Jira on a previously-working profile | Token revoked / rotated | `jira auth login --profile-name <name>` to replace |

### Secret hygiene contract

- Secrets are **never** stored in the TOML config — only metadata
  (backend selector, vault, item ref)
- All logging, including `--debug`, redacts `Authorization` headers
  and any field named `secret` / `token` / `api_token` / `cookie`
- CLI-written files containing credential metadata are mode `0600`
- `jira auth token` deliberately does NOT print the raw token — only
  length, prefix, and backend identity

These contracts are enforced at the HTTP transport layer, not per
command. Anything that calls Jira goes through the same redactor.

## Read-only mode

Block every mutation request at the HTTP layer (single source of
control — no per-command boilerplate to forget):

```sh
JIRA_READ_ONLY=true jira <anything>      # globally for this invocation
```

Or per profile in `config.toml`:

```toml
[[profiles]]
  name = "agent-handoff"
  read_only = true
```

Env wins on the OFF→ON direction — once `JIRA_READ_ONLY` is set, every
profile becomes read-only regardless of its own setting. Any mutation
request returns exit 3 with `read-only mode is active`.

## Editor configuration

Used for the kubectl-style `jira issue edit KEY` flow when no field
flags are provided. Resolution chain (highest precedence first):

1. `JIRA_EDITOR` env var
2. Per-profile `editor` field in TOML
3. Global `editor` field in TOML
4. `EDITOR` env var
5. `VISUAL` env var
6. `vi` (last-resort fallback)

```toml
# config.toml
editor = "code -w"

[[profiles]]
  name = "default"
  editor = "vim"   # overrides the global on this profile
```

## Headless writes — the `--no-input` contract

Every mutation command supports `--no-input`. Under it:

- No interactive prompts — the CLI never reads your terminal
- No implicit stdin reads — only `--json-input -` (command payload) and
  `--secret-stdin` (auth secrets) may read stdin
- Missing required fields → exit 3 (validation), never silent success
- Destructive ops still require `--force` on top of `--no-input`

Per-command headless requirements:

| Command                 | Minimum to succeed under `--no-input` |
|-------------------------|---------------------------------------|
| `issue create`          | summary + project_key + issue_type (or defaults from profile) |
| `issue edit`            | at least one field flag OR `--json-input` |
| `issue comment`         | `--body-markdown` OR `--json-input` |
| `issue transition`      | `--transition <id>` |
| `issue link`            | `--to` and `--type` |
| `issue weblink`         | `--url` |
| `issue delete/clone/move` | `--force` |
| `worklog add`           | `--time-spent <duration>` |
| `auth login`            | `--base-url` |

## Pagination & `--all`

```sh
jira issue list --limit 25                 # stop after 25 results
jira issue list --all                       # drain pages until isLast
jira issue list --all --max-pages 50        # override default 100 pages
jira issue list --all --max-results-total 5000  # override default 10,000
jira issue list --all --unbounded           # remove BOTH limits (never set by TUI)
```

Bounded by default to protect runaway agents. When a bound truncates,
`meta.pagination.truncated = true` and `truncated_reason ∈ {max_pages, max_results}`.

## Debug logging

`--debug` (or `-d`) enables HTTP request/response dumps to stderr with
`Authorization`, `Cookie`, and `X-Atlassian-Token` redacted to
`REDACTED`:

```sh
jira issue edit KAN-1 --json-input fields.json --no-input --debug 2>&1 | grep '^DBG'
```

Useful when Jira's response is unexpected — you can see the exact
body sent and the headers returned, including the `Atl-Traceid` for
support tickets.

## Common pitfalls

1. **`issue view --json` drops `parent` and `subtasks`** from the typed
   envelope. Use `--raw` when you need them.
2. **`issue view --json` shows `issuetype.name` as `null`** for some
   types in the typed envelope. `--raw` is correct.
3. **`environment` field is ADF on most modern Jira instances** — pass
   the full ADF doc, not a plain string. Same for `description`.
4. **`issuelinks` cannot be set via `issue edit` bulk update** — Jira
   refuses with `"Field does not support update 'issuelinks'"`. Use
   `jira issue link` instead.
5. **`issue delete` on a parent with subtasks** requires
   `--delete-subtasks` (Jira refuses the delete otherwise).
6. **`--no-input` with no field flags on `issue edit`** is a validation
   error (exit 3) — never a silent no-op.
7. **Customfield validators are strict** — `select` requires
   `{"value": "non-empty string"}`, `parent` requires
   `{"key": "PROJ-123"}` matching issue-key shape, `date` must be
   `YYYY-MM-DD`, `datetime` must be ISO-8601 with timezone. Malformed
   values fail at stage 4 of the pipeline before the API call.
8. **Unregistered `customfield_NNNN` keys** (no schema in the registry)
   are forwarded opaquely to Jira and emit a `customfield_unknown_type`
   warning so callers can audit what the registry didn't recognise.
9. **Opaque ADF nodes / marks** (anything outside the CLI's MVP set)
   round-trip through the CLI byte-equivalently. Strict mode rejects
   them at validation stage 2 with the offending type name in the
   error; best-effort mode preserves and emits an `unknown_adf_node`
   or `unknown_adf_mark` warning. Even when best-effort lets the doc
   through, Jira's own create endpoint may still reject truly unknown
   types with `INVALID_INPUT (400)` — strict mode is the safer default
   for new code.
10. **`--url` on `issue weblink` rejects non-http(s) schemes** —
    `javascript:`, `file:`, `ftp:`, `data:`, `mailto:`, etc. all fail
    client-side before any HTTP call. Web links must be web URLs.
11. **`auth login --no-input` merges, not replaces** — partial flags
    update only the supplied fields. Pass every field for a clean
    replace.
12. **Multiple agent envs at once** (e.g., `CLAUDECODE=1` +
    `CURSOR_TERMINAL=1`) resolve via a fixed precedence list:
    amp → codex → gemini → copilot → opencode → cursor → claude.
    Set `AI_AGENT=<name>` to override explicitly.

## Quick reference — one-liners

```sh
# Inspect environment
jira me                                              # who am I?
jira config profile                                  # what profiles exist?

# Discover a project
jira cache projects --json
jira cache fields --json | jq '.data.fields[] | select(.id | startswith("customfield_"))'
jira cache issuetypes --json

# Read
jira issue view KAN-1 --json
jira search jql 'assignee = currentUser() AND statusCategory != Done' --json

# Write (always start with --dry-run)
jira issue create --dry-run --no-input --json-input payload.json --json
jira issue create          --no-input --json-input payload.json --json

# Single-shot edits
jira issue edit KEY --summary "New title" --no-input --json
jira issue edit KEY --assignee me        --no-input --json

# Link + flow
jira issue link KEY --to OTHER --type Blocks --json
jira issue weblink KEY --url URL --title "..."  --json
jira issue transition KEY --json                            # list
jira issue transition KEY --transition 21 --json            # execute
jira issue comment KEY --json-input adf.json --no-input --json
jira worklog add KEY --time-spent 1h30m --no-input --json

# Destructive
jira issue delete KEY --force --json
jira issue delete KEY --force --delete-subtasks --json      # parent with subtasks

# Debug
jira <cmd> --debug 2>&1 | grep '^DBG'
```
