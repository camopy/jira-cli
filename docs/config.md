---
title: Configure
description: How jira-cli is configured — one TOML file plus a few JIRA_* environment variables. Profiles, defaults, secret backends, themes, and the full key reference.
icon: material/cog-outline
---

# :gear: Configure

jira-cli reads one TOML file and a small set of `JIRA_*` environment variables.
This page covers where that file lives, what overrides what, and the settings
worth knowing. Tokens are the one thing that never live here — they sit in your
keyring or 1Password; see [Authenticate](auth.md) for that.

For the exact flags on each `config` subcommand, see the command reference:
[`config init`](reference/jira/config/init.md),
[`config get`](reference/jira/config/get.md),
[`config set`](reference/jira/config/set.md),
[`config profile`](reference/jira/config/profile.md),
[`config theme`](reference/jira/config/theme.md).

## Where it lives

`--config <path>` overrides the location on any command, and `config init`
creates the file on first run.

=== "macOS"

    ```text
    ~/.config/jira-cli/config.toml
    ```

    `$XDG_CONFIG_HOME` is honoured when set to an absolute path.

=== "Linux"

    ```text
    ~/.config/jira-cli/config.toml
    ```

    `$XDG_CONFIG_HOME` is honoured when set to an absolute path.

=== "Windows"

    ```text
    %AppData%\jira-cli\config.toml
    ```

    For example `C:\Users\You\AppData\Roaming\jira-cli\config.toml`.
    `XDG_CONFIG_HOME` is honoured when set to an absolute path.

## What overrides what

When the same setting is set in more than one place, the highest wins:

1.  CLI flag — `--output=json`, `--config <path>`, `--profile <name>`.
2.  Environment variable — the `JIRA_*` set below.
3.  Per-profile entry under `[[profiles]]`.
4.  Top-level key in `config.toml`.
5.  Built-in default.

## Profiles

A profile is one Jira site. The `default` profile covers the common case; add
more when you point at several sites.

=== "One profile"

    `config init` writes the profile shell; `auth login` fills in the token.

    ```sh
    jira config init --base-url https://example.atlassian.net --email you@example.com
    ```

    `config init` needs both `--base-url` and `--email`, never prompts, and never
    writes a credential — run [`auth login`](auth.md) next. Add `--dry-run` to
    validate and preview the profile without writing the file (`config theme`
    takes `--dry-run` too).

=== "Several profiles"

    Each `[[profiles]]` block is one site; `name` is the key `--profile` and
    `JIRA_DEFAULT_PROFILE` look up. `default_profile` picks the active one.

    ```toml
    default_profile = "work"

    [[profiles]]
      name = "work"
      base_url = "https://example.atlassian.net"
      email = "you@example.com"
      auth_type = "token"
      secret_backend = "keyring"

    [[profiles]]
      name = "personal"
      base_url = "https://side-project.atlassian.net"
      email = "you@example.com"
      auth_type = "token"
      secret_backend = "1password"
      onepassword_account = "my.1password.com"
      vault = "Private"
      item = "jira-personal"
    ```

    Switch per command with `--profile personal`, or for a whole shell with
    `export JIRA_DEFAULT_PROFILE=personal`. `jira config profile` lists them and
    marks the active one.

## Defaults you set once

Set these on a profile and most commands stop needing the repetitive flags:

```sh
jira config set profiles.default.default_project ENG
jira config set profiles.default.default_issue_type Task
jira config set profiles.default.default_board "Engineering Sprint"
```

After this, `jira issue list` scopes to ENG, `jira issue create` defaults the
type to Task, and `--board` resolves from `default_board`. `config get` reads a
value back by its dotted key:

```sh
jira config get profiles.default.default_project --output=json
```

`config set --dry-run` runs the same key and value validation and reports the
current and new value without writing the file — useful for previewing a
scripted change:

```sh
jira config set theme.name dracula --dry-run --output=json
```

!!! note "Dotted keys only"
    `config get` / `set` address values by full dotted path
    (`profiles.default.default_project`), not bare names — the same name can
    exist at profile and top level. An unknown key exits 3
    (`validation_failed`); a `set` rewrites the file in place, preserving the
    formatting and comments around the touched key.

## Where the token lives

