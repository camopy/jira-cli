---
title: Aliases
description: Save a long jira invocation under a short name with alias set, list, delete, and import.
icon: material/rename-box-outline
---

# :label: Aliases

Save a long `jira` invocation under a short name. Aliases live in the profile
config (`~/.config/jira-cli/config.toml` under `[aliases]`), expand on every
`jira` call, and apply across every profile. JSON examples below show the `data`
block only — the envelope and exit codes live on [Output](output.md), and each
command links to its reference page for the full flag and output-field tables.

```sh
jira alias set inbox "issue list --assignee me --status 'To Do,In Progress'"
jira inbox
```

The second line runs the stored expansion verbatim.

## set

Create or replace an alias. The expansion is the rest of a `jira` command line —
quote any embedded arguments so the parent shell passes them through verbatim.
Setting a name that already exists overwrites the previous expansion without a
prompt; there's no `--force` here, but `--dry-run` previews the change (it
returns `data.dry_run: true` and writes nothing). `alias delete` and `alias
import` take `--dry-run` too. `alias delete` also takes `--force` (see below).

```sh
jira alias set inbox "issue list --assignee me --status 'To Do,In Progress'"
jira alias set my-bugs "search jql 'project = PROJ AND type = Bug AND assignee = currentUser()'"
```

```json
{
  "name": "inbox",
  "expansion": "issue list --assignee me --status 'To Do,In Progress'"
}
```

[Full flags & output fields →](reference/jira/alias/set.md)

## list

Print every alias the active config defines. Human output collapses the map to a
`{...}` placeholder, so use `--output=json` to see the names and expansions; an
empty table comes back as `data: {}`.

```sh
jira alias list
```

```json
{
  "inbox": "issue list --assignee me --status 'To Do,In Progress'",
  "my-bugs": "search jql 'project = PROJ AND type = Bug AND assignee = currentUser()'"
}
```

[Full flags & output fields →](reference/jira/alias/list.md)

## delete

Drop a single alias by name. Removing local state is a mutation, so — like
`cache clear` — a live delete needs `--force` in headless, agent, or
`--no-input` mode; an interactive terminal proceeds without a prompt. `--dry-run`
previews the delete without writing.

```sh
jira alias delete inbox            # interactive terminal
jira alias delete inbox --force    # agent / script / piped
```

```json
{ "name": "inbox", "deleted": true }
```

[Full flags & output fields →](reference/jira/alias/delete.md)

## import

Load aliases from a YAML file (or stdin with `-`). Each top-level key is the
alias name; the value is the expansion string.

```yaml
# aliases.yaml
inbox: issue list --assignee me --status 'To Do,In Progress'
my-bugs: search jql 'project = PROJ AND type = Bug AND assignee = currentUser()'
standup: search saved standup-jql
```

```sh
jira alias import aliases.yaml
jira alias import aliases.yaml --clobber
cat aliases.yaml | jira alias import -
```

Without `--clobber`, the import skips any name that already exists and reports
each conflict under `data.skipped` as a `name → reason` map (`"name already
taken"` for a collision). Other skip reasons cover an empty expansion, or one
that doesn't resolve to a `jira` command or alias. With `--clobber`, existing
names are overwritten and `data.skipped` comes back empty (the other skip reasons
still apply).

```json
{
  "aliases": [],
  "imported": 0,
  "skipped": {
    "inbox": "name already taken",
    "my-bugs": "name already taken"
  }
}
```

[Full flags & output fields →](reference/jira/alias/import.md)

## See also

*   [Configuration › Aliases](config.md#top-level-keys) — the `[aliases]` key the alias commands read and write
*   [`search saved`](search.md#search-saved) — for a query you reuse, a saved `.jql` file beats an alias: it lives outside the shell-quoting layer
*   [Output](output.md) — the JSON envelope and exit codes
</content>
