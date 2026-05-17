# jira

Terminal-first Jira CLI for day-to-day developer workflows.

## Install

```sh
go install github.com/matcra587/jira-cli/cmd/jira@latest
```

Homebrew and GoReleaser release archives include release version metadata.
`go install github.com/matcra587/jira-cli/cmd/jira@latest` builds from source
and may report `dev` or git-derived metadata in `jira version`.
Release archives currently target macOS and Linux.

## Configuration

Configuration lives in `~/.config/jira-cli/config.toml`. Use metadata-only
profiles; credentials belong in the OS keychain, 1Password, or environment
fallbacks.

```sh
jira config init --no-input \
  --profile default \
  --base-url https://company.atlassian.net \
  --auth-type token \
  --email dev@example.com
```

## Auth

```sh
jira auth status
jira auth login
jira auth token
```

Use `token`, `basic`, `pat`, or `mtls` for `auth login`. Unsupported auth types
are rejected instead of being stored as fake authenticated profiles.

The 1Password backend uses the Go SDK when `onepassword_account` is configured
for desktop-app auth or `OP_SERVICE_ACCOUNT_TOKEN` is present for service-account
auth. macOS and Linux release archives are built without CGO, so use a
CGO-enabled source build for 1Password-backed profiles.

```toml
[[profiles]]
name = "work"
base_url = "https://company.atlassian.net"
auth_type = "token"
email = "dev@example.com"
secret_backend = "1password"
onepassword_account = "Team"
vault = "Engineering"
item = "jira-cli-work"
```

### Required scopes

Cloud API tokens and scoped API tokens need permission to call the Jira REST
endpoints the CLI uses. Jira Server/Data Center PATs inherit the user's
permissions and don't use scopes.

Classic Atlassian scopes (3 total — simplest path):

- `read:jira-user` — `jira me`, auth verification
- `read:jira-work` — issue view/list, JQL search, transitions list, worklog
  list, project/label/field/issuetype caches, createmeta
- `write:jira-work` — create/edit/delete issues, execute transitions,
  comments, issue links, web (remote) links, worklog add/delete

Granular scopes (Cloud scoped API tokens):

Read:

- `read:user:jira`, `read:application-role:jira`, `read:group:jira`,
  `read:avatar:jira` — all four are required by `/rest/api/3/myself` and
  enforced as a union; missing any one returns 401
- `read:attachment:jira` — `issue attachment list/download`
- `read:comment:jira` — `issue comment list` and read pair for
  `write:comment:jira`
- `read:field:jira` — field cache (customfield_NNNN map)
- `read:issue:jira` — issue view, list, JQL search
- `read:issue-link-type:jira` — `issue link types`, `cache linktypes`
- `read:issue-meta:jira` — createmeta
- `read:issue-type:jira` — issuetype cache
- `read:issue.transition:jira` — list available transitions
- `read:issue.watcher:jira` — `issue watchers list`
- `read:issue-worklog:jira` — worklog list
- `read:label:jira` — label cache
- `read:project:jira` — project cache, `boards list` per-board project lookup
- `read:project-role:jira` — comment visibility role context
- `read:board-scope:jira` — `boards list`, `cache boards`, `--board NAME` /
  `--board-id N` resolution on `issue list` and `jql build`

Write / delete:

- `delete:attachment:jira` — `issue attachment delete`
- `delete:comment:jira` — `issue comment delete`
- `delete:issue:jira` — issue delete (incl. `--delete-subtasks`)
- `delete:issue-link:jira` — `issue link delete`
- `delete:issue-worklog:jira` — `worklog delete`
- `delete:issue.watcher:jira` — `issue watchers remove`, `unwatch`
- `write:attachment:jira` — `issue attachment add`
- `write:comment:jira` — `issue comment add/edit`
- `write:issue:jira` — create, edit, transition execute
- `write:issue.remote-link:jira` — `issue weblink`
- `write:issue.watcher:jira` — `issue watchers add`, `watch`
- `write:issue-link:jira` — `issue link`
- `write:issue-worklog:jira` — `worklog add`

Atlassian's OpenAPI spec lists additional granular scopes per endpoint
that may be enforced when the corresponding request features are used.
The set above is the empirically minimal one for the surface this CLI
exercises. The CLI does not touch project-admin or global-configuration
endpoints, so `manage:jira-project` and `manage:jira-configuration`
are not needed.

## TUI

Run `jira -i` or `jira tui` in a terminal. The dashboard uses vim-style navigation
and keeps running until `q`.

Core keys: `j/k`, `/`, `Enter`, `e`, `m`, `c`, `w`, `n`, `r`, `P`, `?`, `q`.

## Headless JSON

Non-TTY and agent environments emit JSON without prompts.

```sh
jira issue list --json
jira issue create --json-input payload.json --no-input --dry-run --json
jira agent schema
```

Where `payload.json` is at minimum:

```json
{"summary": "Fix login", "project_key": "PROJ", "issue_type": "Task"}
```

`project_key` and `issue_type` are required under `--no-input`. See the
embedded agent guide (`jira agent guide`) for richer payload shapes
including ADF descriptions and customfields.

TTY commands render successful results through `clog` rich output:

```text
INF ℹ️ listed issues count=0 detail=false
```

Use `--compact` for jq-friendly output, `--plain` to force `clog` rich text,
and `--raw` for Jira REST-native shapes where supported.

## JQL

`jira issue list` defaults to recently-updated issues
(`updated >= -365d ORDER BY updated DESC`). Override with `--jql` or builder
flags:

```sh
jira issue list
jira issue list --jql 'project = PROJ AND status = "In Progress"'
jira jql build --project PROJ --assignee me --status "In Progress"
```

See [docs/jql.md](docs/jql.md) for supported builder flags and Atlassian JQL
references.

## Aliases

```sh
jira alias set mine -- issue list --assignee me
jira alias list
jira alias import aliases.yml
jira alias delete mine
```
