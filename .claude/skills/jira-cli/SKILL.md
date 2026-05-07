---
name: jira-cli
description: Use when the user asks about Jira issues, tickets, sprints, JQL, comments, transitions, links, worklog, or anything that needs the matcra587/jira-cli binary
allowed-tools: Bash(jira agent:*) Bash(jira cache:*)
---

# jira-cli

Operational skill for `matcra587/jira-cli`. The binary embeds its own
authoritative reference. **Read it first:**

```sh
jira agent guide              # full operational guide (markdown)
jira agent schema --compact   # full command tree + flag signatures (JSON)
jira agent adf-matrix --json  # ADF node/mark support matrix
jira agent fieldtypes --json  # customfield encoder registry
```

This skill points to those four and captures the contracts, gotchas, and
patterns that trip agents up before they read the guide.

## When to use

- Create, view, edit, delete, clone, move, or comment on a Jira issue
- Build or run JQL queries
- Manage worklog, transitions, issue links, or web links
- Manage Jira auth, profiles, or read-only mode
- Author ADF (Atlassian Document Format)

## Setup check

```sh
jira me                  # who am I?
jira config profile      # what profiles exist?
jira auth status         # credential health
```

If `jira me` fails: run `jira config init` to create the profile metadata,
then `jira auth login` to add the credential.

## Output mode contract

Four mutually exclusive flags. Combining any two returns exit 3.

| Flag        | Effect |
|-------------|--------|
| `--json`    | Full envelope: `{meta, data, errors, warnings}` |
| `--compact` | Same envelope, jq-friendly single line; success-path strips `meta`/`errors` |
| `--plain`   | clog rich text (TTY default) |
| `--raw`     | Underlying Jira REST shape, no envelope |

Exit codes: `0` success, `1` auth, `2` not-found, `3` validation,
`4` rate-limited, `5` server error.

Agent envs auto-select `--compact` (`CLAUDE_CODE`, `CURSOR_AGENT`,
`COPILOT_CLI`, `CODEX_*`, `GEMINI_CLI`, `OPENCODE`).

## Reading issues

```sh
jira issue view KEY                        --json
jira issue list --jql 'project = KAN'      --json
jira search jql 'JQL'                      --json
```

The typed `--json` view drops `parent`, `subtasks`, `issuetype.name`, and
sometimes `summary`. Use `--raw` to recover them.

## Writing issues

**Use `--json-input` for non-trivial writes.** `--no-input` requires
`project_key` and `issue_type` in the payload. Bare `--summary` combos exit 3.

```sh
echo '{"summary":"X","project_key":"KAN","issue_type":"Task"}' > /tmp/p.json
jira issue create --json-input /tmp/p.json --no-input --dry-run --json
jira issue create --json-input /tmp/p.json --no-input --json
```

Payload key aliases the CLI translates: `project_key → project.key`,
`issue_type → issuetype.name`, `description_markdown → description (lossy)`,
`assignee_account_id → assignee.accountId`.

**For ADF descriptions, use the bare Jira field name** (`description`,
`environment`, `customfield_NNNN`). ADF is the canonical format —
assign the ADF doc directly as the value:

```json
{"summary": "X", "project_key": "KAN", "issue_type": "Task",
 "description": {"type": "doc", "version": 1, "content": [...]}}
```

The CLI walks the payload, detects ADF-shaped values
(`{type: "doc", version: N, content: [...]}`) on any key, and runs
client-side validation. There is no `_adf` suffix convention — Jira
would reject any unknown field name on the wire.

## Command quick reference

For shapes, flags, dry-run, and read-back patterns, run `jira agent guide`.
High-yield triggers:

