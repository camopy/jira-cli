# Overview

`jira` is a terminal-first Jira CLI for day-to-day developer workflows. It has
an interactive dashboard, scriptable JSON output, dry-run paths for mutations,
and embedded agent discovery commands.

## Install

```sh
brew install matcra587/tap/jira
```

Also available through `go install` and GitHub release archives. See
[Installation](installation.md) for version metadata and source-build notes.

## First-time setup

```sh
jira config init --no-input \
  --profile default \
  --base-url https://company.atlassian.net \
  --auth-type token \
  --email dev@example.com

jira auth login
jira auth status
```

Credentials are stored outside the TOML config, either in the OS keyring,
1Password, or an environment override.

## Commands

| Command | Summary |
|---------|---------|
| [`auth`](auth.md) | Configure profiles and credential backends. |
| [`issue`](issues.md) | View, list, create, edit, transition, link, and comment on issues. |
| [`search`](search.md) | Run raw or saved JQL searches. |
| [`jql build`](search.md#jql-builder) | Build JQL from flags. |
| [`boards`](cache.md#boards) | List Jira agile boards visible to the active profile. |
| [`cache`](cache.md) | Prime fields, projects, boards, labels, epics, and link types. |
| [`agent`](agent.md) | Emit command schema, guide, ADF matrix, and field-type registry. |
| `tui` | Launch the persistent dashboard. |
| `version` | Print build metadata. |

## Output

The global output selector is `--output=auto|human|json|compact`.

```sh
jira issue list --output=json
jira issue list --output=compact
jira issue list --output=human
```

`auto` renders human output on a TTY, JSON on non-TTY stdout, and compact JSON
when a supported agent environment is detected.

## Themes

```sh
jira config theme --name catppuccin-mocha
jira config theme --path ~/my-theme.toml
```

Bundled names: `default`, `plain`, `catppuccin-{frappe,latte,macchiato,mocha}`,
`dracula`, `gruvbox-{dark,light}`, `monochrome`, `monokai`, `nord`,
`one-dark`, `synthwave`, `solarized`, `tokyo-night`.

The bundled palettes are defined in
[clib's theme presets](https://github.com/gechr/clib/blob/v0.4.6/theme/presets.go);
the names exposed by `jira config theme --name` are mirrored in
[jira's config enum](https://github.com/matcra587/jira-cli/blob/main/internal/config/theme.go).
Override per process with `JIRA_THEME=<name>`.

## Further Reading

- [Auth](auth.md)
- [Issues](issues.md)
- [ADF](adf.md)
- [Custom fields](custom-fields.md)
- [Agent tooling](agent.md)
