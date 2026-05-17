# Search

Use `issue list` for the common issue-list workflow, `search jql` for raw JQL,
and `search saved` for queries stored on disk.

## Issue List

```sh
jira issue list --output=json
jira issue list --project PROJ --assignee me --status "In Progress"
jira issue list --as-jql --project PROJ --assignee me --output=json
```

With no filters, `issue list` uses:

```jql
updated >= -365d ORDER BY updated DESC
```

## Raw JQL

```sh
jira search jql 'project = PROJ AND status = "In Progress"' --output=json
jira search jql 'project = PROJ' --fields key,summary,customfield_10010
jira search jql 'project = PROJ' --full
```

`--fields` and `--full` are mutually exclusive.

## JQL Builder

```sh
jira jql build --project PROJ --assignee me --status "In Progress"
jira jql build --project PROJ --desc=false
```

The builder defaults to `ORDER BY updated DESC`. Use `--desc=false` for
ascending order.

## Saved Queries

Saved queries live under `~/.config/jira-cli/queries/<name>.jql`:

```text
---
name: my-open-bugs
description: Bugs assigned to me, not done
---
project = PROJ AND assignee = currentUser() AND statusCategory != Done
ORDER BY priority DESC, updated DESC
```

Run with:

```sh
jira search saved my-open-bugs --output=json
```
