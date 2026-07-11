---
title: Troubleshooting
description: Diagnose a misbehaving jira invocation with a four-step decision tree.
icon: material/lifebuoy
---

# :wrench: Troubleshooting

Single decision tree for diagnosing a misbehaving `jira` invocation.
Walk the four checks in order — most failures fall out by step 3.

## 1. Is the binary what you think it is?

```sh
jira version --output=json
```

Confirms the build SHA and version. If `command not found`, see
[Installation](installation.md). If a binary is on `$PATH` but
returns the wrong version, the most common cause is an old install
shadowing a Homebrew or `go install` update — check `which -a jira`.

## 2. Which config and profile are active?

```sh
jira config profile --output=json
```

`data.active_profile` names the profile every other command will use.
If it isn't what you expect:

*   `--profile <name>` on the failing command overrides everything.
*   `JIRA_DEFAULT_PROFILE` environment variable overrides the file default.
*   `default_profile` in `~/.config/jira-cli/config.toml` is the file default.

See [Configuration › Precedence](config.md#what-overrides-what) for the full
resolution order.

## 3. Are credentials wired correctly?

```sh
jira auth status --output=json
```

`data.profiles[]` lists every configured profile with `valid: true|false`
and a per-profile `error` field. Top-level `ok: false` means at least
one is unhealthy.

Map the per-profile `error` value to the next step:

`credential not found`
:   The keyring (or 1Password) doesn't have an entry for this profile.
    Run [`jira auth login`](auth.md#log-in), or supply the token inline with
    `JIRA_TOKEN_<PROFILE>` (the profile name, uppercased).

`the OS keyring is unavailable` (`keyring_unavailable`)
:   No Secret Service answered — the normal state on WSL and headless Linux,
    where there's no gnome-keyring on the session D-Bus. Switch the profile to
    the env backend: export `JIRA_TOKEN_<PROFILE>` and run
    `jira auth login --backend env`. (Or install and start a Secret Service.)

`environment variable JIRA_TOKEN_<PROFILE> is not set` (`env_credential_unset`)
:   The profile uses the env backend but the variable isn't exported in this
    shell. Export it, or wrap the invocation in your injector
    (`op run -- jira ...`). The name is derived from the profile —
    `JIRA_TOKEN_DEFAULT` for the default profile — not `JIRA_API_TOKEN`.

`auth_failed` (HTTP 401)
:   Token is wrong or expired. Rotate it at
    [id.atlassian.com](https://id.atlassian.com/manage-profile/security/api-tokens)
    and re-run `auth login`. The error envelope's `hint` field tells
    you the exact remediation.

`auth_failed` (HTTP 403)
:   Token is valid but lacks permission for this Jira project.
    Check the project's permission scheme; the token's user needs
    Browse Projects + whatever the call requires (Create Issues,
    Transition Issues, etc.).

`server_error` / network timeout
:   `base_url` may be wrong, or Jira is down. Re-check the URL with
    `jira config get profiles.<name>.base_url` and try `curl -I <base_url>/rest/api/3/myself`
    with the token in an `Authorization: Basic` header.

## 4. Is the local cache the problem?

If a `--project` / `--type` / `--board` / `--label` filter resolves to
an unexpected value (or returns 404 for something that exists in
Jira), the local cache may be stale.

```sh
jira cache projects --refresh --output=json
jira cache fields --refresh --output=json
jira cache issuetypes --refresh --output=json
```

Or nuke and re-prime — any read repopulates what's needed:

```sh
jira cache clear --output=json
jira issue list --project PROJ --output=json
```

See [Cache](cache.md) for the full resource list.

## Symptom → next step

`exit 3` with `code: validation_failed` and a `field_errors` map
:   Jira rejected one or more fields. The envelope's
    `errors[0].upstream_field_errors` carries Jira's per-field
    complaint. Common cases: required field missing under
    `--no-input`, customfield value shape wrong (see
    [Custom fields](custom-fields.md)), unknown issue type for the
    target project.

`exit 3` with `code: arg_value_invalid`
:   The positional argument isn't in the allowed set. The message
    lists the valid values. Common cases: `cache clear <unknown>`,
    `config theme --name <unknown>`.

`exit 3` with `code=issue_type_unknown`
:   The `--type` value names no issue type on the project's create
    screen. The valid type names are returned in `errors[0].suggestions`;
    pick one of those.

`exit 3` with `screen schema could not be resolved in strict mode`
:   Strict mode could not fetch the create screen for the payload's
    project — usually an unknown project key, or no live connection to
    resolve the screen. Check the project with [`cache projects`](cache.md).

`exit 1` with `auth_failed` on a previously-working profile
:   Token rotated upstream. Re-run `auth login`.

`exit 2` with `not_found`
:   Issue / project / board / user doesn't exist *for this token*. A
    403 sometimes surfaces as 404 — check
    [`auth status`](auth.md#when-access-fails) to rule out a permission
    issue first.

`exit 4` with `rate_limit`
:   Jira rate-limited the request (HTTP 429), surfaced after auto-retry
    was exhausted or skipped. Reads wait and resend within
    `--max-retry-wait` (default `30s`); mutations and `--max-retry-wait=0`
    fail on the first 429. The envelope carries
    `errors[0].retry_after_seconds`. Raise `--max-retry-wait` (or
    `JIRA_MAX_RETRY_WAIT`), or wait for the window to reset. See
    [Configuration › Rate-limit retry](config.md#behaviour-toggles).

`exit 5` with `server_error`
:   Jira's side, or a local IO failure (cache directory not writable,
    config file permissions). Check
    [status.atlassian.com](https://status.atlassian.com/) first;
    re-run with `--debug` if it persists.

`config init` exits with `required_flag_missing`
:   The command refuses to write unless both `--base-url` and `--email`
    are passed; either flag missing is a validation error (exit 3),
    not a silent overwrite. Re-run with both flags. See
    [Configuration › init](config.md#profiles).

## It looks broken, but it's the contract

Behaviours that read like bugs but are deliberate. Check here before filing one.

**Compact output is missing a field I expected.**
:   `--output=compact` drops every `null`-valued key, recursively, to stay
    token-lean — so an absent key means the value *was* null, not that the field
    doesn't exist. Re-run with `--output=json` for the full schema, nulls kept.
    See [Output › Modes](output.md#modes).

**A create succeeded but `.data.key` is empty.**
:   The new key is at `data.issue.key`, not `data.key` — create wraps the issue
    object. (A `--dry-run` returns `data.preview` instead, with `dry_run: true`.)

**One bad key failed a multi-key command — did it roll back?**
:   No. Multi-key commands have no rollback; the good keys were committed. Read
    the per-key `data.results[]` (`ok` / `error`) and retry only the failures —
    a blind retry double-applies the ones that already succeeded.

**A search result has no `meta.pagination.total`.**
:   Jira's token-paginated `/search/jql` returns no reliable total, so the CLI
    omits the field rather than fabricate a `0`. Paginate on
    `meta.pagination.isLast` / `nextCursor` (pass the cursor back via
    `--cursor`), use `--count` for a number, or `--all` to drain (bounded to
    100 pages / 10 000 issues, `--unbounded` lifts the caps). See [Search](search.md#search-jql).

**`--time-spent "1h 30m"` fails with `invalid duration`.**
:   Durations are space-free: `1h30m`, `2d4h`, `45m`. Units are `d` / `h` / `m`
    only, and fractional values (`1.5h`) are rejected — combine whole units.

**A Markdown body lost its @mentions, dates, or panels, with no warning.**
:   Those ADF constructs have no Markdown spelling, so `--markdown` /
    `description_markdown` can't emit them — and `--adf-strict` can't flag what
    never enters the pipeline. Author rich bodies as native ADF via
    `--json-input`; keep Markdown for plain prose. See [ADF](adf.md).

**`--dry-run` passed but the real submit failed with a 400 / `INVALID_INPUT`.**
:   Dry-run validates local shape only; it never contacts Jira. The server
    applies its own rules on write — required custom fields, its ADF schema, an
    unknown node that round-tripped fine on read. A clean dry-run means
    "well-formed", not "Jira will accept it".

**`--assignee you@example.com --dry-run` fails, but works on a real submit.**
:   An email assignee is resolved through a live `/user/search`, which dry-run
    won't call. Pre-resolve to `accountId:<id>` for the preview, or drop
    `--dry-run` for the email path.

**`JIRA_PROFILE=…` doesn't change which profile runs.**
:   There is no bare `JIRA_PROFILE` selector. Pick the profile with `--profile
    <name>`, `JIRA_DEFAULT_PROFILE`, or `default_profile` in the config.
    (`JIRA_PROFILE_<NAME>_*` is a different feature — per-profile field overrides.)

**`issue edit KEY` exits 3 under an agent or in a pipe.**
:   The bare form opens `$EDITOR` on the description, which needs a terminal, so
    a non-TTY or agent context refuses it rather than hang. Pass `--summary`,
    `--assignee`, `--markdown`, or `--json-input`.

**A destructive command refuses without `--force`.**
:   `--no-input` removes the confirmation prompt, so `--force` is its explicit
    replacement on `delete` / `clone` / `move` / the `*-delete` verbs. Multi-key
    delete needs `--force` even at an interactive terminal. Add
    `--delete-subtasks` for a parent that has subtasks.

## Debug invocation

When the four checks above don't surface the cause, escalate to a
debug capture:

```sh
jira --debug <subcommand> ... --output=json
```

`--debug` logs the full HTTP roundtrip (request URL, headers, response
status, response body) to stderr. The token is redacted.
`meta.upstream_request_id` carries Jira's own trace id (the
`Atl-Traceid` response header) whenever the command reached Jira; quote
it when filing a Jira support ticket. `meta.request_id` is a locally
generated id for correlating CLI invocations with your own logs — it
has no meaning to Atlassian.

## See also

*   [Auth › Troubleshooting access](auth.md#when-access-fails) — the error taxonomy from auth.md's perspective.
*   [Output › Exit codes](output.md#exit-codes) — the typed-exit-code contract.
*   [Agents › `core_contract`](agent.md#guides) — the same recovery surface from an agent-runbook angle.
*   [Configuration](config.md) — env vars and config keys this page references.
