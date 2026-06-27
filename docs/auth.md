---
title: Authenticate
description: Log in to Jira Cloud with your Atlassian email and an API token. jira-cli verifies it once, then stores it in your OS keyring, never in the config file.
icon: material/key-outline
---

# :key: Authenticate

jira-cli signs in to Jira Cloud with your Atlassian account email and an API
token. It verifies the token once, stores it in a credential backend (your OS
keyring by default, or 1Password), and records only a reference in the config
file. The token itself never lands on disk in plain text. See
[Where the token is stored](#where-the-token-is-stored) for the backends and how
jira picks one.

## Before you start

!!! info inline end "Jira Cloud only"
    jira-cli supports Atlassian (Jira) Cloud. Jira Server and Data Centre aren't
    supported.

Have these ready:

- An Atlassian Cloud site, such as `https://example.atlassian.net`.
- The account email for that site.
- An
  [API token](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/)
  for that account. Both scoped and classic tokens work.

A profile name is optional. jira uses `default` unless you set one, and you can
add more profiles later to point at different sites.

## Log in

`auth login` creates or updates a profile, checks the token against Jira, and
stores it in your chosen backend. It writes only a reference to the credential
into the config file, never the token itself. You can run it two ways: an
interactive form when you're sat at a terminal, or a fully flag-driven call for
headless machines and scripts. Both end with the same stored profile.

!!! info "Scoped or classic, jira works it out"
    You don't configure the token type. `auth login` tries the token against your
    site first. If the site rejects it, jira looks up the site's cloud ID and
    retries through Atlassian's gateway; when that succeeds, the token is scoped
    and jira stores the cloud ID on the profile so later calls route the right
    way. Either way, the login command is the same.

=== "Interactive"

    The simplest way when you're setting jira up by hand. Create the profile,
    then log in:

    ```sh
    jira config init --base-url https://example.atlassian.net --email you@example.com
    jira auth login
    ```

    `auth login` opens a short form. Confirm the profile name, site, and email,
    choose where to store the token (your OS keyring by default), then paste the
    token when prompted. jira verifies it against Jira there and then, and prints
    who you've signed in as so you know the credential is good before you go any
    further.

=== "Non-interactive"

    For headless machines and scripts, where there's no prompt to answer. Pass
    each field as a flag and turn off prompting with `--no-input`. Don't put the
    token on the command line: argv leaks through `ps` and `/proc`. Instead feed
    it in through a named environment variable or stdin.

    === "Token via env var"

        ```sh
        JIRA_TOKEN="$(your-secret-source)" jira auth login --no-input \
          --profile-name default \
          --base-url https://example.atlassian.net \
          --email you@example.com \
          --backend keyring \
          --credential-env JIRA_TOKEN
        ```

    === "Token via stdin"

        ```sh
        your-secret-source | jira auth login --no-input \
          --profile-name default \
          --base-url https://example.atlassian.net \
          --email you@example.com \
          --backend keyring \
          --secret-stdin
        ```

    `--secret-stdin` and `--credential-env` both supply the credential, so
    they're mutually exclusive; pick whichever suits your shell.

!!! warning "Never put the token in argv"
    There's no `--token` flag, on purpose: process listings (`ps`, `/proc`) leak
    argv. Deliver the secret with `--secret-stdin`, `--credential-env`, or the
    `JIRA_TOKEN_<PROFILE>` override instead.

## Verify

Confirm the credential works:

```sh
jira auth status
```

`auth status` probes Jira and reports the resolved identity and the permissions
granted for each profile, so you can see at a glance whether a profile is signed
in and what it can do.

## Where the token is stored

Tokens live outside the config file. The TOML records only a backend reference;
the secret sits in the backend you chose.

| Backend | What it is |
|---|---|
| `keyring` | Default. The OS keyring: Keychain on macOS, Credential Manager on Windows, libsecret on Linux. |
| `1password` | A 1Password item, read through the desktop app. |
| `JIRA_TOKEN_<PROFILE>` | An environment override, checked before the stored backend on every command. |

When jira resolves a token it checks `JIRA_TOKEN_<PROFILE>` first (profile `work`
becomes `JIRA_TOKEN_WORK`), then the backend recorded on the profile.

!!! note "1Password on macOS and Linux needs a CGO build"
    The Windows release binary includes the `1password` backend. The macOS and
    Linux release binaries are built without CGO, which the backend needs on
    those platforms, so there either
    [build from source](contributing.md) or
    set `JIRA_TOKEN_<PROFILE>` and skip the backend.

## When access fails

`auth status` tells a bad token apart from a missing permission:

- **401** (`jira_unauthorized`): the token is missing, revoked, mistyped, or for
  a different site than the profile's base URL. Run `jira auth login` to store a
  fresh one.
- **403** (`jira_forbidden`): the token authenticates fine but lacks permission
  for that project or field. That one is resolved on the Jira side.

```sh
jira auth status --output json     # machine-readable health check
jira auth logout <profile>         # remove a stored credential
```

## See also

- [Configure](config.md) for profiles,
  defaults, and the full config reference.
- [Output and scripting](output.md) for the
  JSON envelope and exit codes.
