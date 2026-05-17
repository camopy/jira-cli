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

## Further Reading

- [Auth](auth.md)
- [Issues](issues.md)
- [ADF](adf.md)
- [Custom fields](custom-fields.md)
- [Agent tooling](agent.md)
