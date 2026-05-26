## identity_setup
Goal: Resolve identity once per profile and switch profile per call so `--assignee me`, the TUI `A` key, and "in my epics" JQL all work without further setup.

**Decide**
- First time on a profile or after a credential rotation: persist `account_id` with `jira auth whoami --save`.
- Quick check from cached state: `jira me`.
- List every configured profile and see which is active: `jira config profile`.
- Per-call profile switch: `--profile <name>` on any command.
- Default profile: set via `default_profile = "..."` in `~/.config/jira-cli/config.toml`. `JIRA_PROFILE` is **not** read — config is the source of truth.

**Run**
- Resolve + persist: `jira auth whoami --save`
- Identity card (cached): `jira me --output=json`
- List profiles: `jira config profile --output=json`
- Credential health per profile: `jira auth status --output=json`
- Per-call switch: `jira <cmd> --profile work --output=json`

**Save**
> Requires `--output=json`.
- `data.account_id` [string, required after `whoami --save`] — feed to `--assignee accountId:<id>` and to ADF `mention.attrs.id`.
- `data.profile` [string, required on `me` / `auth status`] — confirms which profile resolved.

**Behavior**
- `jira auth whoami --save` calls `/rest/api/3/myself` and persists the resolved `account_id` to the active profile's TOML entry. After this, `--assignee me`, the TUI `A` key, and "in my epics" JQL all work without further setup.
- `--profile` overrides per call; the environment variable `JIRA_PROFILE` is deliberately NOT consulted.
- Default profile is whatever `default_profile = "..."` in `~/.config/jira-cli/config.toml` names.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `--assignee me` fails to resolve | `account_id` not persisted on this profile | Re-run `jira auth whoami --save` |
| Exit 1 on `whoami` | Credential missing / expired | → `auth_setup` |
| `jira me` shows the wrong profile | Default profile points elsewhere | `jira config profile`, then add `--profile <name>` per call or update `default_profile` in `config.toml` |

**Next**
- Then: → `inspect_schema` (introspect command tree and field encoders before authoring payloads)
- Then: → `auth_setup` (when health checks fail or a profile is missing)
- Composes: → every read/write workflow inherits the resolved profile.
