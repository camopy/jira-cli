## manage_watchers
Goal: Resolve, add, remove, or audit watchers on a Jira issue while handling ambiguous user input via structured `candidates[]`.
When: notifications for an issue must reach an extra account, or the current watcher set needs to be audited or pruned.

**Decide**

# direction
- List: `jira issue watchers list KEY`.
- List several issues/ranges: `jira issue watchers list KEY... -p N`; multi-key output uses `data.results[]`.
- Add: `jira issue watchers add KEY... --user <user>` (alias `jira issue watch KEY...`).
- Remove: `jira issue watchers remove KEY... --user <user>` (alias `jira issue unwatch KEY...`).

# user spec
- Account id (always locally resolvable): `--user accountId:<id>`.
- Self: `--user me` (locally resolvable when the active profile carries an account id).
- Display name or email: needs a remote `/user/search` to resolve — see Behavior for dry-run rules.

# guards
- `--dry-run` is local-only — it contacts Jira for nothing unless paired with `--validate-remote`.
- `--validate-remote` alongside `--dry-run` opts into a read-only resolve (still no watcher `POST`/`DELETE`).

**Run**
- List: `jira issue watchers list KEY --output=json`
- Multi-key list: `jira issue watchers list <PROJECT_KEY>-1..10 -p 4 --output=json`
- Add: `jira issue watchers add KEY --user me --output=json`
- Bulk add: `jira issue watchers add <PROJECT_KEY>-1..10 -p 4 --user me --output=json`
- Remove named user: `jira issue watchers remove KEY --user accountId:<id> --output=json`
- Remove yourself (alias): `jira issue unwatch KEY --output=json`
- Dry-run preview: `jira issue watchers add KEY --user me --dry-run --output=json`
- Dry-run + remote resolve: `jira issue watchers add KEY --user alice --dry-run --validate-remote --output=json`

**Save**
> Requires `--output=json`.

`watchers list` envelope — `is_watching` / `watch_count` mirror Atlassian's native shape:

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

`watchers add` / `watch` (default readback):

```json
{
  "data": {
    "issue": {"key": "<ISSUE_KEY>"},
    "watchers": [{"account_id": "...", "display_name": "Alice", "active": true}],
    "was_already_watching": false,
    "dry_run": false
  }
}
```

`watchers add --dry-run` (locally resolvable input):

```json
{"data": {"issue": {"key": "<ISSUE_KEY>"}, "user": "accountId:712020:abc", "account_id_resolved": "712020:abc", "user_resolved": true, "dry_run": true}}
```

- `data.watchers[].account_id` [string, required] — stable identity; pass back as `accountId:<id>` to subsequent calls.
- `data.watchers[].display_name` / `.email_address` [string, optional] — display fields; `email_address` may be `null` on privacy-restricted directories.
- `data.watchers[].active` [bool, required] — Jira account active flag.
- `data.is_watching` [bool, required] — whether the calling identity is in the list.
- `data.watch_count` [int, required] — total watcher count (may exceed `len(data.watchers)` when truncated).
- Multi-key list/add/remove/watch/unwatch: `data.results[]` [array, required] — ordered by requested key; each successful entry has command-specific `data`.
- `data.was_already_watching` [bool, required on add] — `true` makes the call effectively a no-op.
- `data.user_resolved` [bool, required on dry-run] — `false` when the input needs a remote lookup that dry-run skipped.
- `data.account_id_resolved` [string, optional on dry-run] — only present when local resolution succeeded.
- `data.dry_run` [bool, required on dry-run] — always `true` for previews.

**Preconditions**
- Bare names/emails cannot be locally resolved; without `--validate-remote`, dry-run echoes them back with `user_resolved: false` and no `account_id_resolved`.
- `--user me` resolves locally only when the active profile carries an account id (see → `auth_setup` and → `identity_setup`).

**Behavior**
- `watchers add` and `watchers remove` perform the mutation and then read back the watcher list unless `--no-readback` is set.
- Aliases `watch` / `unwatch` are sugar — identical envelopes, identical exit codes.
- For multi-key watcher mutations, the user identifier is resolved once, then the per-issue mutation/readback runs with bounded `-p` / `--parallelism` concurrency.
- Every error entry carries `type`, `code` (stable snake_case — branch on this, never on `message`), `message`, `hint`, and `retryable`. Optional fields appear when relevant: `flag`, `field`, `http_status`, `retry_after_seconds`, `provider`, `upstream_code`, `upstream_status`. For Jira API errors `upstream_code` is empty — Jira exposes no stable machine error code.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, `code: validation_failed`, `candidates[]` populated | `--user` matched >1 directory entry | Pick a `candidates[].account_id` and re-run with `--user accountId:<id>` |
| exit 2, `code: not_found` | `--user` matched zero directory entries; input echoed in `errors[0].message` | Refine the spelling or use `accountId:<id>` |
| exit 3 on a flag (`flag_unknown`, `flag_value_invalid`, `required_flag_missing`, `arg_count_invalid`, `command_unknown`, ...) | Malformed invocation — fails before any Jira call | See → `core_contract` for the canonical command-line input error table; `hint` may carry a "Did you mean …?" |

Ambiguous-user resolution → exit 3 with structured candidates so the agent can re-run with `--user accountId:<id>`. This envelope is the canonical template for every command that surfaces ambiguous user / option resolution:

```json
{
  "ok": false,
  "meta": {"command": "issue.watchers.add", "exit_code": 3, "timestamp": "..."},
  "data": null,
  "warnings": [],
  "errors": [
    {
      "type": "validation",
      "code": "validation_failed",
      "message": "ambiguous user 'alice' — 3 candidates",
      "hint": "Re-run with --user accountId:<id>.",
      "retryable": false,
      "candidates": [
        {"account_id": "1", "display_name": "Alice Smith", "email_address": "alice.smith@example.com"},
        {"account_id": "2", "display_name": "Alice Jones", "email_address": "alice.jones@example.com"},
        {"account_id": "3", "display_name": "Alice Brown", "email_address": null}
      ]
    }
  ]
}
```

**Next**
- Then: → `read_issue` to confirm the resulting watcher state in the issue context.
- Alternative: → `add_comment` to ping someone instead of subscribing them (watchers are notification side-effects, not @-mentions).
