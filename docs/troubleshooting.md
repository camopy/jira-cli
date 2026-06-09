# Troubleshooting

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

See [Configuration › Precedence](config.md#precedence) for the full
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
    Run [`jira auth login`](auth.md#auth-login). If `secret_backend = "env"`,
    set `JIRA_TOKEN_<PROFILE>` (uppercase, `-` → `_`).

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

Or nuke and re-prime:

```sh
jira cache clear --output=json
jira issue list --project <PROJECT_KEY> --output=json   # any read repopulates what's needed
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

`exit 3` with `screen schema could not be resolved in strict mode`
:   `issue create --json-input` got the edit-shape payload
    (`{"fields": {...}}`) instead of the flat-alias shape
    (`{"project_key": ..., "issue_type": ...}`). See [Issues › `## create`](issues.md#create)
    for the payload-shape warning.

`exit 3` with `validation: --json-input payload has no recognized fields`
:   Inverse of the above: `issue edit --json-input` got a flat
    payload. Wrap it under `{"fields": {...}}`.

`exit 1` with `auth_failed` on a previously-working profile
:   Token rotated upstream. Re-run `auth login`.

`exit 2` with `not_found`
:   Issue / project / board / user doesn't exist *for this token*. A
    403 sometimes surfaces as 404 — check
    [`auth status`](auth.md#auth-status) to rule out a permission
    issue first.

`exit 4` with `rate_limit`
:   Jira rate-limited the request (HTTP 429), surfaced after auto-retry
    was exhausted or skipped. Reads wait and resend within
    `--max-retry-wait` (default `30s`); mutations and `--max-retry-wait=0`
    fail on the first 429. The envelope carries
    `errors[0].retry_after_seconds`. Raise `--max-retry-wait` (or
    `JIRA_MAX_RETRY_WAIT`), or wait for the window to reset. See
    [Configuration › Rate-limit retry](config.md#rate-limit-retry).

`exit 5` with `server_error`
:   Jira's side, or a local IO failure (cache directory not writable,
    config file permissions). Check
    [status.atlassian.com](https://status.atlassian.com/) first;
    re-run with `--debug` if it persists.

`config init` exits with `required_flag_missing`
:   The command refuses to write unless both `--base-url` and `--email`
    are passed; either flag missing is a validation error (exit 3),
    not a silent overwrite. Re-run with both flags. See
    [Configuration › init](config.md#init).

## Debug invocation

When the four checks above don't surface the cause, escalate to a
debug capture:

```sh
jira --debug <subcommand> ... --output=json
```

`--debug` logs the full HTTP roundtrip (request URL, headers, response
status, response body) to stderr. The token is redacted. `request_id`
in `meta.request_id` correlates the CLI invocation with Atlassian's
side; quote it when filing a Jira support ticket.

## See also

*   [Auth › Troubleshooting access](auth.md#troubleshooting-access) — the error taxonomy from auth.md's perspective.
*   [Output › Exit codes](output.md#exit-codes) — the typed-exit-code contract.
*   [Agents › `core_contract`](agent.md#guides) — the same recovery surface from an agent-runbook angle.
*   [Configuration](config.md) — env vars and config keys this page references.
