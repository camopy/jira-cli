# Output

`jira` has one global output selector:

```sh
jira issue list --output=auto
jira issue list --output=human
jira issue list --output=json
jira issue list --output=compact
```

## Modes

| Mode | Behavior |
|------|----------|
| `auto` | TTY uses human output; non-TTY uses JSON; detected agents use compact JSON. |
| `human` | Force `clog` rich text. |
| `json` | On success, write the full envelope to stdout. |
| `compact` | On success, write only the envelope `data` value. |

Errors in `json` and `compact` modes write the full envelope to stderr and leave
stdout empty, so agents can parse `errors[]`, `warnings[]`, and `meta.exit_code`
without poisoning success pipelines.

## Envelope

```json
{
  "ok": true,
  "meta": {
    "command": "issue.list",
    "timestamp": "2026-05-17T05:03:18Z",
    "request_id": "..."
  },
  "data": {},
  "errors": [],
  "warnings": []
}
```

Exit codes are stable:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Authentication failure |
| 2 | Not found |
| 3 | Validation error |
| 4 | Rate limited |
| 5 | Server error |
