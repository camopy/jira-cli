# Alias

Save a long `jira` invocation under a short name. Aliases live in the
profile config (`~/.config/jira-cli/config.toml` under `[aliases]`),
expand on every `jira` call, and apply across every profile.

```sh
jira alias set inbox "issue list --assignee me --status 'To Do,In Progress'"
jira inbox                    # expands to the issue list call above
```

## set

Create or replace an alias. The expansion is the rest of a `jira`
command line — quote any embedded arguments so the parent shell passes
them through verbatim.

```sh
jira alias set inbox "issue list --assignee me --status 'To Do,In Progress'"
jira alias set my-bugs "search jql 'project = <PROJECT_KEY> AND type = Bug AND assignee = currentUser()'"
```

=== "Human"

    ```text
    INF ℹ️ expansion="issue list --assignee me --status 'To Do,In Progress'" name=inbox
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "alias.set", "timestamp": "…", "request_id": "…" },
      "data": {
        "name": "inbox",
        "expansion": "issue list --assignee me --status 'To Do,In Progress'"
      },
      "errors": [],
      "warnings": []
    }
    ```

Setting an alias with an existing name overwrites the previous
expansion without a confirmation prompt — there's no `--force` here.

## list

Print every alias the active config defines.

```sh
jira alias list
```

=== "Human"

    ```text
    INF ℹ️ value={...}
    ```

    The Human formatter collapses the alias map to a `{...}` placeholder;
    use `--output=json` to see the actual names and expansions.

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "alias.list", "timestamp": "…", "request_id": "…" },
      "data": {
        "inbox": "issue list --assignee me --status 'To Do,In Progress'",
        "my-bugs": "search jql 'project = <PROJECT_KEY> AND type = Bug AND assignee = currentUser()'"
      },
      "errors": [],
      "warnings": []
    }
    ```

An empty alias table renders as `INF ℹ️ value={}` (Human) or
`data: {}` (JSON).

## delete

Drop a single alias by name.

```sh
jira alias delete inbox
```

=== "Human"

    ```text
    INF ℹ️ deleted=true name=inbox
    ```

=== "JSON"

    ```json
    {
      "ok": true,
      "meta": { "command": "alias.delete", "timestamp": "…", "request_id": "…" },
      "data": { "name": "inbox", "deleted": true },
      "errors": [],
      "warnings": []
    }
    ```

## import

Load aliases from a YAML file. Each top-level key is the alias name;
the value is the expansion string.

```yaml
# aliases.yaml
inbox: issue list --assignee me --status 'To Do,In Progress'
my-bugs: search jql 'project = <PROJECT_KEY> AND type = Bug AND assignee = currentUser()'
standup: search saved standup-jql
```

```sh
jira alias import aliases.yaml
jira alias import aliases.yaml --clobber   # overwrite existing names
cat aliases.yaml | jira alias import -     # read from stdin
```

Without `--clobber`, the import skips any name that already exists and
reports each conflict under `data.skipped` as a map of `name → reason`
(`"name already taken"` for a collision). Other skip reasons cover an
empty expansion or one that doesn't resolve to a `jira` command or
alias. With `--clobber`, existing-name conflicts are overwritten and
`data.skipped` comes back empty (other skip reasons still apply).

=== "Human"

    Fresh import:

    ```text
    INF ℹ️ aliases="[2 items]" imported=2 skipped={}
    ```

    Re-import without `--clobber` (every name already taken):

    ```text
    INF ℹ️ aliases=[] imported=0 skipped={...}
    ```

=== "JSON"

    Fresh import (no conflicts):

    ```json
    {
      "ok": true,
      "meta": { "command": "alias.import", "timestamp": "…", "request_id": "…" },
      "data": {
        "aliases": ["inbox", "my-bugs"],
        "imported": 2,
        "skipped": {}
      },
      "errors": [],
      "warnings": []
    }
    ```

    Re-import without `--clobber` (every name already exists):

    ```json
    {
      "ok": true,
      "meta": { "command": "alias.import", "timestamp": "…", "request_id": "…" },
      "data": {
        "aliases": [],
        "imported": 0,
        "skipped": {
          "inbox": "name already taken",
          "my-bugs": "name already taken"
        }
      },
      "errors": [],
      "warnings": []
    }
    ```

## See also

*   [Configuration › Aliases](config.md#aliases) — the `[aliases]` TOML key the alias commands read and write.
*   [Search › search saved](search.md#search-saved) — for queries you reuse repeatedly, a saved JQL file beats an alias because it lives outside the shell-quoting layer.