`secret_backend` on each profile decides where the API token is stored — never
the TOML. The token itself is covered in [Authenticate](auth.md); the backends:

| `secret_backend` | What it is |
|---|---|
| `keyring` | Default. OS keychain — Keychain (macOS), Credential Manager (Windows), libsecret (Linux). |
| `1password` | A 1Password item, addressed by `onepassword_account` + `vault` + `item`. |
| `env` | Nothing stored — the token is read from `JIRA_TOKEN_<PROFILE>` every run. For hosts without a keyring (WSL, headless Linux, containers) and per-process injectors like `op run`. |

Move a credential between the storing backends with [`auth migrate`](auth.md);
it never lands in the config file. The env backend isn't a migrate target —
it has no store; re-point a profile at it with `jira auth login --backend env`.

Any backend can be overridden for one run with `JIRA_TOKEN_<PROFILE>` (see
the [environment variables](#environment-variables) below) — it's checked before
the stored backend, whichever backend the profile uses.

## Themes

```sh
jira config theme --name catppuccin-mocha   # a bundled theme by name
jira config theme --path ~/my-theme.toml     # a custom TOML theme
```

Bundled names: `auto`, `dark`, `light`, `catppuccin-{frappe,latte,macchiato,mocha}`,
`dracula`, `gruvbox-{dark,light}`, `monochrome-{dark,light}`, `monokai`, `nord`,
`one-dark`, `plain-{dark,light}`, `synthwave`, `solarized-{dark,light}`,
`tokyo-night`. `JIRA_THEME=<name>` overrides the configured theme for one
process.

??? note "How `auto` detects the terminal"
    `auto` reads the terminal background and picks the matching light or dark
theme, so themed text (priority levels, help, markdown) stays readable on
either surface. Status pills and assignee names carry fixed colours chosen
to read on both light and dark, so they stay legible regardless of detection. Detection needs a real terminal: when output is piped, or
    `--color=never` / `NO_COLOR` is set, `auto` falls back to the dark theme and
    runs no detection. If your terminal is misdetected, pin `light` or `dark`.

## Behaviour toggles

??? info "Read-only mode"
    `read_only = true` on a profile refuses every mutation in that profile;
    `JIRA_READ_ONLY=1` enforces the same for the current process regardless of
    profile. Use it when pointing an exploratory shell at a shared or production
    tenant.

??? info "Editor for `issue edit`"
    The editor resolves in order: `JIRA_EDITOR`, the profile's `editor`, the
    top-level `editor`, then `$EDITOR`. Editors that fork and return (e.g. `code`
    without `--wait`) are refused at spawn — set `editor = "code --wait"` or use
    `nvim --wait`.

??? info "ADF strict mode"
    Mutations validate ADF strictly by default. `JIRA_ADF_STRICT=0` falls back to
    best-effort (warnings instead of exit 3). See [ADF](adf.md) for what's lossy.

??? info "Rate-limit retry"
    On a rate-limited **read** (HTTP 429, or a 503 with `Retry-After`), the CLI
    waits and resends; it honours `Retry-After`, otherwise backs off with jitter.
    Mutations are never auto-retried — a resent write could duplicate.
    `--max-retry-wait` (or `JIRA_MAX_RETRY_WAIT`, a Go duration like `45s`) caps a
    single wait; `0` disables retry. The budget is always capped by `--timeout`.

## Reference

### Per-profile keys

| Key | Purpose |
|---|---|
| `name` | Lookup key; matches `--profile` and `JIRA_DEFAULT_PROFILE` |
| `base_url` | Jira site URL (e.g. `https://example.atlassian.net`) |
| `email` | Atlassian account email; the auth username with API tokens |
| `auth_type` | `token` is the only value (covers classic and scoped API tokens) |
| `cloud_id` | Atlassian cloudId for a scoped token; normally set by `auth login`. Present = route via the gateway; empty = classic, site-addressed |
| `account_id` | Filled by `auth login`; enables `--assignee me` |
| `secret_backend` | `keyring` (default), `1password`, or `env` |
| `onepassword_account`, `vault`, `item` | 1Password addressing when `secret_backend = "1password"` |
| `default_project` | Used when `--project` is omitted on `issue list` / `create` |
| `default_issue_type` | Used when `--type` is omitted on `issue create` |
| `default_board` | Used when `--board` is omitted on `issue list` / `jql build` |
| `read_only` | When `true`, refuses every mutation in this profile |
| `editor` | Override `$EDITOR` for `issue edit`; falls back to top-level `editor` |
| `refresh_interval` | TUI refresh cadence (seconds) |
| `timeout` | HTTP request timeout (seconds) |
| `workday_seconds` | Length of a working day; used by `worklog` time math |

### Top-level keys

| Key | Purpose |
|---|---|
| `default_profile` | Active profile when `--profile` and `JIRA_DEFAULT_PROFILE` are unset |
| `queries_path` | Where [`search saved`](search.md) looks for `.jql` files |
| `editor` | Default editor for `issue edit` (per-profile `editor` wins) |
| `[theme]` | Output and TUI theme; see [Themes](#themes) |
| `[tui]` | Dashboard config: tabs, lenses, sections, preview, keybindings — see [TUI](tui.md) |
| `[aliases]` | Command aliases; manage via [`jira alias`](alias.md), not by hand |

### Environment variables

| Variable | Effect |
|---|---|
| `JIRA_DEFAULT_PROFILE` | Override the active profile |
| `JIRA_TOKEN_<PROFILE>` | Supply the API token inline (profile name uppercased); checked before the stored backend, for any backend |
| `JIRA_ADF_STRICT` | `1`/`true` forces strict ADF validation; `0` enables best-effort fallback |
| `JIRA_EDITOR` | Override the editor for `issue edit` |
| `JIRA_MAX_RETRY_WAIT` | Rate-limit retry budget (Go duration); `0` disables. `--max-retry-wait` wins |
| `JIRA_NO_COLOR` | Disable ANSI colour in Human output |
| `JIRA_NO_UPDATE_CHECK` | Any non-empty value disables the passive new-release check and its hint |
| `JIRA_READ_ONLY` | `1`/`true` refuses every mutation regardless of profile |
| `JIRA_THEME` | Override `[theme].name` for the current process |
| `JIRA_KEYRING_SERVICE` | Override the keyring service name. Test-only; leave unset in production |

## Sample config.toml

A complete file covering two profiles, a theme, aliases, and TUI defaults. Copy,
adapt, drop into `~/.config/jira-cli/config.toml`.

```toml
default_profile = "work"
queries_path    = "~/.config/jira-cli/queries"
editor          = "nvim --wait"

[[profiles]]
  name = "work"
  base_url = "https://example.atlassian.net"
  email = "you@example.com"
  auth_type = "token"
  secret_backend = "keyring"
  default_project = "ENG"
  default_issue_type = "Task"
  default_board = "Engineering Sprint"
  refresh_interval = 30
  timeout = 30
  workday_seconds = 28800
  read_only = false

[[profiles]]
  name = "personal"
  base_url = "https://side-project.atlassian.net"
  email = "you@example.com"
  auth_type = "token"
  secret_backend = "1password"
  onepassword_account = "my.1password.com"
  vault = "Private"
  item = "jira-personal"

[theme]
  name = "catppuccin-mocha"

[tui]
  refresh_interval = 30
  default_tab = "issues"
  default_lens = "Team"
  preview = "right"
  preview_size = 40

[[tui.lenses]]
  title = "Team"
  jql = "project = ENG AND statusCategory != Done ORDER BY updated DESC"

[[tui.sections]]
  title = "Needs review"
  jql = "status = 'In Review' ORDER BY updated DESC"

[tui.keys]
  transition = ["T"]

[aliases]
  todo = "issue list --status \"To Do\""
  mine = "issue list --assignee me"
```

## See also

*   [Authenticate](auth.md) — API tokens, keyring vs 1Password vs env, `auth login` and `auth migrate`.
*   [TUI](tui.md) — everything under `[tui]`: tabs, lenses, sections, preview, keybindings.
*   [Output and scripting](output.md) — `--output` and `JIRA_NO_COLOR` shape what every command emits.
*   [Cache](cache.md) — cached metadata lives under `~/.cache/jira-cli/<profile>-<hash>/`, scoped by profile, base URL, and config path.
