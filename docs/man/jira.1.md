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
- `jira issue list|view|create|edit|transition|clone|move|delete|comment|attachment|link|weblink|watchers|watch|unwatch|mine`
- `jira epic list|board|add|remove`
- `jira search jql|saved`
- `jira jql build`
- `jira alias set|list|delete|import`
- `jira worklog add|list`
- `jira auth login|status|logout|switch|refresh|token|migrate|whoami`
- `jira config init|profile|get|set|theme`
- `jira cache boards|labels|projects|epics|fields|issuetypes|linktypes|clear`
- `jira agent guide|schema|adf-matrix|fieldtypes`
- `jira me`
- `jira version`

## Output

`--output=auto|human|json|compact` is the single output selector. `human`
forces `clog` rich text, `--output=json` writes the full envelope, and
`compact` writes the command data on success. Errors are written as `clog`
diagnostics to stderr; under `json` or `compact` a JSON error envelope is also
written to stderr while stdout stays empty.
