# jira auth

**Configure** Jira profiles and credential backends. `auth login` validates a
credential with Jira and stores a structured backend reference in the local
keyring (or 1Password). The TOML config holds the reference; it never holds the
raw token.

Profiles live in `~/.config/jira-cli/config.toml`. Each profile records the
base URL, email, auth type, and a backend reference, but no secret.

jira talks to **Jira Cloud only**. Server and Data Center are not supported.

## Getting started

Before `auth login`, gather:

*   An <abbr title="A subdomain of atlassian.net, e.g. https://example.atlassian.net">Atlassian Cloud site</abbr> (jira targets Cloud only).
*   The account email tied to that site.
*   A classic
    [API token](https://id.atlassian.com/manage-profile/security/api-tokens)
    tied to that account.
*   A <abbr title="Short local name jira uses to look up the credential, defaults to 'default'. Multiple profiles let you point at different Atlassian tenants.">profile name</abbr>.
*   A <abbr title="Interactive prompt, --secret-stdin, --credential-env, or the per-profile JIRA_TOKEN_{PROFILE} environment variable.">token source</abbr>.

Then run the three-command bootstrap:

```sh
jira config init --base-url https://example.atlassian.net --email john.doe@example.com
jira auth login        # store the API token (interactive on a TTY)
jira auth status       # confirm /myself and /mypermissions probes succeed
```

`auth status` is the access diagnostic, it probes Jira on every run and
distinguishes a bad token (401) from insufficient permissions (403). See
[Troubleshooting access](#troubleshooting-access) for the error taxonomy.

## auth login

Create a profile and store a credential, or replace the credential on an
existing profile.

On a TTY with no flags, `auth login` opens an interactive form. It walks
through three common fields:

1.  **Profile name**, defaults to `default`.
2.  **Jira site**, the Atlassian site (e.g. `example` for
    `example.atlassian.net`, or a full `https://…` URL).
3.  **Atlassian account email**, sent as the basic-auth username.

Then asks which **secret backend** to use, and the rest of the prompts
depend on that choice:

=== "System keychain"

    *macOS Keychain, Windows Credential Manager, or Linux Secret Service.*

    4.  **API token**, Atlassian API token. Stored directly in the OS
        keychain under service `jira-cli`, account `<host>/<profile>`.

=== "1Password"

    *Credential lives in a 1Password vault, read via the Go SDK.*

    Desktop-app SDK authorization is per account and per process. Because each
    `jira` invocation starts a new process, commands that read this backend can
    prompt again even when the 1Password app is unlocked. Approval can use
    biometrics or system unlock; it is not a reusable silent session. For
    prompt-free day-to-day commands, use the system keychain backend after
    bootstrap.

    4.  **1Password account**, desktop-app account name for SDK auth.
        Leave blank to use the `OP_SERVICE_ACCOUNT_TOKEN` env var instead
        (headless / CI).
    5.  **1Password vault**, vault holding the item.
    6.  **1Password item**, item title for this Jira profile.
    7.  **API token**, the Atlassian token, stored in the configured
        1Password item.

The raw token never lands in the TOML; the config only records the backend
reference.

With `--no-input`, every field that isn't already on disk must arrive via
a flag or env var (see the scenarios below). `--no-input` is a root
persistent flag that disables prompting across the whole CLI.

=== "Interactive"

    TTY only; opens the five-field form (or the seven-field 1Password form).

    ```sh
    jira auth login
    ```

=== "bash · zsh"

    Two equivalent ways to deliver the secret without putting it in argv.

    **Token via env var** (`--credential-env` reads the env var by name):

    ```bash
    JIRA_TOKEN="$(your-secret-source)" jira auth login --no-input \
      --profile-name default \
      --base-url https://example.atlassian.net \
      --email john.doe@example.com \
      --backend keyring \
      --credential-env JIRA_TOKEN
    ```

    **Token via stdin** (pipe from `cat`, `op read`, `pass`, `pbpaste`, etc.):

    ```bash
    your-secret-source | jira auth login --no-input \
      --profile-name default \
      --base-url https://example.atlassian.net \
      --email john.doe@example.com \
      --backend keyring \
      --secret-stdin
    ```

=== "fish"

    Two equivalent ways to deliver the secret without putting it in argv.

    **Token via env var** (`--credential-env` reads the env var by name):

    ```fish
    set -lx JIRA_TOKEN (your-secret-source)
    jira auth login --no-input \
      --profile-name default \
      --base-url https://example.atlassian.net \
      --email john.doe@example.com \
      --backend keyring \
      --credential-env JIRA_TOKEN
    ```

    **Token via stdin** (pipe from `cat`, `op read`, `pass`, `wl-paste`, etc.):

    ```fish
    your-secret-source | jira auth login --no-input \
      --profile-name default \
      --base-url https://example.atlassian.net \
      --email john.doe@example.com \
      --backend keyring \
      --secret-stdin
    ```

!!! note "`--secret-stdin` and `--credential-env` are mutually exclusive"
    Pick the input source that fits your shell. For CI, `--credential-env`
    keeps the secret out of process argv *and* out of the command-line
    history of the shell that spawned the runner.

=== "Human"

    ```text
    ✓ Logged in as John Doe
    INF ℹ️ logged in account_id=712020:00000000-0000-4000-8000-000000000000 auth_type=token display_name="John Doe" profile=default secret_backend=keyring skip_verify=false stored_secret=true verified=true
    ```

    With `--skip-verify`, the `✓ Logged in as …` line is omitted and
    `verified=false`. With Jira rejecting the token, the command exits 1
    and emits the standard `auth_failed` error envelope (see
    [Troubleshooting access](#troubleshooting-access)).

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "auth.login", "timestamp": "2026-05-27T00:00:00Z", "request_id": "…" },
      "data": {
        "account_id": "712020:00000000-0000-4000-8000-000000000000",
        "auth_type": "token",
        "display_name": "John Doe",
        "onepassword_account": "",
        "profile": "default",
        "secret_backend": "keyring",
        "skip_verify": false,
        "stored_secret": true,
        "verified": true
      },
      "errors": [],
      "warnings": []
    }
    ```

    `account_id` and `display_name` are present only when the token
    was verified against `/myself` (omitted under `--skip-verify`).

=== "Skip verify"

    With `--skip-verify`, the binary stores the credential without
    calling `/myself`, so the identity fields are absent and
    `verified` is `false`:

    ```json
    {
      "ok": true,
      "meta": { "command": "auth.login", "timestamp": "2026-05-27T00:00:00Z", "request_id": "…" },
      "data": {
        "auth_type": "token",
        "onepassword_account": "",
        "profile": "default",
        "secret_backend": "keyring",
        "skip_verify": true,
        "stored_secret": true,
        "verified": false
      },
      "errors": [],
      "warnings": []
    }
    ```

    The human output drops the `✓ Logged in as …` line for the same
    reason, without `/myself`, there's no name to print.

=== "Error (401)"

    ```json
    {
      "ok": false,
      "meta": { "command": "auth.login", "exit_code": 1, "timestamp": "…", "request_id": "…" },
      "data": null,
      "errors": [
        {
          "type": "auth",
          "code": "auth_failed",
          "message": "invalid Atlassian account email or API token, Jira rejected the credential (HTTP 401); check the email and that the API token is current at id.atlassian.com, or pass --skip-verify to store it without checking",
          "hint": "",
          "retryable": false
        }
      ],
      "warnings": []
    }
    ```

## auth status

The **access diagnostic**. Probes `/myself` and `/mypermissions` against each
configured profile and reports whether the stored credential currently works.
A 401 means the token is bad, a 403 means the token is fine but lacks
permission for the project or field you probed.

```sh
jira auth status
jira auth status --project PROJ          # narrow the permission probe to one project
jira auth status --no-probe              # skip the remote calls; fast offline check
```

=== "Human"

    ```text
    Profile:    default (active)
      Credential:  ✓ keyring  ****ABCD
      Site:        https://example.atlassian.net
      Identity:    ✓ john.doe@example.com
      Permissions: ✓ all 8 · ✓ add_comment  ✓ browse  ✓ create  ✓ delete  ✓ edit  ✓ link  ✓ transition  ✓ work_on
    ```

    With `--no-probe`, the remote section collapses to a single
    `Remote: (probe skipped)` line.

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": {
        "command": "auth.status",
        "timestamp": "2026-05-27T00:00:00Z",
        "request_id": "…"
      },
      "data": {
        "active_profile": "default",
        "profiles": [
          {
            "profile": "default",
            "redacted": "****ABCD",
            "source": "keyring",
            "valid": true,
            "remote": {
              "site": "https://example.atlassian.net",
              "myself": {
                "account_id": "712020:00000000-0000-4000-8000-000000000000",
                "email": "john.doe@example.com",
                "ok": true
              },
              "permissions": {
                "grants": {
                  "ADD_COMMENTS": true,
                  "BROWSE_PROJECTS": true,
                  "CREATE_ISSUES": true,
                  "DELETE_ISSUES": true,
                  "EDIT_ISSUES": true,
                  "LINK_ISSUES": true,
                  "TRANSITION_ISSUES": true,
                  "WORK_ON_ISSUES": true
                },
                "ok": true
              }
            }
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

    With `--no-probe`, the `remote` block is omitted entirely and each
    profile carries only `profile`, `redacted`, `source`, and `valid`.

!!! tip "Use `--no-probe` in tight script loops"
    The default probe issues two HTTPS calls per profile. For health checks
    that only need to confirm a credential is configured (not that Jira is
    reachable right now), `--no-probe` skips the network round-trip.

## auth token

Show **redacted token diagnostics** for the active profile, which backend the
secret came from, whether it's valid, when it expires (if known), and the last
four characters of the resolved token. The raw secret is never printed.

```sh
jira auth token
```

=== "Human"

    ```text
    INF ℹ️ backend=keyring profile=default redacted=****ABCD source=keyring valid=true
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "auth.token", "timestamp": "2026-05-27T00:00:00Z", "request_id": "…" },
      "data": {
        "backend": "keyring",
        "profile": "default",
        "source": "keyring",
        "redacted": "****ABCD",
        "valid": true,
        "expiry": null,
        "error": ""
      },
      "errors": [],
      "warnings": []
    }
    ```

The `source` field distinguishes where the credential came from on this
invocation: `keyring`, `1password`, or `env` (the `JIRA_TOKEN_<PROFILE>`
override). When `source` is `env`, you know the env var won precedence over
the stored backend.

## auth whoami

Resolve the authenticated identity and (with `--save`) cache the account ID on
the active profile. Once cached, `--assignee me` skips the round-trip.

```sh
jira auth whoami
jira auth whoami --save
```

=== "Human"

    ```text
    INF ℹ️ account_id=712020:00000000-0000-4000-8000-000000000000 account_type=atlassian display_name="John Doe" email_address=john.doe@example.com profile=default saved=false time_zone=America/Los_Angeles
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "auth.whoami", "timestamp": "2026-05-27T00:00:00Z", "request_id": "…" },
      "data": {
        "account_id": "712020:00000000-0000-4000-8000-000000000000",
        "account_type": "atlassian",
        "display_name": "John Doe",
        "email_address": "john.doe@example.com",
        "profile": "default",
        "saved": false,
        "time_zone": "America/Los_Angeles"
      },
      "errors": [],
      "warnings": []
    }
    ```

=== "JSON (--save)"

    With `--save`, the only field that changes is `saved`. The data
    block is otherwise identical, and the account ID is also written
    to the active profile's config so `--assignee me` resolves
    offline.

    ```json
    {
      "ok": true,
      "meta": { "command": "auth.whoami", "timestamp": "2026-05-27T00:00:00Z", "request_id": "…" },
      "data": {
        "account_id": "712020:00000000-0000-4000-8000-000000000000",
        "account_type": "atlassian",
        "display_name": "John Doe",
        "email_address": "john.doe@example.com",
        "profile": "default",
        "saved": true,
        "time_zone": "America/Los_Angeles"
      },
      "errors": [],
      "warnings": []
    }
    ```

`saved` reports whether `--save` persisted the account ID to the profile
config on this invocation. When the cached value already matches what
`/myself` returns, `saved: true` still fires (the write is idempotent;
no diff on disk, but the persistence path ran).

## auth switch

Change the default profile selected by `--profile`.

```sh
jira auth switch work
```

The argument is the profile key as it appears in `jira config profile list`.

## auth logout

Remove the credential from the backend and clear the auth reference on the
profile. The profile name is required.

```sh
jira auth logout work
```

=== "Human"

    ```text
    INF ℹ️ logged out profile=work removed=true
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "auth.logout", "timestamp": "2026-05-27T00:00:00Z", "request_id": "…" },
      "data": {
        "profile": "work",
        "removed": true
      },
      "errors": [],
      "warnings": []
    }
    ```

!!! warning "Destructive, the keyring (or 1Password) entry is deleted"
    There is no flag to preserve the backend item, `auth logout` always
    removes the secret. Re-running `auth login` will need a fresh token
    paste (or `--credential-env` / `--secret-stdin`). Subsequent
    `auth status` reports `error: "credential not found"` and
    `valid: false` for that profile until the next login.

## auth migrate

Move the active profile's credential between backends without losing the
stored secret. Useful when switching from the OS keyring to 1Password (or
back) on an already-authenticated machine.

```sh
jira auth migrate --backend 1password \
  --onepassword-account my-account \
  --vault Work \
  --item "Jira API token"

