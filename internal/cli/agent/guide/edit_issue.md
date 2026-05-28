## edit_issue
Goal: Update one or more fields on an existing issue without opening an editor.
When: one or more fields on an existing issue need new values; the bare-form `jira issue edit KEY` editor flow is not safe under an agent and is refused (exit 3).

**Decide**

# target
- Issue key positional argument (e.g. `<ISSUE_KEY>`).

# scope
- Single field, fast path: a field flag (`--summary`, `--assignee`, etc.).
- Multiple fields or ADF body: `--json-input payload.json` with a `fields` envelope.
- Interactive humans only (NOT agents): bare `jira issue edit KEY` opens `$EDITOR` on the description.

# guard
- Always pass `--no-input` in agent context to surface validation errors rather than blocking on a prompt.
- Pass at least one field flag or `--json-input` — empty edits are rejected (exit 3), never silent successes.

**Run**
- Canonical (bulk JSON): `jira issue edit KEY --no-input --json-input fields.json --output=json`
- Single field: `jira issue edit KEY --no-input --summary "New title" --output=json`
- Reassign: `jira issue edit KEY --no-input --assignee me --output=json` (also accepts `none` or a bare `accountId`)
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
- `data.key` [string, required] — echo of the edited issue key.
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

- ADF rules from → `create_issue` apply to `fields.description` (and other ADF fields) verbatim — pass the bare field name with the ADF doc as its value; detection is by value shape, not key suffix.

**Behavior**
- For interactive humans only: the bare form opens `$EDITOR` on the description. Editors that fork-and-return (e.g. `code` without `--wait`) are refused at spawn time with a one-line fix (`set EDITOR='code --wait'`) — silent strikethrough-and-data-loss is gone. See → `configure_editor` for the full editor resolution chain.
- Custom-field encoding follows the same cached-schema path as create; prime → `cache_metadata` if values aren't sticking.

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
- Composes: → `safe_mutation` (same `--dry-run` / `--no-input` guarantees as the other mutation commands).
