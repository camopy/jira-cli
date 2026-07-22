---
name: jira-how-to
description: Route any Jira task to the right jira-cli workflow guide — the CLI ships its own embedded guides and a runtime command schema, so this skill teaches where to look, not what every flag does.
when_to_use: Working with Jira issues, JQL, boards, worklogs, or authentication through the jira CLI, or unsure which jira-cli command or flag fits a task.
allowed-tools: Bash(jira agent guide *), Bash(jira agent schema *), Bash(jira guide *)
license: MIT
metadata:
  origin: jira-cli
---

# jira how-to

jira-cli documents itself. The binary carries workflow guides and a
runtime schema; this skill is the router that gets you to the right one
in two commands. Never guess a flag from memory — the schema is derived
from the live binary and cannot be stale.

## The two-step route

1.  **Pick the workflow.** List the guides and match the task:

    ```sh
    jira agent guide
    ```

    | Task looks like | Load |
    |---|---|
    | Parsing output, exit codes, `--output`, headless gates | `core-contract` |
    | First run, auth errors, switching tenants | `bootstrap` |
    | Unknown project/type/field/status names | `discover` |
    | Reading, listing, searching, the `jql build` flag-to-JQL builder | `find-issues` |
    | Any write, before you run it | `safe-mutation` |
    | Creating or updating issues, transitions | `shape-issues` |
    | Comments, attachments, links, watchers, worklogs | `annotate-issues` |
    | Clone, move, rank, delete | `restructure-issues` |
    | Rich-text bodies, Markdown to ADF | `write-rich-text` |

2.  **Read it, then act:**

    ```sh
    jira agent guide safe-mutation
    ```

## Facts come from the schema, not prose

Flags, argument shapes, per-command input/output JSON schemas:

```sh
jira agent schema --path "issue create"
```

The full tree is `jira agent schema`; each command node embeds its
schemas. `jira agent adf-matrix` and `jira agent fieldtypes` cover the
rich-text and custom-field registries.

## Ground rules that never change

*   Machine output: `--output compact` (bare payload) or `--output json`
    (envelope). Parse stdout; stderr is diagnostics.
*   Every mutation: `--dry-run` first, then `--no-input` to submit;
    destructive commands also need `--force`. The gates are the contract —
    do not script around them.
*   Details and recovery steps live in `core-contract` and
    `safe-mutation`; load them before the first parse and the first write.

## Exported skills

`jira agent export --format agent-skill --dir <dir>` materializes every
guide as a standalone skill for harnesses that prefer installed skills
over CLI calls. The embedded guides remain the source of truth.
