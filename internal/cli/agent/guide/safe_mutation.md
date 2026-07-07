## safe_mutation
Goal: Wrap every destructive or state-changing Jira call with the right confirmation + preview discipline so an agent never blows away data it can't recover.
When: any state-changing call is about to run — the `--dry-run` / `--force` / `--no-input` triad and the preview-then-write loop in this guide apply before invoking any mutating workflow.

This is not a command — it's the cross-cutting contract that `clone_issue`, `move_issue`, `delete_issue`, `edit_issue`, `add_comment`, `link_issues`, `add_weblink`, `attach_file`, `manage_watchers`, and `log_work` all defer to. The Decide block below is the canonical confirmation matrix; the Run block is the canonical preview-then-write loop.

**Decide**

# am I running in a TTY or as an agent?
- Interactive human with a TTY, no `--force`: a `huh` confirmation prompt fires; you must type `Yes, delete` (or the equivalent verb) to proceed.
- Interactive human with a TTY + `--force`: no prompt, command runs.
- Non-TTY / agent context / `--no-input`: `--force` MUST be present, or the command refuses with exit `3` and `validation_error`. There is no fallback prompt.
- `--dry-run`: always allowed, regardless of TTY or `--force`. Never touches Jira.

| Mode                              | Required                                           |
|-----------------------------------|----------------------------------------------------|
| TTY human                         | confirmation prompt (`huh`) — type `Yes, delete`   |
| TTY + `--force`                   | no prompt                                          |
| Non-TTY / agent / `--no-input`    | `--force` MUST be present (else exit `3`)          |
| `--dry-run`                       | always allowed; never touches Jira                 |

# what stage am I in?
- Pre-write: validate the payload locally before any network call.
- Real write: machine-parseable invocation with the confirmation flag the target requires.
- Post-write: capture the returned key + envelope as evidence.

# which dry-run semantics apply to my command?
- Local pipeline (parse → ADF compat → customfield encoding, stops before the API call): `issue create`, `issue edit`, `issue clone`, `issue move`, and `issue comment add`. A bare dry-run catches payload shape, ADF strict errors, and a cached-priority mismatch — it has NO screen schema, so unknown field names, invalid issue types, and customfield types pass through unchecked.
- Server-validated pre-flight (read-only, still no write): add `--validate-remote` to the dry-run. `issue create --dry-run --validate-remote` fetches createmeta and runs the same field-schema + customfield checks a live submit gets (unknown fields, missing required fields, resolvable types, invalid issue type); `issue edit` does the same against editmeta; `issue transition` resolves the target against the issue's live transitions and applies the screenless-payload refusal.
- Local-only preview (does not contact Jira): `watchers add --dry-run` when `--user` is locally derivable (`accountId:<id>`, or `me` on a profile that carries an account id), `issue link --dry-run`, `issue weblink --dry-run`, `worklog add --dry-run`, and `issue delete --dry-run`.
- Hybrid resolve: `watchers add --dry-run --validate-remote` does a read-only `/user/search` to resolve a bare name or email but still issues no watcher POST/DELETE.

# local state mutations (no Jira call at all)
- The same triad covers the commands that mutate local state: `cache clear` (wipes cache files), `config set` (writes config.toml), `auth switch` (changes the default profile), `auth logout` (revokes a stored credential), and `update` (replaces the binary). Each takes `--dry-run` for a no-write preview.
- Headless / agent / `--no-input` gating: `cache clear`, `auth logout`, and `update` require `--force` for the live run. `config set` and `auth switch` are ungated by design — both are single-value writes reversed by setting the previous value back.
- None of these prompt an interactive TTY: the verb plus its explicit target carries the intent (`logout <profile>` names its victim, a cleared cache re-primes, the binary self-replace is checksum-verified and rollback-safe).

**Run** (sequence, per mutation)
1. Dry-run: same command with `--dry-run --output=json`; verify payload shape, ADF validity, and any `*_resolved` flags before committing. Add `--validate-remote` when you want the live screen checked too (read-only; `data.validated_remotely: true` confirms it ran).
2. Real write: drop `--dry-run`, keep `--output=json`, add the target's confirmation flag (`--force` for `clone_issue` / `move_issue` / `delete_issue` / attachment delete / link delete; `--no-input` + field flags or `--json-input` for `edit_issue` and `add_comment`).
3. Record the returned issue key, comment id, link id, worklog id, or attachment id from `data.*` as the evidence trail.

For bulk-safe issue-key mutations, pass key lists/ranges as `KEY...` and add `-p N` / `--parallelism N` for bounded fan-out. Multi-key results use ordered `data.results[]` entries with per-key `ok`/`error` values; do not assume one failed key means the successful keys were rolled back. Commands keyed by a secondary object id remain single-target: `comment edit/delete`, `attachment download/delete`, and `link delete`.

**Preconditions**
- Always pass `--output=json` for automation — never parse `--output=human` (it's display-only).
- `edit_issue` in agent context refuses the bare `jira issue edit KEY` form (exit `3`) because the editor flow needs a TTY; pass `--summary`, `--assignee`, or `--json-input`.
- `--no-input` requires at least one field on `edit_issue` — empty edits exit `3`.
- Live multi-key `delete_issue` always requires `--force`, even in an interactive TTY. This avoids a long prompt loop and keeps bulk deletion explicit.

**Behavior**
- A clean bare `--dry-run` means the payload is shaped correctly, NOT that Jira will accept it — unknown field names, invalid issue types, screen membership, and enum values are server knowledge. `--dry-run --validate-remote` closes most of that gap with read-only metadata fetches; workflow conditions and permission rules still only surface on the live submit.
- ADF strict mode is the default on mutation submit and `--dry-run` preview; reads / `--output=human` extract default to best-effort. Override per-call with `--adf-strict` / `--adf-best-effort` or globally via `JIRA_ADF_STRICT` env / `adf_strict` profile setting. Precedence: flag > env > profile > per-path default. See → `adf_reference`.
- `--dry-run` is local-only by default for `watchers add`; bare name/email won't resolve without `--validate-remote`.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `validation_error` (exit `3`) requesting `--force` | Real destructive write in non-TTY / agent context without `--force` | Re-run with `--force` (and `--delete-subtasks` if subtasks exist for `delete_issue`). |
| `validation_error` requesting interactive terminal | Bare `jira issue edit KEY` invoked under an agent harness | Pass `--summary` / `--assignee` / `--json-input` — see → `edit_issue`. |
| Dry-run was clean, submit returns `INVALID_INPUT (400)` | Project / type required a field the dry-run pipeline couldn't see (server-only rule) or ADF document contains nodes Jira rejects | Re-read the response, add the missing field to override, or re-encode the ADF. See → `adf_reference`. |
| `user_resolved: false` on `watchers add --dry-run` | `--user` was a bare name/email; local preview can't hit `/user/search` | Re-run with `--validate-remote` or pass `--user accountId:<id>`. |

**Next**
- Composes: → `clone_issue`, → `move_issue`, → `delete_issue`, → `edit_issue`, → `add_comment`, → `link_issues`, → `add_weblink`, → `attach_file`, → `manage_watchers`, → `log_work` — every mutating workflow wraps in this discipline.
- See also: → `core_contract` for the `--no-input` / agent-detection envelope and exit-code taxonomy.
