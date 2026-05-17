# Cache

Cache commands prime local Jira metadata for completion, validation, and
agent workflows. Caches are scoped by config file, Jira site, and profile.

## Commands

```sh
jira cache projects --refresh --output=json
jira cache fields --refresh --output=json
jira cache issuetypes --refresh --output=json
jira cache labels --refresh --output=json
jira cache epics --refresh --output=json
jira cache linktypes --refresh --output=json
jira cache boards --refresh --output=json
```

Use `jira cache clear` to remove cached data for the active profile.

## Boards

```sh
jira boards list --output=json
jira boards list --refresh --output=json
jira boards list --unbounded --output=json
```

Board data powers `--board` completion and board-scoped JQL:

```sh
jira issue list --board "Engineering Sprint" --output=json
jira jql build --board "Engineering Sprint" --output=json
```

The default board can be stored on a profile:

```sh
jira config set profiles.default.default_board "Engineering Sprint"
```

When no board flag is provided, `issue list` and `jql build` use the configured
default board if one exists.
