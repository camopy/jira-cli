## core_contract
Goal: Apply the cross-cutting output-mode, envelope, exit-code, headless, pagination, read-only, and debug contract every other workflow inherits.
When: anything about output mode, exit codes, pagination, read-only mode, or headless behaviour needs to be confirmed before another workflow can rely on it.

**Decide**

# output mode
- TTY humans: `--output=human` (or omit; `auto` picks `human` on a TTY).
- Automation: `--output=json` (full envelope) for parsing; `--output=compact` for the JSON `data` payload only, one line, jq-friendly. Compact drops every `null`-valued key recursively to stay token-lean — an absent key means the value was null, while `json` keeps the full schema (nulls included). `false`, `0`, and empty arrays/objects are kept either way.
- Agent harnesses: `auto` resolves to `compact` when an agent env var is set.
- There is no separate raw REST passthrough mode — `json` and `compact` cover every machine-consumption need. Some command payloads preserve Jira objects under command-specific keys, such as `data.issue` for `issue view`.

# headless writes (`--no-input`)
- Set `--no-input` on every mutation when scripting. Under it the CLI never prompts, never reads stdin implicitly, and never silently no-ops.
- Only `--json-input -` (command payload) and `--secret-stdin` (auth secret) may read stdin under `--no-input`.
- Destructive ops (`delete` / `clone` / `move` / `comment delete` / `link delete` / `attachment delete`) still require `--force` on top of `--no-input` → see `safe_mutation`.

# pagination
- One shape, one place: every paginated read emits the camelCase block (`startAt`, `maxResults`, `total`, `isLast`, `nextCursor`) at `meta.pagination`; keyed multi-key results carry the same shape at `results[].data.pagination`. Mutations emit no pagination block.
- `isLast` and `nextCursor` are authoritative. `total` is present only when the endpoint reports one — token-paged search never does, so use `--count` for a number, never a missing total as "empty".
- Single page (default page size): omit `--limit` / `--all`. Custom page size: `--limit N`.
- Walk every page: `--all` (`search jql`, `issue list`, `issue comment list`, `issue attachment list`, `boards list`) — bounded at 100 pages / 10 000 results; `--unbounded` lifts the caps on the drains.
- Page-by-page / resume after a context reset: pass `meta.pagination.nextCursor` back via `--cursor` (`search jql`, `issue list`); a truncated bounded `--all` also returns the resume cursor. Offset-paged reads (`comment list`) surface the next offset in `nextCursor`; client-side windows (`attachment list`, `boards list`) have no cursor — re-run with `--all`.

# read-only mode
- Block every mutation at the HTTP layer for the invocation: `JIRA_READ_ONLY=true jira <anything>`.
- Block per profile in `config.toml`:
  ```toml
  [[profiles]]
    name = "agent-handoff"
    read_only = true
  ```
- Env wins on the OFF→ON direction — once `JIRA_READ_ONLY` is set, every profile is read-only regardless of its own setting.

# debug
- HTTP request/response dumps to stderr with secrets redacted: `--debug` (or `-d`). Under `--debug` each rate-limit backoff and the give-up decision also log their reason.

# rate-limit retry
- Reads auto-retry a 429 (or a 503 carrying `Retry-After`) within `--max-retry-wait` (default `30s`; `0` disables; `JIRA_MAX_RETRY_WAIT` sets it for the process; an explicit flag wins). The budget is always capped by `--timeout`. Mutations are never auto-retried. Exit 4 means Jira still returned 429 after any applicable retry was exhausted or skipped.

**Run**
- Canonical write: `jira <cmd> --no-input --json-input payload.json --output=json`
- Pagination drain: `jira issue comment list KEY --all --output=json`
- Cursor walk: `jira search jql "project = X" --output=json`, then re-run with `--cursor <meta.pagination.nextCursor>` until `isLast=true`
- Per-call read-only: `JIRA_READ_ONLY=true jira issue edit <ISSUE_KEY> --summary "x" --no-input --output=json`
- Debug capture: `jira issue edit <ISSUE_KEY> --json-input fields.json --no-input --debug 2>&1 | grep '^DBG'`

