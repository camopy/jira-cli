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

*   An Atlassian Cloud site, such as `https://example.atlassian.net`.
*   The account email for that site.
*   An
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

    === "Nothing stored (env backend)"

        For hosts with no OS keyring — WSL, headless Linux, containers — or a
        per-process secret injector like `op run`. The profile's credential is
        the `JIRA_TOKEN_<PROFILE>` variable itself; login stores nothing and
        verifies the variable's token when it's set:

        ```sh
        export JIRA_TOKEN_DEFAULT="$(your-secret-source)"
        jira auth login --no-input \
          --profile-name default \
          --base-url https://example.atlassian.net \
          --email you@example.com \
          --backend env
        ```

    `--secret-stdin` and `--credential-env` both supply the credential, so
    they're mutually exclusive; pick whichever suits your shell. The env
    backend takes neither — the variable is the credential.

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
| `env` | Nothing is stored — the profile's `JIRA_TOKEN_<PROFILE>` variable is the credential, read every run. For containers, CI, `op run`-style injectors, or any host without a keyring (see [WSL and headless Linux](#wsl-and-headless-linux)). |
| `JIRA_TOKEN_<PROFILE>` | An environment override, checked before the stored backend on every command — whatever the backend. |

When jira resolves a token it checks `JIRA_TOKEN_<PROFILE>` first (profile `work`
becomes `JIRA_TOKEN_WORK`), then the backend recorded on the profile.

### WSL and headless Linux

Linux keyring support needs a Secret Service on the session D-Bus. Desktop
Linux ships one (gnome-keyring, KWallet); WSL and most headless hosts don't by
default, so `keyring` operations fail with `keyring_unavailable` until you add
one. You have two options.

**Recommended — add a Secret Service, then use `keyring` as normal.** On a
systemd-enabled WSL distro (Ubuntu, Debian) it takes about a minute, and the
keyring then behaves exactly as it does on macOS or Windows.

!!! info "Prerequisite: systemd enabled in WSL"
    The keyring needs a per-user D-Bus session bus, which WSL provides only
    when systemd is running. Check `/etc/wsl.conf` for:

    ```ini
    [boot]
    systemd=true
    ```

    If it's missing, add it, then restart WSL from Windows
    (`wsl --shutdown`) and reopen the distro. Confirm the session bus is live
    before continuing:

    ```sh
    echo "$DBUS_SESSION_BUS_ADDRESS"
    ```

    A value like `unix:path=/run/user/1000/bus` confirms the bus is live; a
    blank result means systemd isn't running — enable it first. (Native
    headless Linux already has a user D-Bus session under a normal login and
    can skip straight to the steps below.)

```sh
sudo apt install gnome-keyring libsecret-tools # (1)!

printf '' | gnome-keyring-daemon --unlock --components=secrets --daemonize # (2)!

jira auth login --backend keyring \
  --base-url https://<site>.atlassian.net --email you@example.com # (3)!
```

1.  Install a Secret Service provider — the keyring daemon plus `secret-tool`,
    handy for verifying.
2.  Create the keyring once. WSL has no graphical login to unlock it, so it is
    created **without a password**, which lets it unlock automatically from then
    on. This only writes the keyring file; the running daemon is started later,
    on demand.
3.  Store the token in the keyring. Omit `--base-url` / `--email` to reuse the
    profile's existing values.

From then on D-Bus starts and unlocks the keyring on demand — including after
every WSL restart — so there is no daemon to keep running and nothing to add to
your shell profile. Confirm it resolves with nothing in the environment:

```sh
jira auth status
```

The `Credential` line should read `keyring`.