| Need | Command |
|------|---------|
| Edit one field | `jira issue edit KEY --summary "..." --json` |
| Bulk edit | `jira issue edit KEY --json-input fields.json --json` |
| List comments (oldest-first) | `jira issue comment list KEY --json` |
| Add comment (markdown) | `jira issue comment add KEY --body-markdown "..." --no-input --json` |
| Add comment (ADF, preferred) | `jira issue comment add KEY --json-input adf.json --no-input --json` |
| Edit comment | `jira issue comment edit KEY COMMENT_ID --body-markdown "..." --json` |
| Clear comment visibility | `jira issue comment edit KEY COMMENT_ID --clear-visibility --json` |
| Delete comment | `jira issue comment delete KEY COMMENT_ID --force --no-input --json` |
| List attachments | `jira issue attachment list KEY --json` |
| Upload attachment | `jira issue attachment add KEY --file ./report.pdf --json` |
| Download attachment | `jira issue attachment download KEY ATT_ID --output ./local.pdf --json` |
| Delete attachment | `jira issue attachment delete KEY ATT_ID --force --no-input --json` |
| List watchers | `jira issue watchers list KEY --json` |
| Self-watch / unwatch | `jira issue watch KEY --json`  /  `jira issue unwatch KEY --json` |
| Add watcher (email/me/accountId) | `jira issue watchers add KEY --user teammate@example.com --json` |
| Remove watcher | `jira issue watchers remove KEY --user accountId:712020:abc --json` |
| List issue links | `jira issue link list KEY --json` |
| Delete issue link | `jira issue link delete KEY LINK_ID --force --no-input --json` |
| List configured link types | `jira issue link types --json` |
| Prime link-types cache | `jira cache linktypes --refresh --json` |
| Issue link (create) | `jira issue link <BLOCKED> --to <BLOCKER> --type Blocks --json` |
| List transitions | `jira issue transition KEY --json` |
| Execute transition | `jira issue transition KEY --transition <id> --json` |
| Worklog | `jira worklog add KEY --time-spent 1h30m --no-input --json` |
| Web link (http/https only) | `jira issue weblink KEY --url URL --title "..." --json` |
| Clone | `jira issue clone KEY --force [--json-input overrides.json] --json` |
| Move project/type | `jira issue move KEY --force --json-input move.json --json` |
| Delete a parent | `jira issue delete KEY --force --delete-subtasks --json` |

Destructive operations require `--force` under `--no-input` —
`issue delete`, `issue clone`, `issue move`, `comment delete`,
`attachment delete`, and `link delete` all gate behind it. The bare
`jira issue comment KEY ...` form remains as an alias for
`jira issue comment add KEY ...`.

Watcher `--user` resolution: `me` → caller's accountId; `accountId:<id>` →
sent verbatim; otherwise `/user/search` resolves it. 0 matches → exit 2;
1 match → proceed; 2+ matches → exit 3 with every candidate listed
(re-run with `accountId:<id>` to disambiguate). Watcher add/remove are
idempotent; a follow-up GET reflects the final state unless
`--no-readback` is set.

`comment list` renders ADF bodies as best-effort Markdown by default;
constructs the renderer can't fully express (`inlineCard`, custom
panels, extension nodes) surface as `warnings[]` of shape
`{type, comment_id, lossy_constructs: [...]}`. Re-run with `--raw` for
Atlassian's native ADF verbatim.

## JQL

Use the builder. It avoids quoting bugs and keyword-injection risk.

```sh
jira jql build --project KAN --assignee me --status Done --json
# → {"jql": "project = KAN AND assignee = currentUser()
#           AND status = Done ORDER BY updated DESC"}
```

Builder flags: `--project`, `--epic`, `--assignee me`, `--reporter me`,
`--status`, `--priority`, `--label`, `--type`, `--order-by`, `--desc`.

`jira issue list --as-jql --json` previews builder output without hitting the
API. Pipe the result into `jira search jql` to execute.

## Cache

Prime the field cache once per session before authoring custom fields:

```sh
jira cache fields --refresh --json   # → customfield_NNNN ↔ name map
```

Other primers: `cache projects`, `cache issuetypes`, `cache labels`,
`cache epics`. Wipe with `cache clear [<resource>]`.

## Gotchas (non-obvious — these will trip you up)

- `--no-input` is strict. Missing any required field → exit 3.
- `issue view --json` drops `parent`, `subtasks`, `issuetype.name`, and
  sometimes `summary`. Use `--raw` when you need them.
- `issuelinks` **cannot** be set via `issue edit` bulk update. Jira refuses.
  Use `jira issue link`.
- `subtasks` block deletion of the parent. Pass `--delete-subtasks`.
- Read-only mode (`JIRA_READ_ONLY=1` or profile `read_only=true`) blocks every
  mutation at the HTTP layer. `--force` does NOT bypass. `--dry-run` is
  always permitted.
- `auth login --no-input` **merges** partial flags into the existing profile.
  Pass every field if you want a clean replace.
- ADF-shaped values are validated client-side on **any** payload key —
  detection is by value shape (`{type: "doc", ...}`), not key suffix.
  Strict mode rejects unknown nodes/marks; best-effort warns.
- Web link URLs accept only `http://` and `https://`. Other schemes
  (`javascript:`, `mailto:`, `file:`, etc.) are rejected before any HTTP call.

For deep ADF authoring patterns (block/inline/mark recipes and the full gotcha
list), read the ADF reference section of `jira agent guide`.

## When this skill isn't enough

Read the embedded guide. It is kept in lockstep with the binary.

```sh
jira agent guide
jira agent schema --compact
```
