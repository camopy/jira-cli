---
title: Watching issues
description: Watch and unwatch issues, and manage the watcher list with watchers add, list, and remove.
icon: material/eye-outline
---

# :eyes: Watching issues

Five verbs over one noun. `watch` and `unwatch` add or remove **you** (the active
profile); `watchers add` and `watchers remove` take an explicit `--user`;
`watchers list` reads the current set. JSON examples below show the `data` block
only — the envelope wrapper and exit codes live on [Output](../output.md), and
each command links to its reference page for the full flag and field tables.

All five take issue-key lists and ranges. A single key returns a flat `data`
object; multiple keys return ordered `data.results[]`, each entry keyed by issue
with its own `ok` and payload. Add `-p` / `--parallelism` to fan the requests out
(default `1`, max `16`).

## watch

Start watching one or more issues as the current user. This is a shortcut for
`jira issue watchers add --user me` — no `--user` flag, because the target is
always you.

```sh
jira issue watch PROJ-123
jira issue watch PROJ-1 PROJ-2 PROJ-3 -p 4
jira issue watch PROJ-123 --dry-run
```

After the mutation the command reads the watcher list back and reports it. The
JSON `data` carries `was_already_watching`, which separates "we added you just
now" from "you were already on the list":

```json
{
  "watchers": [
    { "account_id": "712020:…", "display_name": "John Doe", "email_address": "john.doe@example.com" }
  ],
  "is_watching": true,
  "watch_count": 1,
  "was_already_watching": false
}
```

!!! tip "Skip the read-back"
    `--no-readback` skips the post-mutation GET. `data` then shrinks to
    `account_id` and `attempted: true` — faster, but you lose `watch_count` and
    `was_already_watching`.

[Full flags & output fields →](../reference/jira/issue/watch.md)

## unwatch

Stop watching one or more issues as the current user — the inverse of `watch`,
and a shortcut for `jira issue watchers remove --user me`.

```sh
jira issue unwatch PROJ-123
jira issue unwatch PROJ-1..PROJ-5 -p 4
jira issue unwatch PROJ-123 --dry-run
```

`data` mirrors [`watch`](#watch): `watchers`, `is_watching`, `watch_count`, and
`was_already_watching` (here `true` means you were on the list and have now been
removed). `--no-readback` applies the same way.

[Full flags & output fields →](../reference/jira/issue/unwatch.md)

## watchers list

Read watcher state without changing it. Use it before a mutation, or to ask "am I
on this one?" — `is_watching` answers that without comparing account IDs by hand.
There's no `--user` here; it's a read.

```sh
jira issue watchers list PROJ-123
jira issue watchers list PROJ-1 PROJ-2 PROJ-3 -p 4 --output=json
```

Human output names each watcher and flags whether you're among them:

```text
Watchers  (1 watcher)  (you are watching)
  John Doe  712020:…  john.doe@example.com
```

The JSON `data` is the same shape minus `was_already_watching` (nothing was
mutated):

```json
{
  "watchers": [
    { "account_id": "712020:…", "display_name": "John Doe", "email_address": "john.doe@example.com" }
  ],
  "is_watching": true,
  "watch_count": 1
}
```

[Full flags & output fields →](../reference/jira/issue/watchers/list.md)

## watchers add

Add **someone else** as a watcher. `--user` is required and accepts `me`,
`accountId:<id>`, or an email. The identifier is resolved once and applied to
every key.

```sh
jira issue watchers add PROJ-123 --user me
jira issue watchers add PROJ-123 --user accountId:5b10ac8d82e05b22cc7d4ef5
jira issue watchers add PROJ-123 --user dev@example.com
jira issue watchers add PROJ-123 --user me --dry-run
```

`data` matches [`watch`](#watch) — `watchers`, `is_watching`, `watch_count`,
`was_already_watching` — and `--no-readback` trims it the same way.

!!! warning "Ambiguous or unknown users fail loud"
    An email or name that resolves to zero users exits non-zero with a
    `not_found` error; one that matches several exits with a `validation` error
    listing the candidates, hinting you to re-run with `--user accountId:<id>`.

[Full flags & output fields →](../reference/jira/issue/watchers/add.md)

## watchers remove

Remove a user from the watcher list. Same `--user` contract as
[`watchers add`](#watchers-add): required, resolved once, applied to every key.

```sh
jira issue watchers remove PROJ-123 --user accountId:5b10ac8d82e05b22cc7d4ef5
jira issue watchers remove PROJ-123 --user dev@example.com
jira issue watchers remove PROJ-123 --user me --dry-run
```

`data` mirrors [`watchers add`](#watchers-add). The same not-found and ambiguity
errors apply when an email or name can't be resolved to exactly one account.

[Full flags & output fields →](../reference/jira/issue/watchers/remove.md)

## Dry-run and validation

`--dry-run` is a **local preview that never contacts Jira**. It echoes the
request and resolves `--user` only from local state — `me` (from the active
profile's account ID) or `accountId:<id>`. A bare email or name can't be resolved
offline, so it comes back unresolved:

```json
{
  "issue": { "key": "PROJ-123" },
  "user": "me",
  "dry_run": true,
  "user_resolved": true,
  "account_id_resolved": "712020:…"
}
```

Add `--validate-remote` to a dry-run when you want the user actually checked: it
performs a **read-only** lookup against Jira (never a watcher write) and fills in
`account_id_resolved` even for an email. `user_resolved` stays honest about
whether resolution ran.

## See also

*   [Reading issues](read.md) — `view` also reports watcher detail
*   [Comments](comments.md) — the other per-issue subscription surface
*   [Output](../output.md) — the JSON envelope and exit codes
