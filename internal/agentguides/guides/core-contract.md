---
slug: core-contract
title: Read every jira-cli response the same way
description: The output contract — envelope shape, output modes, exit codes, and the global gates every invocation obeys.
when_to_use: Parsing any jira-cli output, choosing --output, handling a nonzero exit, or scripting the CLI headlessly.
commands: []
order: 1
---

## Decide

Pick an output mode once and parse accordingly:

*   `--output compact` — the bare JSON data payload, envelope stripped, and
    every null-valued key dropped recursively to stay token-lean. An absent
    key means the value was null; `false`, `0`, and empty arrays/objects
    are kept. Best default for agents.
*   `--output json` — the full envelope described under Save.
*   `auto` (the default) resolves per context: detected agent environment →
    `compact`, non-TTY stdout → `json`, interactive terminal → `human`.
    Detection reads the cross-tool `AGENT` convention first (`AGENT=0`
    opts out), then harness markers such as `CLAUDECODE`.

Human mode is prose for people. Never parse it.

## Run

Rules that hold for every command:

*   Machine output goes to stdout; status, spinners, and diagnostics go to
    stderr. Read them separately.
*   Treat stdout as unusable if the destination closes, fills, or accepts only
    a prefix. The command exits 8; a mutation may already have reached Jira.
*   `--jq <expr>` filters the JSON output in-process — no external jq
    needed; string results print raw. It implies `--output json` when no
    mode is set, filters failure envelopes too (exit code preserved), and
    conflicts with `--output human` (exit 3).
*   Flags from other Jira CLIs (`--plain`, `--paginate`, …) are rejected
    with a `flag_foreign` error whose `suggestions[]` lists this CLI's
    equivalents. Trust the suggestion, not muscle memory.
*   Headless mutations require `--no-input`; destructive ones additionally
    require `--force`. Both are deliberate gates — do not script around
    them. Under `--no-input`, only `--json-input -` and `--secret-stdin`
    may read stdin.
*   `JIRA_READ_ONLY=1` blocks every Jira write at the HTTP transport.
*   `--timeout` bounds the whole invocation. Reads auto-retry a 429/503
    within `--max-retry-wait`; **mutations are never auto-retried** — a
    rate-limited write returns exit 4 for you to decide.

## Save

The envelope, on stdout for success and failure alike:

```json
{
  "ok": true,
  "meta": {
    "command": "issue.view",
    "timestamp": "2026-01-01T00:00:00Z",
    "request_id": "…"
  },
  "data": {},
  "errors": [],
  "warnings": []
}
```

*   `ok: false` → read `errors[]`. Each error carries `type`, a stable
    snake_case `code`, `message`, `hint` (the recommended next action), and
    `retryable`. Context fields appear when they apply: `flag`, `field`,
    `path`, `suggestions` (valid values for a miss), `candidates`
    (structured matches on an ambiguous lookup), and on Jira failures
    `http_status`, `retry_after_seconds`, `rate_limit_remaining`,
    `provider`, `upstream_code`, `upstream_status`,
    `upstream_request_id`, `upstream_messages`, `upstream_field_errors`.
*   Issue identity is always an object — `{"key": …, "id": …, "self": …}` —
    never a bare string.
*   Each subcommand owns its `data` object. Discover its fields and selectors
    from `jira agent schema --path "issue create"` (replace the path with the
    command you will run); the focused schema embeds that command's input and
    output shapes. Do not infer fields from a sibling command.
*   Mutation output separates stable request context from conditional server
    outcomes. Validated fields, targets, files, users, and local/remote
    validation facts stay present after a successful live write when the
    dry-run carried them. Created or updated Jira objects, readbacks,
    verification results, written paths, and byte counts appear only on the
    paths that produce them.
*   Every mutation carries `data.dry_run`: `true` on preview, `false` on the
    live write.
*   Multi-key commands take key lists and ranges as positional arguments
    (`PROJ-1 PROJ-3`, `PROJ-1..PROJ-10`), expand them transparently, and
    return ordered `data.results[]` with per-key errors; successful keys
    are never discarded by one failure. The operation-specific object that a
    single-key command places at `data` moves unchanged to
    `data.results[].data`; selectors therefore gain only the keyed prefix.
*   `warnings[]` carries coded, non-fatal signals — act on the code, not
    the text: `rate_limit_near` means throttle before the 429,
    `lossy_adf_conversion` means content was flattened.
*   `meta.pagination` appears on paginated reads; pass its `nextCursor` back
    via `--cursor`.
*   Error envelopes are always the full shape, even under
    `--output compact` — parse failures the same way in every mode.
*   The schema root's `contract_version` is the contract revision: major
    bump = breaking, minor = additive, patch = wording. Re-read guides and
    schema before reusing saved recipes across a major bump.

## Preconditions

None. The contract holds from the first invocation, before any credential
exists.

## Recover

Exit codes are a published contract: `0` ok, `1` auth, `2` not-found, `3`
validation, `4` rate-limit, `5` server, `6` canceled (`code=canceled`),
`7` timeout (`code=timeout`), `8` local output failure
(`code=output_write_failed`, `type=io`, `retryable=false`). A write refused
by read-only mode reports `code=read_only`.

On failure act on `errors[0].hint`; it names the command or flag that fixes
the problem. Retry only when `retryable` is true. For wire-level detail,
re-run with `--debug` — auth headers are redacted, so output stays safe to
share.

For exit 8, fix the pipe, file or terminal destination and discard any partial
stdout. Do not automatically repeat a mutation: inspect Jira first because the
remote write may have succeeded before local output failed.

## Next

*   `safe-mutation` — the write discipline built on these gates.
*   `bootstrap` — get an authenticated profile.