**Save**
> Requires `--output=json`.
- `ok` [bool, required] — `true` on success, `false` on failure.
- `meta.command` [string, required] — dotted command path (e.g. `issue.create`).
- `meta.timestamp` [string, required] — ISO 8601 UTC.
- `meta.request_id` [string, required] — request correlation id.
- `meta.exit_code` [int, present on failure] — mirrors the process exit code; `data` is `null` when set.
- `meta.pagination` [object, optional] — `startAt`, `maxResults`, `total` (only when the endpoint reports one), `isLast`, `nextCursor`; walk until `isLast=true`, resume via `--cursor` where supported. Absent entirely on mutations.
- `data` [object, required on success] — command-specific payload; `null` on failure.
- `errors[]` [array, required] — each entry carries `type`, `code` (stable snake_case — branch on this, never on `message`), `message`, `hint` (never empty), `retryable`. Optional fields when relevant: `flag` (argv-level: the offending flag name), `field` (payload-level: the offending JSON key), `path` (a document path within the offending value), `suggestions` ("did you mean" candidates for the input, as the caller would type them), `http_status`, `retry_after_seconds`, `rate_limit_remaining`, `provider`, `upstream_code`, `upstream_status`, `upstream_messages` (Jira's errorMessages array), `upstream_field_errors` (Jira's per-field errors map), `candidates` (structured disambiguation rows for `user_ambiguous` / `board_ambiguous`). Jira API errors leave `upstream_code` empty — Jira exposes no stable machine error code.
- `warnings[]` [array, required] — non-fatal diagnostics; never blank on a successful command that degraded.

**Behavior**
- Envelope shape (success):
  ```json
  {
    "ok": true,
    "meta": {
      "command": "issue.create",
      "timestamp": "2026-05-04T22:48:55Z",
      "request_id": "...",
      "pagination": { "startAt": 0, "maxResults": 50, "total": 12, "isLast": true }
    },
    "data":     { /* command-specific */ },
    "errors":   [],
    "warnings": []
  }
  ```
- `--output=auto` resolution table:

  | Context           | Resolved mode                                                |
  |-------------------|--------------------------------------------------------------|
  | TTY human         | `human`  (clog rich text)                                    |
  | Non-TTY (pipe)    | `json`  (full envelope)                                      |
  | Detected agent    | `compact`  (envelope `data` only, single line, jq-friendly)  |

- Agent env detection (first match wins, fixed precedence amp → codex → gemini → copilot → opencode → cursor → claude): `AGENT=amp`, `CODEX_SANDBOX`, `CODEX_CI`, `CODEX_THREAD_ID`, `CODEX`, `OPENAI_CODEX`, `GEMINI_CLI`, `COPILOT_CLI`, `COPILOT`, `GITHUB_COPILOT`, `OPENCODE`, `CURSOR_TERMINAL`, `CURSOR_AGENT`, `CLAUDECODE`, `CLAUDE_CODE`. `AI_AGENT=<name>` is the explicit override.
- `--output=compact` strips `meta` and `errors` on success. **Error paths still emit the full envelope** so failures stay parseable regardless of mode flags.
- Machine envelopes never carry `meta.profile` — a command that reports a profile puts it in command-specific `data`.
- `--output` value table:

  | `--output` value | Effect                                                                         |
  |------------------|--------------------------------------------------------------------------------|
  | `auto`           | Detect: TTY → human, pipe → json, agent → compact                              |
  | `human`          | Force human-friendly clog rich text                                            |
  | `json`           | Force the full structured envelope on stdout for success                       |
  | `compact`        | Force the JSON `data` payload only — no `ok`/`meta`/`warnings`/`errors`; null-valued keys dropped |

- In `--output=human` mode, `warnings[]` mirrors to stderr as clog `WRN` lines so stdout stays clean for piping.
- In `--output=json` and `--output=compact`, failures write the full error envelope to stdout — the same stream as success — with `ok:false` and a non-zero exit. Parse one stream regardless of outcome; no human diagnostic line is printed to break the parse.
- Warning `type` values you'll see:

  | `type`                       | Meaning                                                                        |
  |------------------------------|--------------------------------------------------------------------------------|
  | `unknown_adf_node`           | ADF node outside the MVP set, preserved opaquely                               |
  | `unknown_adf_mark`           | ADF mark outside the MVP set, preserved opaquely                               |
  | `customfield_unknown_type`   | `customfield_NNNN` key forwarded with no registry schema (Jira handles type)   |
  | `lossy_adf_conversion`       | markdown → ADF or roundtrip dropped detail                                     |
  | `cache-truncated`            | Cache primer hit its page/row safety bound; data fields include `truncated`/`truncated_reason` |
  | `adf-lossy-comment`          | A comment body contains constructs lost on render; entry names `comment_id` and `lossy_constructs[]` |
  | `rate_limit_near`            | A successful response carried Jira's `X-RateLimit-NearLimit`; ease off or lower `--parallelism` before a 429 |

