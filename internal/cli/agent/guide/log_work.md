## log_work
Goal: Record time spent against a Jira issue using durations that resolve via the profile's `workday_seconds`, and read the typed worklog list back.
When: time spent on an issue must be recorded against its worklog for sprint reporting, billing, or capacity tracking.

**Decide**

# duration
- `--time-spent <duration>` — accepts `1d 2h 30m`-style strings; `1d` resolves via the per-profile `workday_seconds` (default 28,800 = 8h).

# optional metadata
- `--started <RFC3339>` — backdate or pin a start time; otherwise Jira stamps "now".
- `--markdown "<text>"` — short worklog comment; markdown is lossy (see → `adf_reference`).
- `--json-input <file>` — full payload, including ADF-bodied comment, for the lossless path.

**Run**
- Quick log: `jira worklog add KEY --time-spent 1h30m --output=json`
- Bulk log: `jira worklog add <PROJECT_KEY>-1..10 -p 4 --time-spent 15m --markdown "triage" --output=json`
- Backdated: `jira worklog add KEY --time-spent 2h --started 2026-05-04T09:00:00.000+0000 --output=json`
- With markdown comment: `jira worklog add KEY --time-spent 45m --markdown "fixed bug X" --output=json`
- Full payload (ADF comment supported): `jira worklog add KEY --json-input wl.json --output=json`
- Read back: `jira worklog list KEY --output=json`
- Multi-key read back: `jira worklog list <PROJECT_KEY>-1..10 -p 4 --output=json`

**Save**
> Requires `--output=json`.
- `worklog add` returns the persisted entry envelope; capture the entry id for follow-up edits. Multi-key `worklog add KEY... -p N` returns ordered `data.results[]`.
- `worklog list KEY` returns the worklog list as the typed envelope — iterate to compute totals or surface recent entries.
- Multi-key `worklog list KEY...` returns `data.results[]`, ordered by requested key; each successful entry has `data.issue` and `data.worklogs`.

**Preconditions**
- `1d` resolution depends on the active profile's `workday_seconds`; a profile configured for 6h days will resolve `1d` to 21,600 seconds, not 28,800.
- ADF worklog comments must satisfy → `adf_reference`.

**Behavior**
- Durations are concatenable (`1d 2h 30m`); whitespace is permitted between units.
- `--markdown` and the `comment` field inside `--json-input` are mutually exclusive on the same call — pick one body path.
- `-p` / `--parallelism` is bounded to 1..16 and affects multi-key worklog add/list.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, `code: flag_value_invalid` on `--time-spent` | Duration string didn't parse (typo, unknown unit) | Re-run with a `d/h/m/s` combination |
| Unexpected `1d` totals | Profile `workday_seconds` differs from 8h | Check active profile (see → `auth_setup`) and recompute |

**Next**
- Then: → `read_issue` (worklog totals influence the issue summary).
- Composes: → `add_comment` when the worklog comment alone isn't enough context.
