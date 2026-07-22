---
name: jira-how-to
description: Route any Jira task to the right jira-cli workflow guide — the CLI ships its own embedded guides and a runtime command schema, so this skill teaches where to look, not what every flag does.
when_to_use: Any task that touches Jira, even when jira-cli isn't named — creating, editing, transitions, comments, attachments, links, worklogs, JQL search, boards, auth or profile setup, ADF rich text, custom fields, headless/agent automation — or unsure which jira-cli command or flag fits a task.
allowed-tools: Bash(jira agent *), Bash(jira guide *)
license: MIT
metadata:
  origin: jira-cli
---

# jira how-to

jira-cli documents itself. The binary (v0.14.0+ on PATH) carries
workflow guides and a runtime schema; this skill is the router that gets
you to the right one in two commands. Never guess a flag from memory —
the schema is derived from the live binary and cannot be stale.

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
    | ADF with marks, panels, tables, or unusual nodes | `jira agent adf-matrix` |
    | Writing a `customfield_NNNN` value | `jira agent fieldtypes` |

2.  **Read it, then act:**

    ```sh
    jira agent guide safe-mutation
    ```

Most real tasks span guides — load every applicable one together at the
start, not one at a time. Common combinations:

| Task | Load together |
|---|---|
| Create an issue with a rich description | `safe-mutation` + `shape-issues` + `write-rich-text` |
| First mutation in an unfamiliar project | `discover` + `safe-mutation` + the verb's guide |
| Script that parses results headlessly | `core-contract` + `find-issues` |

Boundaries where guides overlap: `discover` is for names and ids that
must be exact (types, fields, statuses); `find-issues` is for the issues
themselves. `shape-issues` changes what an issue says, `annotate-issues`
adds records to it, `restructure-issues` changes where it lives.
`safe-mutation` is the cross-cutting write discipline — it rides along
with all three, never replaces them.

## Facts come from the schema, not prose

Flags, argument shapes, per-command input/output JSON schemas:

```sh
jira agent schema --path "issue create"
```

The full tree (`jira agent schema`) is structure only — nodes carry
`has_input_schema`/`has_output_schema` markers; fetch a command's bodies
with `--path`, or everything with `--shapes`.

## Ground rules that never change

*   Machine output: `--output compact` (bare payload) or `--output json`
    (envelope). Parse stdout; stderr is diagnostics.
*   Every mutation: `--dry-run` first, then `--no-input` to submit;
    destructive commands also need `--force`. The gates are the contract —
    do not script around them.
*   Details and recovery steps live in `core-contract` and
    `safe-mutation`; load them before the first parse and the first write.

## Never

*   Never build a command from memory or another Jira CLI's flags — read
    the schema; if prose and the live schema disagree, the schema wins.
*   Never treat a clean `--dry-run` as proof Jira will accept the live
    write — the preview validates locally; the server can still refuse.
*   Never script around the gates: `--no-input`, `--force`, and read-only
    mode exist because of agents. Read-only blocks at the HTTP layer;
    `--force` does not bypass it.
*   Never copy guide content into this skill or your own notes — the
    embedded guides are the source of truth and this router must stay
    thin.

## Exported skills

`jira agent export --format agent-skill --dir <dir>` materializes every
guide as a standalone skill for harnesses that prefer installed skills
over CLI calls. The embedded guides remain the source of truth.
