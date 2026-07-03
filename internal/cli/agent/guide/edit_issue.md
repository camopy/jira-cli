## edit_issue
Goal: Update one or more fields on an existing issue without opening an editor.
When: one or more fields on an existing issue need new values; the bare-form `jira issue edit KEY` editor flow is not safe under an agent and is refused (exit 3).

**Decide**

# target
- Issue key positional argument, list, or range (e.g. `<ISSUE_KEY>` or `<PROJECT_KEY>-1..10`).

# scope
- Single field, fast path: a field flag (`--summary`, `--assignee`, etc.).
- Rich description (or any ADF field): `--json-input payload.json` with a `fields` envelope carrying native ADF — the canonical, lossless path, and the only one that can express mentions, dates, panels, status, and tables. Prefer this in agent context. See → `adf_reference`.
- Description from Markdown, lossy shortcut: `--markdown "..."` or `--markdown-file FILE` (- reads stdin) (or a `description_markdown` key inside `fields`) converts Markdown to ADF with the same converter `create` uses. Use only for plain prose you can afford to flatten — it silently cannot emit mentions/dates/panels/status/tables; strict mode aborts on any lossy node, `--adf-best-effort` keeps the rest with a warning.
- Interactive humans only (NOT agents): bare `jira issue edit KEY` opens `$EDITOR` on the description.

# guard
- Always pass `--no-input` in agent context to surface validation errors rather than blocking on a prompt.
- Pass at least one field flag or `--json-input` — empty edits are rejected (exit 3), never silent successes.

**Run**
- Canonical (bulk JSON): `jira issue edit KEY --no-input --json-input fields.json --output=json`
- Single field: `jira issue edit KEY --no-input --summary "New title" --output=json`
- Description from Markdown: `jira issue edit KEY --no-input --markdown "## Notes\n\nUpdated." --output=json`
- Multi-key single field: `jira issue edit <PROJECT_KEY>-1..10 -p 4 --no-input --summary "New title" --output=json`
- Reassign: `jira issue edit KEY --no-input --assignee me --output=json` (also accepts `none`, a bare `accountId`, or an email — an `@` value must be a bare valid address, resolved to an account id via a live `/user/search`, so it is rejected under `--dry-run`)
- Stdin variant: `cat fields.json | jira issue edit KEY --no-input --json-input - --output=json`

Bulk edit payload shape:

```json
{
  "fields": {
    "summary": "New title",
    "labels": ["updated", "v2"],
    "priority": {"name": "Lowest"},
    "duedate": "2026-07-15",
    "description": {"type": "doc", "version": 1, "content": [...]}
  }
}
```

**Save**
> Requires `--output=json`.
- `data.issue` [string, required] — echo of the edited issue key.
- Multi-key edits return ordered `data.results[]`; each successful entry has `data.issue`, `data.fields`, `data.dry_run`, and live submits include `data.result`.
- `meta.command` [string] — `issue.edit`.
- Field values after the edit are NOT projected in the envelope; → `read_issue` to confirm rendered state.

**Preconditions**
- **Agents must use field flags or `--json-input`.** The bare `jira issue edit KEY` form opens an interactive editor on the description. The CLI detects agent context (env vars like `CLAUDECODE`, `AI_AGENT`, etc.) and non-TTY stdin and refuses with exit 3 plus a remediation pointer:

  ```text
  validation: issue edit requires an interactive terminal for the editor flow;
    in agent or non-TTY context, provide --summary, --assignee, or --json-input
  ```

- `--no-input` requires **at least one field**. Empty edits are validation errors (exit 3):

  ```sh
  jira issue edit KEY --no-input             # ❌ exit 3
  jira issue edit KEY --no-input --summary X # ✓ ok
  ```

- ADF rules from → `create_issue` apply to `fields.description` (and other ADF fields) verbatim — pass the bare field name with the ADF doc as its value; detection is by value shape, not key suffix. Native ADF is the agent path for full node coverage. `--markdown` / the `description_markdown` payload key are the lossy plain-prose shortcut (same converter as create) — use only when the loss is acceptable; the flag is mutually exclusive with `--json-input` — pick one path per call.

**Behavior**
- For interactive humans only: the bare form opens `$EDITOR` on the description. Editors that fork-and-return (e.g. `code` without `--wait`) are refused at spawn time with a one-line fix (`set EDITOR='code --wait'`) — silent strikethrough-and-data-loss is gone. See → `configure_editor` for the full editor resolution chain.
- Multi-key edit requires explicit field flags or `--json-input`; the editor flow is always single-key.
- Custom-field encoding follows the same cached-schema path as create; prime → `cache_metadata` if values aren't sticking.
- `-p` / `--parallelism` is bounded to 1..16 and applies to multi-key explicit edits.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3 with `requires an interactive terminal` | Bare `jira issue edit KEY` in agent / non-TTY context | Re-run with `--summary`, `--assignee`, or `--json-input` |
| exit 3 from `--no-input` with no fields | Called without any field flag and without `--json-input` | Add at least one field flag or `--json-input` |
| `Field does not support update 'issuelinks'` | Tried to set `issuelinks` via bulk edit | Drop `issuelinks` from the payload — issue links cannot be set this way; → `link_issues` instead |
| Editor spawn refused with `set EDITOR='code --wait'` | `$EDITOR` forks and returns (data-loss path) | → `configure_editor` to set a blocking editor, then retry; or switch to field flags / `--json-input` |
| ADF rejection on `description` | Plain string instead of an ADF doc, or unknown node | Wrap in `{type: "doc", version: 1, content: [...]}`; consult → `adf_reference` |

**Next**
- Then: → `read_issue` to verify the change (the edit envelope does not project field values).
- Then: → `transition_issue` if the edit was a precursor to a workflow move.
- Alternative: → `link_issues` for `issuelinks` (cannot be set via bulk edit).
- Composes: → `safe_mutation` (same `--dry-run` / `--no-input` guarantees as the other mutation commands), → `author_adf` (pre-flight a rich description with `adf convert` before submitting it in the `fields` envelope).
