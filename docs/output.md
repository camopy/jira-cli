---
title: Output & exit codes
description: The one --output selector, the JSON envelope, typed exit codes, and the debug trace — the contract every command shares.
icon: material/code-json
---

# :outbox_tray: Output & exit codes

`jira` has one global output selector, `--output` (short `-o`), with four
values:

```sh
jira issue list --output=auto
jira issue list --output=human
jira issue list -o json
jira issue list -o compact
```

## Modes

| Mode | Behavior |
|------|----------|
| `auto` | TTY without an agent → human; non-TTY → JSON envelope; detected agent → compact JSON. |
| `human` | Force `clog` rich text regardless of TTY state. |
| `json` | Full envelope on stdout with `ok`, `meta`, `data`, `errors[]`, `warnings[]`. |
| `compact` | The envelope's `data` payload only, no wrapper, with null-valued keys dropped. |

Under `auto`, jira emits compact JSON when it detects an agent (e.g.
`CLAUDECODE`, `CURSOR_TERMINAL`, `AGENT=amp`), the full JSON envelope on any
other non-TTY stream (a pipe, CI), and human output on an interactive terminal.

`compact` is the lean, token-economical view for agents: it emits only the
`data` payload and drops every `null`-valued key, recursively. An absent key
therefore means the value was null. Empty arrays and objects, `false`, and `0`
are kept — they carry meaning. `json` keeps the full, stable schema (nulls
included) for consumers that rely on a fixed shape.

```mermaid
flowchart LR
    Input(["--output=auto"]) --> Agent{"agent detected?"}
    Agent -- Yes --> Compact(["compact JSON"])
    Agent -- No --> TTY{"stdout a TTY?"}
    TTY -- No --> JSON(["JSON envelope"])
    TTY -- Yes --> Human(["human clog output"])

    classDef decision stroke:#d97706,stroke-width:2px
    classDef jsonOut  stroke:#2563eb,stroke-width:2px
    classDef humanOut stroke:#16a34a,stroke-width:2px
    class TTY,Agent decision
    class JSON,Compact jsonOut
    class Human humanOut
```

Failures in `json` and `compact` modes write the full envelope to **stdout** —
the same stream as success — with `ok` set to `false` and a non-zero exit code.
Agents parse one stream regardless of outcome: `cmd … | jq` reads `errors[]`,
`warnings[]`, and `meta.exit_code` on success and failure alike, and no human
diagnostic line is emitted to break the parse. Human mode keeps its diagnostic
on stderr.

## Envelope

```json
{
  "ok": true,
  "meta": {
    "command": "issue.list",
    "timestamp": "2026-05-26T05:03:18Z",
    "request_id": "337f5bd1a5f24bb88da5510cb801f62d"
  },
  "data": {},
  "errors": [],
  "warnings": []
}
```

*   **`ok`**, true on success, false on any non-zero exit.
*   **`meta.exit_code`**, present only on failure, equal to the process exit
  code (see table below).
*   **`meta.request_id`**, a 32-character hex id for correlating CLI invocations
  with logs. Generated locally; it has no meaning to Atlassian.
*   **`meta.upstream_request_id`**, Jira's own trace id (the `Atl-Traceid`
  or `X-ARequestId` response header) when the command reached Jira. Quote
  this one to Atlassian support; it also rides on each `errors[]` entry.
*   **`data`**, command-specific payload. `compact` emits only this value.
*   **`errors[]`**, populated on failure: `{type, code, message, hint,
  retryable, …}`. Agents branch on `code` (stable snake_case), never on
  `message`.
*   **`warnings[]`**, non-fatal advisories (e.g. ADF conversion approximations).

## Exit codes

| Code | `code` | Meaning |
|------|--------|---------|
| 0 | — | Success |
| 1 | `auth_failed` | Invalid, expired, or missing credential |
| 2 | `not_found` | Issue, project, board, or user doesn't exist for this token |
| 3 | `validation_failed` | Bad flags, malformed input, or Jira rejected the value |
| 4 | `rate_limited` | Jira 429; `retry_after_seconds` rides on the error record |
| 5 | `server_error` | Jira 5xx, or a local filesystem / runtime failure |
| 6 | `canceled` | The operation was cancelled (e.g. Ctrl-C) |
| 7 | `timeout` | The operation exceeded its deadline |

The `code` column is the stable branch key on `errors[]`; the matching error
`type` values are `auth`, `not_found`, `validation`, `rate_limit`, and `server`.

The action label (e.g. `Issue created`) goes to stdout only on success. JSON
mode keeps the same envelope on success and failure.

## Debug

`-d` / `--debug` prints the HTTP request and response trace to **stderr**. It
never touches stdout — in `json` mode stdout stays a clean, parseable envelope,
so `jira … --output=json --debug | jq` still works. The `Authorization` header
is always redacted; request and response bodies are redacted for known secret
fields. The trace is identical in shape for every networked command, so it is
documented once here; other pages just note that `-d` is available.

```sh
jira me --output=json --debug
```

=== "stdout (envelope)"

    The envelope is unchanged by `--debug`:

    ```json
    {
      "ok": true,
      "meta": { "command": "me", "timestamp": "…", "request_id": "…" },
      "data": {
        "account_id": "712020:1a2b3c…",
        "display_name": "Example User",
        "email_address": "you@example.com",
        "profile": "default",
        "time_zone": "Etc/UTC"
      },
      "errors": [],
      "warnings": []
    }
    ```

=== "stderr (debug trace)"

    The redacted HTTP request and response, as `clog` `DBG` lines (abbreviated):

    ```text
    DBG 🐞 output detection mode=json agent=…
    DBG 🐞 jira request method=GET url=https://your-site.atlassian.net/rest/api/3/myself headers.Accept=application/json headers.Authorization=REDACTED
    DBG 🐞 jira response status_code=200 status="200 OK" headers.X-Ratelimit-Remaining=349 …
    DBG 🐞 jira response body body="{\"accountId\":\"712020:1a2b3c…\",\"displayName\":\"Example User\",…}"
    DBG 🐞 fetched account time=287ms
    ```

On a failure the envelope still goes to stdout (machine mode) or the `ERR` line
to stderr (human mode); `--debug` adds the same trace alongside, so the response
status and body that explain the failure are visible without changing the
parse contract.

## Per-command output

Each command page shows its own output, with a JSON `data` example where the
shape isn't obvious — see [`auth`](auth.md), [`issue`](issue/read.md),
[`cache`](cache.md), and so on. The envelope above is constant; only the `data`
payload varies per command.

## Further reading

*   [`agent schema`](agent.md#schema), the full command tree and
  exit-code contract in JSON, for programmatic consumers.