jira auth migrate --backend keyring          # migrate 1Password -> keyring
jira auth migrate --backend 1password --dry-run   # preview without writing
```

`--dry-run` reads the source secret, validates the target backend
parameters, and reports what would change, no writes. Migrating **to**
1Password requires `--vault` (and typically `--item`) unless the profile
already has 1Password metadata recorded.

=== "Success"

    ```json
    {
      "ok": true,
      "meta": { "command": "auth.migrate", "timestamp": "2026-05-27T00:00:00Z", "request_id": "…" },
      "data": {
        "dry_run": false,
        "target_backend": "1password",
        "profiles": [
          {
            "profile": "default",
            "source_backend": "keyring",
            "target_backend": "1password",
            "dry_run": false,
            "migrated": true
          }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

=== "No-op"

    ```json
    {
      "ok": true,
      "data": {
        "dry_run": false,
        "target_backend": "keyring",
        "profiles": [
          {
            "profile": "default",
            "source_backend": "keyring",
            "target_backend": "keyring",
            "dry_run": false,
            "migrated": false,
            "reason": "already using target backend"
          }
        ]
      }
    }
    ```

=== "Error"

    ```json
    {
      "ok": false,
      "meta": { "command": "auth.migrate", "exit_code": 1, "timestamp": "…", "request_id": "…" },
      "data": null,
      "errors": [
        {
          "type": "auth",
          "code": "auth_failed",
          "message": "auth migrate: read destination credential for profile \"default\": error resolving secret reference: the specified field cannot be found within the item",
          "hint": "",
          "retryable": false
        }
      ],
      "warnings": []
    }
    ```

A per-profile object carries `error` (and `migrated: false`) when the
precondition for that profile fails. A top-level error (like the one
above) wraps everything in the standard error envelope with
`ok: false` and `meta.exit_code`.

!!! warning "Migrating away from 1Password leaves the item behind"
    When migrating **from** 1Password to another backend, jira-cli
    removes the credential field from the 1Password item but **keeps
    the item itself**. The response surfaces this in two places:
    `data.cleanup_notes[]` and a `warnings[]` entry with
    `type: credential_notice`. Migrating **back to the same item name**
    will fail with `auth_failed` because the destination-precheck looks
    for a field that was removed, use a fresh `--item` value or delete
    the orphan item via the 1Password UI (the `op` CLI refuses to
    delete a Password-category item whose secret field has been
    cleared).

## auth refresh

Re-resolve the active profile's credential and trigger a backend-specific
refresh flow. Today, jira-cli only supports classic API tokens, so this is a
**no-op** that reports why nothing happened, it exists as a future hook for
OAuth-style flows.

```sh
jira auth refresh
```

=== "Human"

    ```text
    INF ℹ️ auth_type=token profile=default reason="selected auth type has no refresh flow" refreshed=false
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "auth.refresh", "timestamp": "2026-05-27T00:00:00Z", "request_id": "…" },
      "data": {
        "auth_type": "token",
        "profile": "default",
        "refreshed": false,
        "reason": "selected auth type has no refresh flow"
      },
      "errors": [],
      "warnings": []
    }
    ```

To re-validate a rotated API token, run `auth status` instead, it
re-probes `/myself` and `/mypermissions` with the current resolved
credential.

## Backends

| Backend | Use |
|---------|-----|
| `keyring` | Default. OS keyring (Keychain on macOS, libsecret on Linux). |
| `1password` | SDK-backed 1Password item storage. Requires the desktop app integration setting. |
| `JIRA_TOKEN_<PROFILE>` | Environment override; checked before stored credentials on every command. |

!!! info "1Password desktop-app integration"
    The 1Password backend uses the official Go SDK, which talks to the
    1Password desktop app. Sign in to the account that owns the item, then
    enable **Integrate with other apps** under Settings → Developer. For
    biometric approval, enable the OS unlock option under Settings →
    Security. Without the desktop app integration setting, the SDK-backed
    backend cannot read the item. Desktop-app authorization is per account and
    per process, expires after ten minutes of inactivity, and is revoked when
    the account locks. See the
    [1Password SDK desktop app integration](https://www.1password.dev/sdks#1password-desktop-app)
    docs.

!!! note "CGO requirement for 1Password"
    Release archives are built without CGO. To use the `1password` backend,
    build from a source checkout (see [Contributing › Building from
    source](contributing.md#building-from-source)), or use
    `JIRA_TOKEN_<PROFILE>` and skip the backend.

## Credential storage

Tokens never appear in `~/.config/jira-cli/config.toml`. The TOML stores a
backend reference; the secret lives outside it.

`keyring` on macOS

:   macOS Keychain. Service `jira-cli`, account `<host>/<profile>`. Inspect
    with **Keychain Access**.

`keyring` on Linux

:   libsecret / GNOME Keyring (or whichever secret-service implementation
    is running). Service `jira-cli`, account `<host>/<profile>`.

    ```sh
    secret-tool lookup service jira-cli account example.atlassian.net/default
    ```

`1password`

:   An item in the 1Password vault you specified at `auth login`. The SDK
    reads it via the desktop app integration, sign-in to the account that
    owns the item must be live, and **Integrate with other apps** must be
    enabled under Settings → Developer. Desktop-app authorization is per
    process, so separate `jira` invocations may prompt separately.

`JIRA_TOKEN_<PROFILE>`

:   Process environment. Nothing on disk. Checked before stored
    credentials on every command, see [Credential precedence](#credential-precedence).

!!! tip "Custom keyring service name"
    Override the keyring service name by setting `JIRA_KEYRING_SERVICE`
    before any jira invocation. Used by the test suite to keep CI off the
    real user keyring; useful in custom multi-tenant setups too.

## Credential precedence

When jira looks up a token for a profile, it tries sources in order:

1.  **`JIRA_TOKEN_<PROFILE>` environment variable**, the profile name is
    uppercased (profile `work` → `JIRA_TOKEN_WORK`). When set, this
    overrides the configured backend for that single invocation.
2.  **The backend configured on the profile**, the `keyring` or
    `1password` reference recorded by `auth login`.

If neither resolves, the call fails with `auth_failure` (exit 1) and the
`errors[0].type` field in the JSON envelope is `auth_failure`.

`jira auth status --output=json` reports which source resolved per profile,
useful as a quick health check from a script.

!!! warning "Never put the token in argv"
    Don't `jira auth login --token "$JIRA_TOKEN"`, process listings
    (`ps`, `/proc`) leak argv. The CLI doesn't accept `--token` as a flag
    for this reason. Use `--secret-stdin`, `--credential-env`, or
    `JIRA_TOKEN_<PROFILE>` instead.

## Troubleshooting access

Jira returns a small set of HTTP statuses that map to stable codes in the JSON
envelope. Branch CI logic on `errors[0].code` (or `meta.exit_code`), not on
the human message, codes are stable, messages may change.

=== "401"

    **Invalid token.** Stable signals in the JSON envelope:

    *   `errors[0].code = "jira_unauthorized"`
    *   `errors[0].type = "auth_failure"`
    *   `meta.exit_code = 1`

    The token is missing, revoked, mistyped, or for a different Atlassian
    site than the profile's `base_url`.

    **Next:**

    ```sh
    jira auth status --output=json     # confirm Jira-side rejection
    jira auth login                    # store a fresh token in the backend
    ```

=== "403"

    **Insufficient permission.** Stable signals in the JSON envelope:

    *   `errors[0].code = "jira_forbidden"`
    *   `errors[0].type = "auth_failure"`
    *   `meta.exit_code = 1`

    The token authenticates fine but lacks project or field-level permission for
    the requested action. Auth itself is healthy; only the *capability* is
    missing.

    **Next:**

    ```sh
    jira auth status --project KEY --output=json   # probe permissions for one project
    ```

    If `--project` reports the user is missing a permission, the resolution is
    on the Jira side (admin grants role, project permission changes, etc.), not
    in jira itself.

=== "429"

    **Rate-limited.** Stable signals in the JSON envelope:

    *   `errors[0].code = "jira_rate_limited"`
    *   `errors[0].type = "rate_limit"`
    *   `meta.exit_code = 4`

    Jira returned HTTP 429. Two extra fields on the error record drive
    the retry strategy:

    *   `errors[0].retry_after_seconds`, wait this long before retrying.
    *   `errors[0].rate_limit_remaining`, calls left in the current
        window (often `0` when 429 fires).

    **Next:** honour `retry_after_seconds` with exponential backoff in the
    caller. Don't tight-loop the same command on 429.

CLI-side validation errors stay separate from API errors:

| Error | Cause | Fix |
|---|---|---|
| `profile <name> is already authenticated; pass --force` | Profile already has a stored credential. | Pass `--force` to overwrite, or `auth logout` first. |
| `--secret-stdin and --credential-env are mutually exclusive` | Both input flags passed at once. | Pick one. |
| `1password backend requires CGO` | `auth_type = "1password"` with a non-CGO build. | Build from source, or switch to `keyring` / env override. |

!!! tip "Diagnosing a failure"
    The recipe is always the same: `jira auth token --output=json` to confirm
    which credential is being used and whether it's valid locally, then `jira
    auth status --output=json` to confirm Jira still accepts it. Together
    they distinguish "no token resolved", "stale token", and "Jira-side
    permission" failures.

## Token support

jira talks to **Jira Cloud only**; Server/Data Center are not supported. Today
the CLI supports classic Atlassian API tokens.

Scoped API tokens are not supported yet. They require routing REST API v3 calls
through Atlassian's gateway URL
(`https://api.atlassian.com/ex/jira/<cloudId>/...`) instead of the normal
`https://your-site.atlassian.net/...` REST base URL. Support for scoped tokens
is planned for a future auth update that changes how the CLI stores and
resolves Jira API base URLs.

## Further reading

*   [Installation](installation.md), get the binary first.
*   [Configuration › Where it lives](config.md#where-it-lives), where
  the config file and cache live on disk.
*   [`agent guide auth_setup`](agent.md#guides), the agent-facing
  runbook for the same flow.
