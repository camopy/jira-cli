## link_issues
Goal: Create, audit, list, or delete typed issue-to-issue links, respecting Jira's inward/outward semantics and the bulk-edit exclusion.
When: two issues need a typed relationship (blocks, relates, is-blocked-by, etc.) or the existing link set on an issue needs to be reviewed or pruned.

**Decide**

# direction & semantics
- `KEY` is the inward side, `--to` the outward. "<BLOCKER_ISSUE_KEY> blocks <BLOCKED_ISSUE_KEY>" means `KEY=<BLOCKED_ISSUE_KEY> --to <BLOCKER_ISSUE_KEY> --type Blocks`.
- A blocks B (B is blocked by A): `jira issue link <BLOCKED> --to <BLOCKER> --type Blocks`.
- A and B related (no direction): `jira issue link <BLOCKED_ISSUE_KEY> --to <BLOCKER_ISSUE_KEY> --type Relates`.
- A is a duplicate of canonical B: `jira issue link <DUP> --to <CANONICAL> --type Duplicate`.
- A is a clone of B: `jira issue link <CLONE> --to <ORIGINAL> --type Cloners`.

# subcommand
- Create: `jira issue link KEY... --to OTHER --type <Name>` (with optional `--dry-run`).
- List existing links on an issue: `jira issue link list KEY`.
- List links for several issues/ranges: `jira issue link list KEY... -p N`; multi-key output uses `data.results[]`.
- Delete by link id: `jira issue link delete KEY <link-id> --force`.
- Discover available link types: `jira issue link types` (cached; primable via `jira cache linktypes`).

# guards
- `link delete` is force-gated under `--no-input`.
- `--dry-run` previews creation without contacting Jira.

**Run**
- Create blocker: `jira issue link <BLOCKED_ISSUE_KEY> --to <BLOCKER_ISSUE_KEY> --type Blocks --output=json`
- Bulk create blocker: `jira issue link <PROJECT_KEY>-1..10 -p 4 --to <BLOCKER_ISSUE_KEY> --type Blocks --output=json`
- Create undirected: `jira issue link <BLOCKED_ISSUE_KEY> --to <BLOCKER_ISSUE_KEY> --type Relates --output=json`
- Preview: `jira issue link <BLOCKED_ISSUE_KEY> --to <BLOCKER_ISSUE_KEY> --type Blocks --dry-run --output=json`
- List on issue: `jira issue link list KEY --output=json`
- Multi-key list: `jira issue link list <PROJECT_KEY>-1..10 -p 4 --output=json`
- Delete: `jira issue link delete KEY 9001 --force --output=json`
- Available link types: `jira issue link types --output=json | jq -r '.data.link_types[].name' | sort -u`
- Prime / refresh link-type cache: `jira cache linktypes --output=json`

**Save**
> Requires `--output=json`.

`link list` flattens Atlassian's wire shape — direction-aware `other_issue` instead of inward/outward branching at the call site:

```json
{
  "data": {
    "links": [
      {"id": "9001",
       "type": {"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
       "direction": "outward",
       "other_issue": {"key": "<DOWNSTREAM_ISSUE_KEY>", "summary": "downstream service work", "status": "In Progress"}},
      {"id": "9002",
       "type": {"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
       "direction": "inward",
       "other_issue": {"key": "<UPSTREAM_ISSUE_KEY>", "summary": "upstream API contract", "status": "Done"}}
    ]
  }
}
```

`link delete`:

```json
{"data": {"link_id": "9001", "deleted": true}}
```

`link types` / `cache linktypes`:

```json
{
  "data": {
    "link_types": [
      {"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
      {"id": "10001", "name": "Cloners", "inward": "is cloned by", "outward": "clones"},
      {"id": "10002", "name": "Relates", "inward": "relates to", "outward": "relates to"}
    ],
    "from_cache": true,
    "fetched_at": "2026-05-05T12:00:00Z",
    "count": 3,
    "cache_state": "fresh",
    "cache_source_state": "fresh",
    "cache_empty": false
  }
}
```

- `data.links[].id` [string, required] — pass as `<link-id>` to `link delete`.
- `data.links[].type` [object, required] — `{id, name, inward, outward}`; the verb you want is `outward` when `direction = outward`, `inward` otherwise.
- `data.links[].direction` [string, required] — `outward` or `inward`; tells you which side of the link `KEY` is on.
- `data.links[].other_issue` [object, required] — `{key, summary, status}` for the issue at the far end; this is the field agents should branch on, not raw inward/outward arrays.
- Multi-key create/list: `data.results[]` [array, required] — ordered by requested key; each successful entry has command-specific `data`.
- `data.link_types[].name` [string, required] — what to pass as `--type` on `link create`. Custom types may exist; always discover before assuming.
- `data.from_cache` / `data.cache_state` / `data.cache_empty` [bool / string / bool] — cache-primer convention; see → `cache_metadata`.
- `data.link_id` / `data.deleted` [string / bool, required on delete] — echo + success flag.

**Preconditions**
- Bulk edit cannot update `issuelinks` — Jira refuses with `"Field does not support update 'issuelinks'"`. This command is the only path; do NOT attempt → `edit_issue` to set links.
- Link type names are instance-specific (admins add custom ones). Discover before hard-coding.

**Behavior**
- `link delete --force` is required under `--no-input` (force-gated).
- `-p` / `--parallelism` is bounded to 1..16 and affects multi-key link create/list. `--to` remains one target issue; `link delete` remains single-target because it takes one link id.
- The link-type cache follows the cache-primer convention — `cache linktypes` adds `data.profile` per primer rules.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, error message names a missing type | Unknown `--type` (typo or custom-only type) | Run `jira issue link types` to list configured names, then retry |
| upstream "Field does not support update 'issuelinks'" | Attempted to set `issuelinks` via → `edit_issue` (bulk edit) | Drop that path — use `jira issue link …` instead |
| exit 3 on delete | `--force` missing under `--no-input` | Re-run with `--force` |
| exit 2 (`not_found`) on delete | Wrong link id | Re-run `link list` and copy `data.links[].id` verbatim |

**Next**
- Then: → `read_issue` to confirm the resulting link panel.
- Composes: → `cache_metadata` (link-type cache is part of the metadata-primer suite).
- Alternative: → `add_weblink` for remote URL references (different endpoint, not issue-to-issue).