!!! tip "If a keyring lookup fails from a script, cron job, or agent shell"
    Non-login and non-interactive shells don't always inherit
    `DBUS_SESSION_BUS_ADDRESS`, and without it the keyring is unreachable even
    though it works in your interactive terminal. Export it in the rc your tools
    load:

    === "Bash / Zsh"

        ```sh
        # ~/.bashrc or ~/.zshrc
        export DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$(id -u)/bus"
        ```

    === "Fish"

        ```fish
        # ~/.config/fish/config.fish
        set -gx DBUS_SESSION_BUS_ADDRESS "unix:path=/run/user/"(id -u)"/bus"
        ```

    And if step 3 prompts for a password or fails to unlock, a passworded
    `login.keyring` left by a previous desktop session may be shadowing the
    empty one — remove `~/.local/share/keyrings/login.keyring` and retry.

**Simpler — skip the keyring, use the `env` backend.** The profile reads
`JIRA_TOKEN_<PROFILE>` on every command and stores nothing:

```sh
export JIRA_TOKEN_DEFAULT="$(op read "op://<vault>/<item>/<field>")"
jira auth login --backend env
```

!!! warning "WSL + the Windows `op.exe` bridge: use `op read`, not `op run`"
    A common WSL setup aliases `op` to the Windows `op.exe` so 1Password's
    biometric/desktop-app auth works from Linux. Secret *reads* cross the
    bridge fine, but `op run` spawns its child process **on Windows** — with
    the Windows `PATH`, environment, and config — so
    `op run -- jira auth status` runs a Windows `jira` (or fails with
    `executable file not found in %PATH%`), never your WSL binary, and the
    injected variable never reaches it. Use `op read` into the variable, as
    shown above. `op run` as a wrapper works only with a **native Linux** `op`
    signed in inside WSL. (The `%PATH%` spelling in the error is the tell that
    the Windows binary handled the call.)

### 1Password

The `1password` backend keeps the token in a 1Password item and reads it through
the desktop app on demand. Two things to know before you choose it:

!!! note "1Password on macOS and Linux needs a CGO build"
    The Windows release binary includes the `1password` backend. The macOS and
    Linux release binaries are built without CGO, which the backend needs on
    those platforms, so either install with CGO enabled
    (`CGO_ENABLED=1 go install github.com/matcra587/jira-cli/cmd/jira@latest`)
    or set `JIRA_TOKEN_<PROFILE>` and skip the backend.

!!! note "1Password desktop-app authorization"
    The desktop app must be installed, signed in to the account that owns
    the item, and configured to allow SDK integrations: in 1Password open
    Settings › Developer and enable **Integrate with other apps**.
    Authorization is per account and per process — separate `jira`
    invocations may prompt separately even while the app is unlocked.

    Further reading:
    [1Password SDK desktop app integration](https://www.1password.dev/sdks#1password-desktop-app)

## When access fails

`auth status` tells a bad token apart from a missing permission:

*   **401** (`jira_unauthorized`): the token is missing, revoked, mistyped, or for
  a different site than the profile's base URL. Run `jira auth login` to store a
  fresh one.
*   **403** (`jira_forbidden`): the token authenticates fine but lacks permission
  for that project or field. That one is resolved on the Jira side.

```sh
jira auth status --output json     # machine-readable health check
jira auth logout <profile>         # remove a stored credential
```

Removing a credential is destructive — getting it back means re-entering the
token — so a script, agent, or `--no-input` invocation must pass `--force`.
An interactive terminal proceeds without a prompt: naming the profile is the
intent. `--dry-run` reports which credential a live logout would remove
without touching the secret backend:

```sh
jira auth logout work --dry-run --output json
jira auth logout work --force --output json
```

A stored credential outlives its profile: deleting the profile from config
does not remove the secret from the keychain, and a plain
`jira auth logout <profile>` refuses a name that is no longer in config. Pass
the profile's old site as `--base-url` — the keychain entry is keyed by site
host and profile name, and the pair identifies the orphaned credential
without the config entry:

```sh
jira auth logout old-work --base-url acme.atlassian.net
```

## See also

*   [Configure](config.md) for profiles,
  defaults, and the full config reference.
*   [Output and scripting](output.md) for the
  JSON envelope and exit codes.
