---
title: For agents
description: Drive jira from an AI agent — the machine-readable schema, embedded runbooks, and the ADF matrix.
icon: material/robot-outline
---

# :robot: For agents

`jira agent` exposes the CLI's machine-readable metadata and the embedded
runbooks an LLM-driven agent (Claude, Codex, Cursor, etc.) needs to drive `jira`
non-interactively without re-discovering the contract on every session.

## What an agent can rely on

*   **One stable envelope, on success and failure alike.** Both return the same
    JSON shape on stdout — branch on `meta.exit_code` and `errors[].code`, never
    on prose. See [Output](output.md#envelope).
*   **A token-economical mode.** `--output=compact` drops the wrapper and every
    `null` key, recursively ([Output › Modes](output.md#modes)); narrow reads
    further with `--fields` on [search](search.md#search-jql) and
    [`issue list`](issue/read.md#list).
*   **Raw payloads, not just flags.** Every mutation takes `--json-input` (or `-`
    for stdin) alongside the convenience flags, so an agent submits a structured
    body directly. See [Creating & editing](issue/create-edit.md).
*   **Runtime introspection.** Discover the contract from the binary, not
    pre-stuffed docs: [`agent schema`](#schema), [`agent fieldtypes`](#registries),
    and [`agent adf-matrix`](#registries) below, plus live instance metadata from
    [`jql reference`](jql.md#reference).
*   **A preview before every write.** `--dry-run` validates and echoes the
    resolved payload without contacting Jira — see the [`safe_mutation`](#guides)
    runbook.
*   **Auto-detection.** Under `--output=auto`, a detected agent gets compact JSON
    with no flag needed ([Output › Modes](output.md#modes)).

The subcommands, each detailed below:

```sh
jira agent schema --output=json
jira agent guide
jira agent guide <slug>
jira agent adf-matrix --output=json
jira agent fieldtypes --output=json
```

## schema

`agent schema` reports the binary's own contract as JSON:

*   the agent-contract version at `data.schema_version` (semver; the full
    `agent guide` output stamps the same value)
*   command paths and subcommands
*   global and local flags
*   flag enums and completion predictors
*   mutual-exclusion and required-together flag groups
*   input schemas for JSON payloads
*   output schemas for the envelope and selected commands

Use `--output=compact` when an agent wants the schema data without the
JSON envelope wrapper.

## Guides

`agent guide <slug>` prints one runbook from the embedded set. Each
guide follows a **Decide → Run → Save → Preconditions → Recover → Next**
shape so an agent can pattern-match on a known structure. The slugs
below are the canonical invocation names.

### Foundation

Cross-cutting contracts every workflow inherits.

`core_contract`
:   The output-mode, envelope, exit-code, headless, pagination,
    read-only, and debug contract every other workflow inherits.

`identity_setup`
:   Resolve identity once per profile so `--assignee me`, the TUI
    `A` key, and "in my epics" JQL all work without further setup.

`auth_setup`
:   Wire a Jira profile to a valid credential: backend chosen,
    profile populated, secret never on disk in plaintext.

`inspect_schema`
:   Introspect the CLI's command tree, JSON output schemas, ADF
    support set, and customfield encoders without reading prose.

`configure_editor`
:   Pin the editor for `jira issue edit KEY` so TTY edits land in a
    tool that blocks until you save, instead of forking and losing
    the change.

`safe_mutation`
:   Wrap every destructive or state-changing Jira call with the right
    confirmation + preview discipline.

`cache_metadata`
:   Prime, refresh, or clear the per-profile local caches so repeated
    reads and client-side validation don't hit Jira every call.

### Read paths

`read_issue`
:   Fetch one or more issue JSON envelopes so downstream workflows can
    inspect Jira issue objects.

`list_issues`
:   Page through issues for the active profile: by JQL, by board, or
    by expanded key set, or by the default project.

`search_jql`
:   Run a JQL query (hand-authored, flag-built, or saved on disk) and
    capture matching issue keys.

`list_comments`
:   Walk an issue's comment thread oldest-first, paginating through
    every page, with lossy-ADF surface detection.

`discover_board`
:   Enumerate the agile boards visible to the active profile so
    `--board` filters resolve to a known id.

### Mutations

`create_issue`
:   Create a Jira issue from a structured payload and capture the new
    key for follow-up mutations.

`create_subtask`
:   Create a subtask under an existing parent issue and capture the
    new child key.

`edit_issue`
:   Update one or more fields on an existing issue without opening an
    editor.

`transition_issue`
:   Move an issue to a new workflow state by picking an available
    transition ID.

`clone_issue`
:   Duplicate an existing issue (same or different project),
    sanitising lifecycle/timing noise, optionally with overrides.

`move_issue`
:   Swap an existing issue's `project` and/or `issuetype` in place
    without creating a new issue or changing the key history.

`delete_issue`
:   Permanently remove an issue (and optionally its subtasks) after
    confirming nothing downstream depends on it.

### Side workflows

`add_comment`
:   Post a comment on a Jira issue, capturing the persisted comment
    shape for follow-up edits.

`attach_file`
:   List, upload, download, or remove file attachments with size-cap
    and clobber/force guards.

`manage_watchers`
:   Resolve, add, remove, or audit watchers, handling ambiguous user
    input via structured `candidates[]`.

`link_issues`
:   Create, audit, list, or delete typed issue-to-issue links,
    respecting Jira's inward/outward semantics.

`add_weblink`
:   Attach a remote URL (with display title) via the remote-link
    endpoint, rejecting non-web schemes client-side.

`log_work`
:   Record time spent against an issue using durations that resolve
    via the profile's `workday_seconds`.

### Reference

`adf_reference`
:   Node shapes, mark composition, gotchas (mention id, date
    timestamp, code block content), strict-vs-best-effort modes.

`jql_reference`
:   Operators, keywords, functions, and recipes that compose into a
    query string.

Run `jira agent guide --help` to see the live list (slugs and one-line
descriptions) as the binary ships with them.

## Registries

`jira agent adf-matrix --output=json`
:   The live ADF node/mark support set: which constructs the CLI
    accepts under strict mode, which downgrade under best-effort, and
    which are rejected.

`jira agent fieldtypes --output=json`
:   The customfield type registry: how each `customfield_*` value
    shape is encoded on the wire, keyed by Jira's `schema.custom`
    token.

Both share the standard envelope shape under `--output=json` and the
unwrapped data block under `--output=compact`.
