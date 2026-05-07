# jira(1)

## Name

jira - TUI-first, agent-ready Jira CLI

## Synopsis

`jira [flags] [command]`

## Description

`jira -i` or `jira tui` launches a persistent dashboard when stdout is a
terminal. In non-TTY or recognized agent environments ordinary commands emit
structured JSON.

## Commands

- `jira tui`
- `jira issue list|view|create|edit|transition|clone|move|delete|comment|link|weblink|mine`
- `jira epic list|board|add|remove`
- `jira search jql|saved`
- `jira jql build`
- `jira alias set|list|delete|import`
- `jira worklog add|list`
- `jira auth login|status|logout|switch|refresh|token|migrate|whoami`
- `jira config init|profile|get|set|theme`
- `jira cache labels|projects|epics|fields|issuetypes|clear`
- `jira agent guide|schema|adf-matrix|fieldtypes`
- `jira me`
- `jira schema`
- `jira version`

## Output

`--json`, `--compact`, `--plain`, and `--raw` are mutually exclusive; combining
any two returns exit 3. `--plain` forces `clog` rich text output. Errors are
written as `clog` diagnostics to stderr; under `--json` or `--compact` an
error envelope is also written to stdout so machine consumers can parse the
failure.
