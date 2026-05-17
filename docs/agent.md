# Agent Tooling

The `agent` commands expose machine-readable CLI metadata and runbooks.

```sh
jira agent schema --output=json
jira agent schema --output=compact
jira agent guide
jira agent guide issues
jira agent adf-matrix --output=json
jira agent fieldtypes --output=json
```

## Schema

`agent schema` reports:

- command paths and subcommands
- global and local flags
- flag enums and completion predictors
- mutual-exclusion and required-together flag groups
- input schemas for JSON payloads
- output schemas for the envelope and selected commands

Use `--output=compact` when an agent wants the schema data without the JSON
envelope.

## Guide

`agent guide` prints the embedded Markdown guide. A section slug prints one
section:

```sh
jira agent guide output-modes-envelope
jira agent guide headless-writes-the-no-input-contract
```

Run `jira agent guide --help` for the current section list.

## Registries

`agent adf-matrix` describes ADF support. `agent fieldtypes` describes
customfield encoders. Both use the same envelope shape under
`--output=json`.
