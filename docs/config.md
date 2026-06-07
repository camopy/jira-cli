# Configuration

`jira-cli` reads one TOML file and a small set of environment variables.
This page is the reference for what's in that file, what overrides it,
and what each setting does. Skim [Basics](#basics), reach for
[Advanced](#advanced) when you outgrow a single profile, and jump to
[Reference](#reference) or [Sample](#sample) when you need exhaustive
detail.

## Where it lives

```text
~/.config/jira-cli/config.toml
```

Override with `--config <path>` on any command. The directory is created
on first run by [`config init`](#init). Tokens never live in the config
file: they sit in the OS keyring, 1Password, or an environment variable,
addressed by the per-profile `secret_backend` setting (see
[Auth](auth.md) for the full secret-storage story).

## Precedence

Highest first:

1.  CLI flag (e.g. `--output=json`, `--config <path>`).
2.  Environment variable (`JIRA_*`).
3.  Per-profile entry under `[[profiles]]`.
4.  Top-level key in `config.toml`.
5.  Hardcoded default.

## Basics

The 80% case: one Jira site, one profile, defaults you set once and
forget.

### init

Create or rewrite the config file with a `default` profile shell.
Non-interactive: pass `--base-url` and `--email` as flags. The command
does not prompt and does not write credentials — run
[`auth login`](auth.md) next to populate the keyring.

```sh
jira config init --base-url https://example.atlassian.net --email john.doe@example.com
```

`config init` requires both `--base-url` and `--email`; either flag missing
exits with `required_flag_missing` (exit 3) before any write, so a stray
invocation can't silently blank an existing profile. For the interactive
walkthrough that prompts for site, email, and credential, use
[`auth login`](auth.md) instead.

=== "Human"

    ```text
    INF ℹ️ auth_type=token base_url=https://example.atlassian.net profile=default stored_auth=false
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "config.init", "timestamp": "…", "request_id": "…" },
      "data": {
        "auth_type": "token",
        "base_url": "https://example.atlassian.net",
        "profile": "default",
        "stored_auth": false
      },
      "errors": [],
      "warnings": []
    }
    ```

`stored_auth: false` reflects that `config init` never writes credentials;
run [`auth login`](auth.md) next.

### Default profile

The `default_profile` top-level key picks which `[[profiles]]` block is
active when `--profile` and `JIRA_DEFAULT_PROFILE` are both unset:

```toml
default_profile = "work"
```

### Common per-profile keys

Set these once and most commands stop needing repetitive flags:

```sh
jira config set profiles.default.default_project ENG
jira config set profiles.default.default_issue_type Task
jira config set profiles.default.default_board "Engineering Sprint"
```

After this, `jira issue list` scopes to ENG, `jira issue create` picks
Task as the type, and `--board` resolves from `default_board`. See the
[Per-profile keys](#per-profile-keys) reference for the full set.

## Advanced

Reach for these once one profile isn't enough or you want to bend
default behaviour.

### Multiple profiles

Each `[[profiles]]` block describes one Jira site. The `name` is the
lookup key used by `--profile` and `JIRA_DEFAULT_PROFILE`:

```toml
default_profile = "work"

[[profiles]]
  name = "work"
  base_url = "https://example.atlassian.net"
  email = "john.doe@example.com"
  auth_type = "token"
  secret_backend = "keyring"

[[profiles]]
  name = "personal"
  base_url = "https://side-project.atlassian.net"
  email = "john.doe@example.com"
  auth_type = "token"
  secret_backend = "onepassword"
  onepassword_account = "my.1password.com"
  vault = "Private"
  item = "jira-personal"
```

Switch per command with `--profile <name>` or globally for a shell with
`export JIRA_DEFAULT_PROFILE=personal`. Inspect what's active:

=== "Human"

    ```text
    INF ℹ️ active_profile=work profiles="[2 items]"
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "config.profile", "timestamp": "…", "request_id": "…" },
      "data": {
        "active_profile": "work",
        "profiles": [
          { "name": "work", "active": true },
          { "name": "personal", "active": false }
        ]
      },
      "errors": [],
      "warnings": []
    }
    ```

### Secret backends

`secret_backend` on each profile picks where the API token lives:

*   `keyring` (default): OS keychain via `libsecret` / Keychain / Credential Manager.
*   `onepassword`: looked up by `onepassword_account` + `vault` + `item`.
*   `env`: read from `JIRA_TOKEN_<PROFILE>` (uppercase, `-` → `_`).

Migrate between backends with [`auth migrate`](auth.md); the secret
never lands in the TOML.

### Aliases

User-defined command aliases live under the `[aliases]` table. The
value is the full command line minus the leading `jira`:

```toml
[aliases]
  todo = "issue list --status \"To Do\""
  mine = "issue list --assignee me"
```

`jira todo` runs `jira issue list --status "To Do"`. Manage the table
through the [`jira alias`](alias.md) commands rather than editing the
TOML by hand — `alias set`, `alias delete`, and `alias import` keep
the file shape stable.

### Themes

```sh
jira config theme --name catppuccin-mocha
jira config theme --path ~/my-theme.toml
```

Bundled names: `auto`, `dark`, `light`, `catppuccin-{frappe,latte,macchiato,mocha}`,
`dracula`, `gruvbox-{dark,light}`, `monochrome-{dark,light}`, `monokai`,
`nord`, `one-dark`, `plain-{dark,light}`, `synthwave`,
`solarized-{dark,light}`, `tokyo-night` — the set clib provides. Setting an
unknown name is rejected; a name already in a config that clib no longer knows
(e.g. a pre-v0.5 `default`) is tolerated on load and falls back to `dark`.

`auto` detects the terminal background and picks the matching light or dark
theme, so hash-coloured fields (status, priority, assignee) stay readable on
either surface — a fix for light terminals where the dark palette's bright
colours wash out. Detection needs a real terminal: when output is piped or
`--color=never`/`NO_COLOR` is set, `auto` falls back to the dark theme and
runs no detection. If your terminal is misdetected, pin `light` or `dark`
explicitly. `auto` applies to both one-shot output and the dashboard.

Override per-process with `JIRA_THEME=<name>`, which wins over the configured
theme (including `auto`).

=== "Human"

    ```text
    INF ℹ️ changed=true name=catppuccin-mocha path=
    ```

    `jira config theme` with no flags reports the current state with
    `changed=false`; passing `--name` or `--path` updates the config
    and flips the flag.

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "config.theme", "timestamp": "…", "request_id": "…" },
      "data": {
        "changed": true,
        "name": "catppuccin-mocha",
        "path": ""
      },
      "errors": [],
      "warnings": []
    }
    ```

### Read-only mode

`read_only = true` on a profile refuses every mutation in that profile.
`JIRA_READ_ONLY=1` enforces the same in the current process regardless
of profile setting. Use it when pointing at a shared / production
tenant from an exploratory shell.

### Editor

The editor for `issue edit` resolves in this order:

1.  `JIRA_EDITOR` (process)
2.  `editor` on the active profile
3.  `editor` at the top level of `config.toml`
4.  `$EDITOR`

Editors that fork-and-return (e.g. `code` without `--wait`) are refused
at spawn time. Set `editor = "code --wait"` or use `nvim --wait`.

### ADF strict mode

Mutations validate ADF strictly by default. Set `JIRA_ADF_STRICT=0` to
fall back to best-effort (degrades silently with warnings instead of
exit 3). See [ADF](adf.md) for what's lossy.

## Reference

### Per-profile keys

| Key | Purpose |
|-----|---------|
| `name` | Lookup key; matches `--profile` and `JIRA_DEFAULT_PROFILE` |
| `base_url` | Jira site URL (e.g. `https://example.atlassian.net`) |
| `email` | Atlassian account email; used as the auth username with API tokens |
| `auth_type` | `token` is the only supported value today |
| `account_id` | Filled in by [`auth login`](auth.md); enables `--assignee me` |
| `secret_backend` | `keyring`, `onepassword`, or `env` |
| `onepassword_account`, `vault`, `item` | 1Password addressing when `secret_backend = "onepassword"` |
| `default_project` | Used when `--project` is omitted on `issue list` / `create` |
| `default_issue_type` | Used when `--type` is omitted on `issue create` |
| `default_board` | Used when `--board` is omitted on `issue list` / `jql build` |
| `read_only` | When `true`, the CLI refuses every mutation in this profile |
| `editor` | Override `$EDITOR` for `issue edit`; falls back to top-level `editor` |
| `refresh_interval` | TUI refresh cadence (seconds) |
| `timeout` | HTTP request timeout (seconds) |
| `workday_seconds` | Length of a working day; used by `worklog` time math |

### Top-level keys

| Key | Purpose |
|-----|---------|
| `default_profile` | Profile to use when `--profile` and `JIRA_DEFAULT_PROFILE` are both unset |
| `queries_path` | Where [`search saved`](search.md#search-saved) looks for `.jql` files |
| `editor` | Default editor for `issue edit` (per-profile `editor` wins) |
| `[theme]` | Output and TUI theme; see [`config theme`](#themes) |
| `[tui]` | TUI defaults (`refresh_interval`, `default_tab`, `tabs`) — TUI is experimental |
| `[aliases]` | User-defined command aliases; managed via [`jira alias`](alias.md) |

### Environment variables

| Variable | Effect |
|----------|--------|
| `JIRA_DEFAULT_PROFILE` | Override the active profile |
| `JIRA_TOKEN_<PROFILE>` | Supply the API token inline. Highest priority for `secret_backend = "env"` |
| `JIRA_ADF_STRICT` | `1`/`true` forces strict ADF validation; `0` enables best-effort fallback |
| `JIRA_EDITOR` | Override the editor for `issue edit` |
| `JIRA_KEYRING_SERVICE` | Override the keyring service name. Test-only; leave unset in production |
| `JIRA_NO_COLOR` | Disable ANSI colour in Human output |
| `JIRA_READ_ONLY` | `1`/`true` refuses every mutation regardless of profile setting |
| `JIRA_THEME` | Override `[theme].name` for the current process |

### get

Read a single value by **dotted key**. Bare names are rejected because
the same name can exist at multiple levels (profile vs. top-level).

```sh
jira config get profiles.default.default_project --output=json
jira config get default_profile --output=json
```

=== "Human"

    ```text
    INF ℹ️ key=profiles.default.default_project
    ```

    Human output prints only the key. The value lives in the JSON
    envelope's `data.value` field; pass `--output=json` when scripting.

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "config.get", "timestamp": "…", "request_id": "…" },
      "data": { "key": "profiles.default.default_project", "value": "" },
      "errors": [],
      "warnings": []
    }
    ```

Unknown keys exit 3 with `validation_failed`:

```json
{
  "ok": false,
  "meta": { "command": "config.get", "exit_code": 3, "timestamp": "…", "request_id": "…" },
  "data": null,
  "errors": [
    {
      "type": "validation",
      "code": "validation_failed",
      "message": "unknown config key \"base_url\"",
      "hint": "",
      "retryable": false
    }
  ],
  "warnings": []
}
```

### set

Write a single value by dotted key. The file is rewritten in place;
TOML formatting and comments outside the touched key are preserved.

```sh
jira config set profiles.default.default_project <PROJECT_KEY>
jira config set profiles.default.default_board "Example board"
jira config set default_profile personal
```

=== "Human"

    ```text
    INF ℹ️ key=profiles.default.default_project
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "config.set", "timestamp": "…", "request_id": "…" },
      "data": {
        "key": "profiles.default.default_project",
        "value": "<PROJECT_KEY>"
      },
      "errors": [],
      "warnings": []
    }
    ```

## Sample

A complete `config.toml` covering two profiles, a theme, aliases, and
TUI defaults. Copy, adapt, drop into `~/.config/jira-cli/config.toml`.

```toml
default_profile = "work"
queries_path    = "~/.config/jira-cli/queries"
editor          = "nvim --wait"

[[profiles]]
  name = "work"
  base_url = "https://example.atlassian.net"
  email = "john.doe@example.com"
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
  email = "john.doe@example.com"
  auth_type = "token"
  secret_backend = "onepassword"
  onepassword_account = "my.1password.com"
  vault = "Private"
  item = "jira-personal"

[theme]
  name = "catppuccin-mocha"

[tui]
  refresh_interval = 30
  default_tab = "issues"
  tabs = ["issues", "epics", "search", "activity"]

[aliases]
  todo = "issue list --status \"To Do\""
  mine = "issue list --assignee me"
```

## See also

*   [Auth](auth.md): API tokens, keyring vs 1Password vs env, `auth login` and `auth migrate`.
*   [`search saved`](search.md#search-saved): `queries_path` points here.
*   [Cache](cache.md): cached metadata lives under `~/.cache/jira-cli/<profile>-<hash>/`, scoped by profile + base URL + config path.
*   [Output](output.md): `--output` and `JIRA_NO_COLOR` shape what every command emits.
