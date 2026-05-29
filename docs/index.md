# Overview

`jira` is a terminal-first Jira CLI for developers and agents. One binary,
machine-readable JSON envelopes, every command driveable from flags,
stdin, JSON input files, or environment variables. The binary is `jira`;
the repo and Go module are `jira-cli`.

## Quick start

```sh
brew install matcra587/tap/jira                    # 1. install
jira config init --base-url https://example.atlassian.net --email john.doe@example.com
jira auth login                                    # 2. authenticate (interactive on a TTY)
jira issue list --assignee me                      # 3. first read
```

Other install paths (one-line installer, `go install`, pre-built
tarballs) live in [Installation](installation.md). For the headless
flow (CI / scripted bootstrap), see [`auth login`](auth.md#auth-login).

## What it does

*   **Read and search** issues, comments, attachments, watchers, worklog. → [Issues](issues.md), [Search](search.md), [JQL](jql.md)
*   **Create, edit, transition, link** issues with rich-text ADF payloads and dry-run preview. → [Issues](issues.md)
*   **Cache** project, field, board, label, and link-type metadata so completion and validation don't pay a round trip. → [Cache](cache.md)
*   **Stream structured output** for scripts and CI. JSON envelope with `ok` / `meta` / `data` / `errors[]` / `warnings[]`, typed exit codes. → [Output](output.md)
*   **Author rich text** in Atlassian Document Format directly, with strict validation before submission. → [ADF](adf.md)
*   **Drive from an LLM agent.** Machine-readable command schema, per-workflow runbooks, live ADF coverage matrix. → [Agents](agent.md)

## Common commands

| I want to… | Command |
|---|---|
| Authenticate against a Jira tenant | [`jira auth login`](auth.md#auth-login) |
| Switch between configured profiles | [`jira auth switch`](auth.md#auth-switch) or `--profile/-P` |
| Find issues by filter flags | [`jira issue list`](issues.md#list) |
| Find issues with raw JQL | [`jira search jql`](search.md#search-jql) |
| Save and re-run a JQL query | [`jira search saved`](search.md#search-saved) |
| Build JQL without memorising operators | [`jira jql build`](jql.md#build) |
| Preview a mutation without contacting Jira | `--dry-run` on any mutating command |
| Apply a batch of edits from a file | [`--json-input <file>`](issues.md#edit) |
| Cache project / field / board metadata | [`jira cache <resource>`](cache.md) |
| Store credentials in 1Password | [`jira auth login --backend 1password`](auth.md#backends) |
| Get a machine-readable command tree | [`jira agent schema`](agent.md) |
| Get an LLM-readable runbook | [`jira agent guide <slug>`](agent.md) |
| Log time spent on an issue | [`jira worklog add`](worklog.md#add) |
| Browse epics or attach an issue to one | [`jira epic`](epic.md) |
| Shorten a long command into a name | [`jira alias set`](alias.md#set) |
| Identify the active profile and user | [`jira me`](auth.md#auth-whoami) |
| Correlate a CLI invocation with Jira logs | `--output=json`, then `meta.request_id` |

!!! note "`jira tui` is experimental"
    The persistent dashboard (`jira tui` / `jira -i`) ships with the
    binary but isn't actively developed. For day-to-day work the
    headless commands above are the supported surface.

## Output modes

`--output` (default `auto`) is the single shape control. On a TTY without
an agent it renders human-readable clog output; piped or under a
detected agent / CI environment (`CLAUDECODE`, `CURSOR_TERMINAL`,
`GITHUB_ACTIONS`, etc.) it switches to compact JSON. The full envelope
shape, every per-command example, and the exit-code taxonomy live on
[Output](output.md).

## Where to next

<div class="grid cards" markdown>

* :rocket: **Install**

    ---

    Platform-specific paths, the one-line installer, version pinning,
    and uninstall.

    [Read more →](installation.md)

* :key: **Authenticate**

    ---

    OS keyring and 1Password backends, the `JIRA_TOKEN_<PROFILE>`
    override, credential precedence, and CI patterns.

    [Read more →](auth.md)

* :gear: **Configure**

    ---

    Per-profile defaults, multi-profile setup, aliases, themes, env
    vars, and the full config.toml reference.

    [Read more →](config.md)

* :robot: **For agents**

    ---

    Machine-readable command schema, per-workflow runbooks, and the
    live ADF coverage matrix for structured tool integration.

    [Read more →](agent.md)

</div>
