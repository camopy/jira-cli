# Issues

Issue commands cover read paths, dry-run writes, comments, attachments,
watchers, issue links, web links, transitions, clone, move, and delete.

## Read

```sh
jira issue view PROJ-1 --output=json
jira issue list --output=json
jira issue list --jql 'project = PROJ ORDER BY updated DESC' --output=json
jira issue list --as-jql --output=json
```

`issue list --detail` fetches full issue records. Without it, the list shape is
the summary set: key, summary, status, assignee, priority, and updated.

## Create And Edit

```sh
jira issue create --no-input --json-input payload.json --output=json
jira issue create --dry-run --no-input --json-input payload.json --output=json
jira issue edit PROJ-1 --summary "New title" --output=json
jira issue edit PROJ-1 --json-input fields.json --output=json
```

Use `--dry-run` to inspect mutation payloads without submitting to Jira.
Destructive commands require `--force`.

## Comments

```sh
jira issue comment add PROJ-1 --body-markdown "Looks good" --output=json
jira issue comment add PROJ-1 --json-input adf.json --no-input --output=json
jira issue comment list PROJ-1 --all --output=json
jira issue comment delete PROJ-1 10042 --force --output=json
```

Native ADF JSON is the canonical rich-text path. Markdown is a convenience
layer.

## Links And Watchers

```sh
jira issue link PROJ-2 --to PROJ-1 --type Blocks --output=json
jira issue link list PROJ-2 --output=json
jira issue link delete PROJ-2 9001 --force --output=json
jira issue watchers list PROJ-1 --output=json
jira issue watch PROJ-1 --output=json
jira issue unwatch PROJ-1 --output=json
```

Use `jira issue link types --output=json` to inspect the link types configured
in the Jira instance.
