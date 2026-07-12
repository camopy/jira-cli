---
description: >
  Layered configuration: the koanf loader, precedence, the profile model
  and secret backends, JIRA_* env vars, theme resolution, and the rules for
  changing the config shape.
paths:
  - "internal/config/**/*.go"
  - "internal/cli/config/**/*.go"
  - "internal/cli/auth/**/*.go"
---

# Config

Layered configuration using koanf v2, implemented in
`internal/config/loader.go`. Profiles are metadata-only; credentials live in
secret backends (see [security.md](security.md)).

## Precedence (last wins)

`defaults (confmap) → TOML file → JIRA_* env overlay`

*   `Load` never writes. `LoadOrInit` bootstraps a missing file and
    **skips the env overlay** — deliberately, so transient env values can
    never be persisted into the config file.
*   Decode is **strict** (`ErrorUnused: true`): unknown keys are rejected,
    not ignored. A key rename without migration breaks existing files —
    see "Changing the config shape".

## File

`~/.config/jira-cli/config.toml` (`%AppData%\jira-cli\config.toml` on
Windows), resolved through `x/shell.ConfigDir()` — never hardcode the path.
All writes are atomic and symlink-following via `x/os.AtomicWrite`
(loader.go).

## Profiles and secret backends

`[[profiles]]` entries carry metadata only: `name`, `base_url`, `auth_type`
(`token` is the only supported value — unsupported types are rejected, never
stored as fake-authenticated), `email`, `secret_backend`
(`keyring` | `1password` | `env`), and for 1Password:
`onepassword_account`, `vault`, `item`. Scoped tokens add a stored
`cloud_id` for gateway routing (see `internal/jira`).

*   The 1Password backend is CGO-gated with `_nocgo` stubs (see
    [go.md](go.md)) — release archives don't ship it.
*   The `env` backend stores nothing: the profile's `JIRA_TOKEN_<PROFILE>`
    variable is the credential, read at run time. It is never a migrate
    destination — re-point a profile with `auth login --backend env`.
*   `auth migrate` moves credentials between storing backends; never write
    credential material into the TOML file, envelopes, or logs.

## Env vars (prefix `JIRA_`)

| Var | Effect |
|-----|--------|
| `JIRA_DEFAULT_PROFILE` | selects the active profile |
| `JIRA_PROFILE_<NAME>_<FIELD>` | per-profile field override |
| `JIRA_THEME` (+ `_LIGHT`/`_DARK` via clib's `SetEnvPrefix("JIRA")`) | theme override for help, plain output, and the TUI |
| `JIRA_READ_ONLY` | truthy set (`1/true/yes/on`) blocks mutations; wins on the OFF→ON direction only |
| `JIRA_ADF_STRICT` | ADF mode override (flag > env > path default) |
| `JIRA_MAX_RETRY_WAIT` | rate-limit retry budget (Go duration); unparseable → default, never silently disabled |
| `JIRA_LIVETEST_PROJECT` | live-suite target project (a dedicated probe project) |
| `JIRA_NO_UPDATE_CHECK` | non-empty disables the passive update check (handled inside clive/notify; the name derives from the binary name) |
| `NO_COLOR` | presence disables color and hyperlinks (per no-color.org, unprefixed); `--color=always` overrides. There is no JIRA_NO_COLOR — the JIRA env prefix covers clog/clib theme/level/hyperlink vars, not this |

Agent detection (`CLAUDECODE`, `AI_AGENT`, …) lives in
`internal/cli/detector.go` and selects compact output — it is not config.

## Theme resolution (`internal/config/theme.go`)

*   Selectable names = `"auto"` + clib's presets (`clibtheme.Names()`), no
    jira-cli aliasing. `JIRA_THEME` wins over the config key on every path.
*   `"auto"` (background detection) is **opt-in, never the default** —
    detection misfires silently under tmux/SSH/container exec, and a wrong
    guess is worse than the fixed dark palette. Keep that comment-documented
    rationale intact if you touch it.
*   Validation is write-time only (`config theme --name`); config **load**
    deliberately does not validate, so a theme clib later renames degrades
    to the dark fallback at render instead of blocking every command.

## Changing the config shape

*   A purely additive key (new, optional, defaulted) lands with: the loader
    default, the TOML docs, and the schema surface together — one PR.
*   A breaking change (rename, retype, remove) must keep strict decode from
    rejecting existing files: provide an upgrade path (migration or
    load-time tolerance) and tests proving an old file still loads. There
    is no schema-version/migration framework — a breaking change ships its
    own upgrade path.
*   Editor resolution precedence is profile `editor`, then global `editor`
    (`cmdutil.ConfiguredEditorFor`) — keep new per-profile settings on the
    same pattern.

## Gotchas

*   Reading config in command paths goes through `cmdutil` helpers
    (`ConfigPath(cmd)`, `ActiveProfile`, `ReadOnlyEnabled`) — don't call
    `config.Load` with a hand-built path from command code.
*   `config init` requires `--base-url` and `--email` under `--no-input`;
    it exits before writing on a partial spec — preserve that (no
    half-written profiles).