- Exit codes (stable contract — never reused for new categories):

  | Exit | Meaning                                          |
  |------|--------------------------------------------------|
  | 0    | Success                                          |
  | 1    | Authentication failure                           |
  | 2    | Not found                                        |
  | 3    | Validation error / mutex flags / no-input gap    |
  | 4    | Rate limited                                     |
  | 5    | Server error                                     |
  | 6    | Canceled (`code=canceled`: SIGINT or the root context canceled mid-request; retryable) |
  | 7    | Timeout (`code=timeout`: the `--timeout` deadline elapsed; retryable — raise `--timeout`) |

- `--debug` redacts `Authorization`, `Cookie`, and `X-Atlassian-Token` headers to `REDACTED` in the dumped traffic. `Atl-Traceid` is preserved — quote it on Atlassian support tickets.
- Bounded drains (boards cache, etc.) emit a `cache-truncated` warning naming the bound that fired, and surface `truncated` / `truncated_reason` in `data`.
- Read-only blocks happen at the HTTP transport layer — there is no per-command boilerplate to forget.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit 1 | Authentication failure | → `auth_setup` |
| Exit 2 | Not found (issue key, attachment id, link id, profile, etc.) | Re-resolve the identifier; check `auth_setup` if the resource should exist |
| Exit 2, `code=profile_not_defined` | `--profile` names a profile that does not exist | `jira config profile` to list; `jira auth login --profile <name>` to create it |
| Exit 2, `code=profile_incomplete` | The profile exists but has no base URL | `jira auth login --profile <name>` to finish setting it up |
| Exit 2, `code=jira_gone` | The resource was permanently deleted (HTTP 410) | Drop any cached reference to it |
| Exit 3, `code=jira_bad_request` | Jira rejected the request body (HTTP 400) | Check `upstream_messages` / `upstream_field_errors`, correct the input |
| Exit 3, `code=jira_conflict` | The resource changed since it was read (HTTP 409) | Re-fetch the issue, then retry against the latest version |
| Exit 3, `code=flag_unknown` | Unrecognized flag (often a removed legacy boolean like `--json`/`--compact`/`--raw`) | Drop the flag; use `--output=json|compact` |
| Exit 3, `code=flag_foreign` | A flag from a different Jira CLI (e.g. `--plain`, `--gjq`) | Use this CLI's equivalents from `suggestions` |
| Exit 3, `code=flag_value_missing` | A flag that needs a value was given none | Supply the value |
| Exit 3, `code=flag_value_invalid` | Flag value failed type or range parsing | Match the documented value set |
| Exit 3, `code=flag_syntax_invalid` | Malformed flag token | Re-quote / re-escape the argv |
| Exit 3, `code=required_flag_missing` | A required flag was not set; `flag` names the first one | Supply that flag |
| Exit 3, `code=arg_count_invalid` | Wrong number of positional arguments | Match the documented arity |
| Exit 3, `code=arg_value_invalid` | Positional argument value is outside the command's accepted set | Use one of the documented values |
| Exit 3, `code=command_unknown` | Unrecognized command; `suggestions` may carry near-miss names | Use the suggested name |
| Exit 3, `code=read_only` | `JIRA_READ_ONLY=true` or profile `read_only = true` | Unset / flip the profile, or do the work elsewhere |
| Exit 6, `code=canceled` | SIGINT / root context canceled mid-request | Retry when ready |
| Exit 7, `code=timeout` | The `--timeout` deadline elapsed | Raise `--timeout` or retry |
| Exit 3 under `--no-input` with no field flags | Empty headless edit | Pass at least `--summary` / `--assignee` / `--json-input` → `edit_issue` |
| Exit 4 | Rate limited; surfaced after any applicable retry was exhausted or skipped; `errors[0].retry_after_seconds` set when known | Raise `--max-retry-wait` / `JIRA_MAX_RETRY_WAIT`, or wait for the window to reset |
| Exit 5 | Server error (Jira 5xx or a 413 upload-size cap); `errors[0].message` carries the upstream text | Retry if transient; for 413 split or shrink the upload |

**Next**
- Then: → `identity_setup` (resolve `account_id` once per profile so `--assignee me` works)
- Then: → `auth_setup` (when exit 1)
- Then: → `inspect_schema` (when you need the machine-readable command tree, ADF matrix, or customfield registry)
- Composes: → every other workflow inherits this contract.
