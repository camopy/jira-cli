# Jira CLI Agent Guide

Comprehensive steering document for AI coding assistants and developers
using `jira-cli`. This file is embedded in the binary and surfaced via
`jira agent guide`. Pair with `jira agent schema --output=compact` for
the machine-readable command tree, `jira agent adf-matrix --output=json`
for the ADF support matrix, and `jira agent fieldtypes --output=json`
for the customfield encoder registry.

The guide assumes you are interacting through the JSON envelope path
(`--output=json` or auto-detected agent mode); flags and recipes still
apply to TTY humans, the output rendering differs.

### How to read this guide

Each workflow below is a goal-oriented runbook. The layout is consistent:

- **Goal** — one sentence on the outcome.
- **Decide** — which flags / inputs to pick.
- **Run** — canonical command shapes.
- **Save** — the JSON fields you carry forward (requires `--output=json`).
- **Preconditions** — non-obvious constraints.
- **Behavior** — runtime quirks worth knowing.
- **Recover** — symptom → cause → next, including the exit code or error code.
- **Next** — which workflows naturally follow; `→ ` + a backticked slug
  points at another runbook in this guide.

Run `jira agent guide <slug>` to print a single workflow without the
rest of the file. The slug is the lowercased heading with `_` replaced
by `-` (so `## core_contract` is reachable as `core_contract` or
`core-contract`).

### Workflows

Cross-cutting:

- `core_contract` — output modes, envelope shape, exit codes, headless writes, pagination, read-only mode, debug logging.
- `identity_setup` — resolve and persist the active profile's account id.
- `auth_setup` — wire a profile + credential through one of the supported backends.
- `inspect_schema` — agent-facing schema / guide / ADF matrix / field-types surfaces.
- `configure_editor` — editor resolution for the bare-form `issue edit`.
- `safe_mutation` — the cross-cutting `--dry-run` / `--force` / `--no-input` contract every mutating workflow inherits.

Read:

- `read_issue` — `jira issue view KEY`.
- `list_issues` — `jira issue list`, board filtering, `--detail` vs `--full`.
- `search_jql` — `jira search jql`, `jira search saved`, `jira jql build`.

Discover:

- `discover_board` — `jira boards list` and the board cache primer.
- `cache_metadata` — per-profile JSON cache for labels / projects / fields / issuetypes / linktypes / boards / epics.

Create / mutate issues:

- `create_issue` — native ADF + alias-driven create.
- `create_subtask` — `issue_type: "Subtask"` + `parent.key`.
- `edit_issue` — field flags or `--json-input` (no editor under agent).
- `transition_issue` — list, then execute a workflow transition.

Side-channel writes:

- `add_comment` — `jira issue comment` (ADF preferred).
- `list_comments` — read / paginate / delete comments.
- `attach_file` — `jira issue attachment` add / list / download / delete.
- `manage_watchers` — `jira issue watchers` add / remove / list.
- `link_issues` — `jira issue link` (direction-aware), plus the link-type cache.
- `add_weblink` — remote URL attachments via `jira issue weblink`.
- `log_work` — `jira worklog add`.

Destructive:

- `clone_issue` — GET → sanitize → POST.
- `move_issue` — swap project / issuetype in place.
- `delete_issue` — `--force`-gated delete, with `--delete-subtasks` for parents.

Reference (not workflows — lookup material):

- `adf_reference` — ADF node / mark catalogue, gotchas, strict-vs-best-effort.
- `jql_reference` — JQL operators, keywords, functions, recipes.

## core_contract
Goal: Apply the cross-cutting output-mode, envelope, exit-code, headless, pagination, read-only, and debug contract every other workflow inherits.

**Decide**

# output mode
- TTY humans: `--output=human` (or omit; `auto` picks `human` on a TTY).
- Automation: `--output=json` (full envelope) for parsing; `--output=compact` for the JSON `data` payload only, one line, jq-friendly.
- Agent harnesses: `auto` resolves to `compact` when an agent env var is set.
- There is no raw REST passthrough — `json` and `compact` cover every machine-consumption need.

# headless writes (`--no-input`)
- Set `--no-input` on every mutation when scripting. Under it the CLI never prompts, never reads stdin implicitly, and never silently no-ops.
- Only `--json-input -` (command payload) and `--secret-stdin` (auth secret) may read stdin under `--no-input`.
- Destructive ops (`delete` / `clone` / `move` / `comment delete` / `link delete` / `attachment delete`) still require `--force` on top of `--no-input` → see `safe_mutation`.

# pagination
- Single page (default page size): omit `--limit` / `--all`.
- Custom page size: `--limit N`.
- Walk every page until `is_last=true`: `--all` (`issue comment list`, `issue attachment list`).
- Board cache drain: `jira boards list --unbounded` removes the default 100-page / 10 000-board safety bound.

# read-only mode
- Block every mutation at the HTTP layer for the invocation: `JIRA_READ_ONLY=true jira <anything>`.
- Block per profile in `config.toml`:
  ```toml
  [[profiles]]
    name = "agent-handoff"
    read_only = true
  ```
- Env wins on the OFF→ON direction — once `JIRA_READ_ONLY` is set, every profile is read-only regardless of its own setting.

# debug
- HTTP request/response dumps to stderr with secrets redacted: `--debug` (or `-d`).

**Run**
- Canonical write: `jira <cmd> --no-input --json-input payload.json --output=json`
- Pagination drain: `jira issue comment list KEY --all --output=json`
- Per-call read-only: `JIRA_READ_ONLY=true jira issue edit KAN-1 --summary "x" --no-input --output=json`
- Debug capture: `jira issue edit KAN-1 --json-input fields.json --no-input --debug 2>&1 | grep '^DBG'`

**Save**
> Requires `--output=json`.
- `ok` [bool, required] — `true` on success, `false` on failure.
- `meta.command` [string, required] — dotted command path (e.g. `issue.create`).
- `meta.timestamp` [string, required] — ISO 8601 UTC.
- `meta.request_id` [string, required] — request correlation id.
- `meta.exit_code` [int, present on failure] — mirrors the process exit code; `data` is `null` when set.
- `meta.pagination` [object, optional] — `startAt`, `maxResults`, `total`, `isLast`; walk until `isLast=true`.
- `data` [object, required on success] — command-specific payload; `null` on failure.
- `errors[]` [array, required] — each entry carries `type`, `code` (stable snake_case — branch on this, never on `message`), `message`, `hint`, `retryable`. Optional fields when relevant: `flag`, `field`, `http_status`, `retry_after_seconds`, `provider`, `upstream_code`, `upstream_status`. Jira API errors leave `upstream_code` empty — Jira exposes no stable machine error code.
- `warnings[]` [array, required] — non-fatal diagnostics; never blank on a successful command that degraded.

**Behavior**
- Envelope shape (success):
  ```json
  {
    "ok": true,
    "meta": {
      "command": "issue.create",
      "timestamp": "2026-05-04T22:48:55Z",
      "request_id": "...",
      "pagination": { "startAt": 0, "maxResults": 50, "total": 12, "isLast": true }
    },
    "data":     { /* command-specific */ },
    "errors":   [],
    "warnings": []
  }
  ```
- `--output=auto` resolution table:

  | Context           | Resolved mode                                                |
  |-------------------|--------------------------------------------------------------|
  | TTY human         | `human`  (clog rich text)                                    |
  | Non-TTY (pipe)    | `json`  (full envelope)                                      |
  | Detected agent    | `compact`  (envelope `data` only, single line, jq-friendly)  |

- Agent env detection (first match wins, fixed precedence amp → codex → gemini → copilot → opencode → cursor → claude): `AGENT=amp`, `CODEX_SANDBOX`, `CODEX_CI`, `CODEX_THREAD_ID`, `CODEX`, `OPENAI_CODEX`, `GEMINI_CLI`, `COPILOT_CLI`, `COPILOT`, `GITHUB_COPILOT`, `OPENCODE`, `CURSOR_TERMINAL`, `CURSOR_AGENT`, `CLAUDECODE`, `CLAUDE_CODE`. `AI_AGENT=<name>` is the explicit override.
- `--output=compact` strips `meta` and `errors` on success. **Error paths still emit the full envelope** so failures stay parseable regardless of mode flags.
- Machine envelopes never carry `meta.profile` — a command that reports a profile puts it in command-specific `data`.
- `--output` value table:

  | `--output` value | Effect                                                                         |
  |------------------|--------------------------------------------------------------------------------|
  | `auto`           | Detect: TTY → human, pipe → json, agent → compact                              |
  | `human`          | Force human-friendly clog rich text                                            |
  | `json`           | Force the full structured envelope on stdout                                   |
  | `compact`        | Force the JSON `data` payload only — no `ok`/`meta`/`warnings`/`errors`        |

- In `--output=human` mode, `warnings[]` mirrors to stderr as clog `WRN` lines so stdout stays clean for piping.
- Warning `type` values you'll see:

  | `type`                       | Meaning                                                                        |
  |------------------------------|--------------------------------------------------------------------------------|
  | `unknown_adf_node`           | ADF node outside the MVP set, preserved opaquely                               |
  | `unknown_adf_mark`           | ADF mark outside the MVP set, preserved opaquely                               |
  | `customfield_unknown_type`   | `customfield_NNNN` key forwarded with no registry schema (Jira handles type)   |
  | `lossy_adf_conversion`       | markdown → ADF or roundtrip dropped detail                                     |
  | `cache-truncated`            | Cache primer hit its page/row safety bound; data fields include `truncated`/`truncated_reason` |
  | `adf-lossy-comment`          | A comment body contains constructs lost on render; entry names `comment_id` and `lossy_constructs[]` |

- Exit codes (stable contract — never reused for new categories):

  | Exit | Meaning                                          |
  |------|--------------------------------------------------|
  | 0    | Success                                          |
  | 1    | Authentication failure                           |
  | 2    | Not found                                        |
  | 3    | Validation error / mutex flags / no-input gap    |
  | 4    | Rate limited                                     |
  | 5    | Server error                                     |

- `--debug` redacts `Authorization`, `Cookie`, and `X-Atlassian-Token` headers to `REDACTED` in the dumped traffic. `Atl-Traceid` is preserved — quote it on Atlassian support tickets.
- Bounded drains (boards cache, etc.) emit a `cache-truncated` warning naming the bound that fired, and surface `truncated` / `truncated_reason` in `data`.
- Read-only blocks happen at the HTTP transport layer — there is no per-command boilerplate to forget.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit 1 | Authentication failure | → `auth_setup` |
| Exit 2 | Not found (issue key, attachment id, link id, profile, etc.) | Re-resolve the identifier; check `auth_setup` if the resource should exist |
| Exit 3, `code=flag_unknown` | Unrecognized flag (often a removed legacy boolean like `--json`/`--compact`/`--plain`/`--raw`) | Drop the flag; use `--output=json|compact` |
| Exit 3, `code=flag_value_missing` | A flag that needs a value was given none | Supply the value |
| Exit 3, `code=flag_value_invalid` | Flag value failed type or range parsing | Match the documented value set |
| Exit 3, `code=flag_syntax_invalid` | Malformed flag token | Re-quote / re-escape the argv |
| Exit 3, `code=required_flag_missing` | A required flag was not set; `flag` names the first one | Supply that flag |
| Exit 3, `code=arg_count_invalid` | Wrong number of positional arguments | Match the documented arity |
| Exit 3, `code=command_unknown` | Unrecognized command; `hint` may carry `Did you mean <name>?` | Use the suggested name |
| Exit 3, `read-only mode is active` | `JIRA_READ_ONLY=true` or profile `read_only = true` | Unset / flip the profile, or do the work elsewhere |
| Exit 3 under `--no-input` with no field flags | Empty headless edit | Pass at least `--summary` / `--assignee` / `--json-input` → `edit_issue` |
| Exit 4 | Rate limited; `errors[0].retry_after_seconds` set when known | Back off then retry |
| Exit 5 | Server error (Jira 5xx or a 413 upload-size cap); `errors[0].message` carries the upstream text | Retry if transient; for 413 split or shrink the upload |

**Next**
- Then: → `identity_setup` (resolve `account_id` once per profile so `--assignee me` works)
- Then: → `auth_setup` (when exit 1)
- Then: → `inspect_schema` (when you need the machine-readable command tree, ADF matrix, or customfield registry)
- Composes: → every other workflow inherits this contract.

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

## auth_setup
Goal: Wire a Jira profile to a valid credential — backend chosen, profile populated, secret never on disk in plaintext — before any other workflow can call Jira.

**Decide**

# backend

| Backend | Pick when |
|---------|-----------|
| **Env var** | CI / containers / ephemeral runners. `JIRA_TOKEN_<PROFILE>` overrides stored credentials for that profile. |
| **Configured backend lookup** | Normal profile usage. `secret_backend = "keyring"` reads the OS keyring; `secret_backend = "1password"` reads the SDK-backed 1Password store. |
| **OS keyring** (default) | Single workstation, zero extra setup, OS provides Secret Service / Keychain / Credential Manager. |
| **1Password (Go SDK)** | Team uses 1Password and you have `OP_SERVICE_ACCOUNT_TOKEN` or a CGO-enabled source build with desktop app integration. SDK-only — does NOT shell out to the `op` CLI. |

Resolution order: the environment override is checked first; if unset, the profile's configured backend (`secret_backend = "keyring"` or `"1password"`) is used.

# auth type
- One of `token`, `basic`, `pat`, `mtls`. Anything else returns exit 3 — no fake authenticated profile is stored.

# command shape
- First-time TTY: bare `jira auth login` walks profile name → base URL → email → auth type → backend → credential prompt (reads stdin without echoing).
- Headless (CI / agent): `--no-input` with explicit flags for every field. Secret feeds via `--secret-stdin` (keyring) or `--vault` + `--item` (1Password).

# guard
- Partial flags **merge** into the existing profile — fields not supplied retain their current values. To replace cleanly, pass every field.

**Run**
- Preflight: `jira auth status --output=json`
- Interactive (TTY): `jira auth login`
- Headless, keyring backend:
  ```sh
  echo "$JIRA_TOKEN" | jira auth login --no-input \
    --profile-name work \
    --base-url https://company.atlassian.net \
    --email dev@example.com \
    --auth-type token \
    --backend keyring \
    --secret-stdin
  ```
- Headless, 1Password backend:
  ```sh
  jira auth login --no-input \
    --profile-name work \
    --base-url https://company.atlassian.net \
    --email dev@example.com \
    --auth-type token \
    --backend 1password \
    --vault Engineering \
    --item jira-cli-work
  ```
- Switch active profile: `jira auth switch <profile>`
- Re-resolve credential from backend: `jira auth refresh`
- Move credential between backends: `jira auth migrate --backend 1password`
- Remove credential (keeps TOML metadata): `jira auth logout <profile>`
- Redacted token diagnostics (length, prefix, backend — never the raw token): `jira auth token --output=json`
- Verify post-login: `jira auth whoami --save` then `jira me`.

**Save**
> Requires `--output=json`.
- `data.profile` [string, required] — confirms which profile was wired.
- `data.backend` [string, required on `auth token` / `auth status`] — `keyring`, `1password`, or `env`.
- `data.account_id` [string, required after `whoami --save`] — feed to `identity_setup` consumers.

**Preconditions**
- The TOML config never holds the secret — only metadata (backend selector, vault, item ref). Anything that calls Jira goes through the same HTTP redactor.
- For 1Password desktop-app auth: 1Password must be installed, signed in to the account that owns the item, and configured to allow SDK integrations. In the 1Password app, open Settings > Developer and enable **Integrate with other apps**. For biometric approval, also enable the OS unlock option under Settings > Security.
- For 1Password desktop-app integration, see Further reading below.

Further reading:

- [1Password SDK desktop app integration](https://www.1password.dev/sdks#1password-desktop-app)

**Behavior**
- `auth login --no-input` with **partial flags merges** into the existing profile. This protects against mistyped one-flag updates wiping unrelated fields like `email` or `account_id`. To replace cleanly, pass every field.
- Auth types accepted: `token`, `basic`, `pat`, `mtls`. Anything else returns exit 3.
- Secret hygiene contract (HTTP-transport-level — enforced once, not per command):
  - Secrets are **never** stored in the TOML config — only metadata (backend selector, vault, item ref).
  - All logging, including `--debug`, redacts `Authorization` headers and any field named `secret` / `token` / `api_token` / `cookie`.
  - CLI-written files containing credential metadata are mode `0600`.
  - `jira auth token` deliberately does NOT print the raw token — only length, prefix, and backend identity.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit 1 on every call | Credential missing or expired | `jira auth status` → identify failing backend → `jira auth login --profile-name <name>` |
| `unsupported auth type "X"` | Typo in `--auth-type` | Use one of `token`, `basic`, `pat`, `mtls` |
| `credential not found` | Backend has no entry for this profile | `jira auth login --profile-name <name>` |
| `OP_SERVICE_ACCOUNT_TOKEN not set` | 1Password service-account env missing | Export it, or fall back to keyring backend via `jira auth migrate --backend keyring` |
| 401 on a previously-working profile | Token revoked / rotated | `jira auth login --profile-name <name>` to replace |

**Next**
- Then: → `identity_setup` (run `jira auth whoami --save` to persist `account_id`)
- Then: → `configure_editor` (for the `jira issue edit` TTY flow, if relevant)
- Composes: → every other workflow inherits the active credential.

## inspect_schema
Goal: Introspect the CLI's command tree, JSON output schemas, ADF support set, and customfield encoders without reading prose, so payload authoring is grounded in the binary's own contract.

**Decide**
- Need the full command tree + flag signatures + per-command JSON output schemas: `jira agent schema`.
- Need this guide as text (or a specific section by slug): `jira agent guide [<section>]`.
- Need to know which ADF nodes/marks the CLI authors, renders, preserves, validates, or submits: `jira agent adf-matrix --output=json`.
- Need to know which `customfield_NNNN` keys the encoder registry knows and what input shape they expect: `jira agent fieldtypes --output=json`.

**Run**
- Command tree (compact, jq-friendly): `jira agent schema --output=compact`
- This guide (full): `jira agent guide`
- This guide (one section): `jira agent guide <slug>` — e.g. `jira agent guide jql_reference`
- ADF support matrix: `jira agent adf-matrix --output=json`
- Customfield encoder registry: `jira agent fieldtypes --output=json`

**Save**
> Requires `--output=json` (or `--output=compact`).
- `data[].kind` [string, required] — `node`, `mark`, or `field-type`.
- `data[].name` [string, required] — e.g. `paragraph`, `strong`, `customfield_10010`.
- `data[].status` [string, required] — `mvp` or `preserve-only`.
- `data[].capabilities` [object, required] — booleans for `author`, `render`, `preserve`, `validate`, `submit`.
- `data[].input_shape` [object, required] — JSON Schema 2020-12 fragment for what the CLI accepts.
- `data[].output_shape` [object, required] — JSON Schema 2020-12 fragment for what the CLI returns.
- `data[].warnings` [array, required] — known degradation cases for this entry.
- `data[].official_url` [string, optional] — Atlassian docs page for the node/mark/field.
- `data[].notes` [string, optional] — free-form caveats.
- `data[].submit_description` [string, optional] — how this entry behaves on a mutation submit (e.g. "ADF: included in a Jira rich-text field payload after ADF validation passes.").

**Behavior**
- `adf-matrix --output=json` and `fieldtypes --output=json` emit arrays of the **same envelope shape** — a single agent parser handles both surfaces:
  ```json
  {
    "kind": "node|mark|field-type",
    "name": "paragraph",
    "status": "mvp|preserve-only",
    "capabilities": { "author": true, "render": true, "preserve": true, "validate": true, "submit": true },
    "input_shape": { /* JSON Schema 2020-12 fragment */ },
    "output_shape": { /* JSON Schema 2020-12 fragment */ },
    "warnings": [],
    "official_url": "https://developer.atlassian.com/...",
    "notes": "...",
    "submit_description": "ADF: included in a Jira rich-text field payload after ADF validation passes."
  }
  ```
- `agent guide <section>` accepts the slugs used throughout this guide — handy when you want a focused subset rather than the whole text.
- Use `agent fieldtypes` to learn what shape `customfield_NNNN` expects before authoring it; pair with the cache primer (`jira cache fields`) to map names ↔ ids on the live instance.

**Next**
- Then: → `auth_setup` (if `agent schema` reveals a command you need but the profile is unconfigured)
- Then: → `cache_metadata` (prime `fields` / `projects` / `issuetypes` for live id ↔ name mapping)
- Composes: → any write workflow (`create_issue`, `edit_issue`, `add_comment`) consumes the schemas to validate payloads before submit.

## configure_editor
Goal: Pin the editor used by the kubectl-style `jira issue edit KEY` flow so TTY edits land in a tool that blocks until you save, instead of forking and losing your change.

**Decide**
- Used only for the bare `jira issue edit KEY` form (no field flags). Agents must pass `--summary`, `--assignee`, or `--json-input` instead → see `edit_issue`.
- Resolution chain (highest precedence first):
  1. `JIRA_EDITOR` env var
  2. Per-profile `editor` field in TOML
  3. Global `editor` field in TOML
  4. `EDITOR` env var
  5. `VISUAL` env var
  6. `vi` (last-resort fallback)
- Editors that fork-and-return (e.g. `code` without `--wait`) are refused at spawn time with a one-line fix — silent strikethrough-and-data-loss is gone.

**Run**
- Per-invocation override: `JIRA_EDITOR='code --wait' jira issue edit KAN-1`
- Global default in `~/.config/jira-cli/config.toml`:
  ```toml
  editor = "code -w"
  ```
- Per-profile override (wins over the global on that profile):
  ```toml
  [[profiles]]
    name = "default"
    editor = "vim"
  ```

**Preconditions**
- Editor command must block until the file is closed (`--wait` / `-w` on VS Code; `vim` / `nvim` block by default). A forking editor is refused at spawn with `set EDITOR='code --wait'`-style remediation.
- Only relevant on a TTY — the bare `jira issue edit KEY` form is refused in agent / non-TTY context with exit 3.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Spawn-time refusal naming `--wait` | Editor command forks and returns immediately | Set `EDITOR='code --wait'` (or `JIRA_EDITOR='code --wait'`) and retry |
| Exit 3, "issue edit requires an interactive terminal for the editor flow" | Running under an agent / non-TTY context | Pass `--summary`, `--assignee`, or `--json-input` instead → `edit_issue` |
| Edits open in `vi` unexpectedly | No editor configured at any precedence level | Set `JIRA_EDITOR`, `editor` in TOML, or `EDITOR`/`VISUAL` — pick the slot that matches your workflow |

**Next**
- Then: → `edit_issue` (single-shot field edits or bulk `--json-input` for agent / non-TTY contexts)
- Composes: → `auth_setup` (editor config is per-profile so it lives next to the credential metadata).

## safe_mutation
Goal: Wrap every destructive or state-changing Jira call with the right confirmation + preview discipline so an agent never blows away data it can't recover.

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
- Full validation pipeline (parse → ADF compat → field schema → customfield encoding, stops before the API call): `issue create`, `issue edit`, `issue clone`, `issue move`, `issue comment`. These dry-runs catch payload shape, ADF strict errors, missing required customfields, and unknown field names.
- Local-only preview (does not contact Jira): `watchers add --dry-run` when `--user` is locally derivable (`accountId:<id>`, or `me` on a profile that carries an account id), `issue link --dry-run`, `worklog add --dry-run`.
- Hybrid (local preview by default, opts into a read-only resolve when asked): `watchers add --dry-run --validate-remote` does a read-only `/user/search` to resolve a bare name or email but still issues no watcher POST/DELETE.

**Run** (sequence, per mutation)
1. Dry-run: same command with `--dry-run --output=json`; verify payload shape, ADF validity, and any `*_resolved` flags before committing.
2. Real write: drop `--dry-run`, keep `--output=json`, add the target's confirmation flag (`--force` for `clone_issue` / `move_issue` / `delete_issue` / attachment delete / link delete; `--no-input` + field flags or `--json-input` for `edit_issue` and `add_comment`).
3. Record the returned issue key, comment id, link id, worklog id, or attachment id from `data.*` as the evidence trail.

**Preconditions**
- Always pass `--output=json` for automation — never parse `--output=human` (it's display-only).
- `edit_issue` in agent context refuses the bare `jira issue edit KEY` form (exit `3`) because the editor flow needs a TTY; pass `--summary`, `--assignee`, or `--json-input`.
- `--no-input` requires at least one field on `edit_issue` — empty edits exit `3`.

**Behavior**
- `--dry-run` on full-pipeline commands runs every local validation stage but stops before the network call; a clean dry-run means the payload is shaped correctly, not that Jira will accept it (server-side rules like project-required customfields still apply on submit).
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


## read_issue
Goal: Fetch one issue's typed JSON envelope so downstream workflows have a stable shape to parse.

**Decide**
- Single issue, known key: `jira issue view KEY`.
- Need the comment thread, attachments, links, or worklog for the same key? → `list_comments`, → `attach_file`, → `link_issues`, → `log_work` instead — `view` does not project those collections.

**Run**
- Canonical: `jira issue view KEY --output=json`

**Save**
> Requires `--output=json`.
- `data` [object, required] — the typed issue envelope (`key`, `summary`, `status`, `assignee`, `priority`, `updated`, plus other projected fields).

**Behavior**
- The typed projection covers the common fields. A few fields are not yet mapped into the envelope shape and there is no raw REST passthrough mode to recover them — closing the gap means extending the typed projection.

| Field                              | Typed JSON     |
|------------------------------------|----------------|
| `parent`                           | not projected  |
| `subtasks`                         | not projected  |
| `issuetype.name` on `issue view`   | may be `null`  |

- Because `subtasks` is not projected, there is no CLI-side verification of the subtask list after a → `create_subtask` call.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `2` (`not_found`) | Wrong key, or the active profile cannot see this issue | Verify the key with → `search_jql` or → `list_issues` under the right project/profile |
| `parent` / `subtasks` absent from JSON | Known typed-output gap (not an error) | Treat as unavailable from the CLI for now; do not assume "no subtasks" |
| `issuetype.name` is `null` | Known typed-output gap on some issue types | Cross-check with → `list_issues` (richer projection per row) |

**Next**
- Then: → `list_comments` to read the discussion thread on the same key.
- Then: → `transition_issue` to advance workflow state.
- Then: → `edit_issue` to patch fields in place.
- Alternative: → `list_issues` or → `search_jql` when you don't already have the key.

## list_issues
Goal: Page through issues for the active profile — by JQL, by board, or by the default project — capturing keys for downstream per-issue work.

**Decide**

# target
- Default profile filter (whatever `default_project` / `default_board` resolves to): `jira issue list`.
- Ad-hoc JQL: `--jql 'JQL'`.
- Show the JQL that WOULD run without calling Jira: `--as-jql` (local-only preview, no API call).
- Restrict to one agile board: `--board <NAME>` (exact case-insensitive) or `--board-id <id>` (numeric escape when names collide).

# field set
- Default summary set per row: `key, summary, status, assignee, priority, updated`.
- Full field records for every row in the page: `--detail` (this flag is `issue list` only — `search jql` / `search saved` don't accept it).
- Wire-shape `fields:["*all"]`: `--full`.
- Explicit selector: `--fields key,summary,customfield_10010`.

# guard
- `--board` and `--board-id` are mutually exclusive — passing both exits 3.
- Explicit `--board ""` suppresses the configured `default_board` for one invocation.

**Run**
- Default: `jira issue list --output=json`
- With JQL: `jira issue list --jql 'project = KAN AND statusCategory != Done' --output=json`
- Preview JQL only: `jira issue list --as-jql --output=json`
- Full field records: `jira issue list --detail --output=json`
- Board filter (name): `jira issue list --board "Engineering Sprint" --output=json`
- Board filter (id): `jira issue list --board-id 42 --output=json`

**Save**
> Requires `--output=json`.
- `data.issues[].key` [string, required] — feed to → `read_issue`, → `edit_issue`, → `transition_issue`, → `add_comment`, etc.
- `data.issues[]` [object array] — summary set fields by default; full records under `--detail`.
- `meta.pagination.startAt` / `.maxResults` / `.total` / `.isLast` [int / int / int / bool] — paginate until `isLast=true`.

**Preconditions**
- `--board NAME` requires a primed boards cache. Empty cache → exit 3 with `boards cache is empty — run "jira cache boards"`. See → `cache_metadata` for the prime command and → `discover_board` for resolution semantics.
- `--board` resolution is **exact case-insensitive only** — no substring fallback. Ambiguous matches exit 3 with structured `candidates[]`; pass `--board-id` to disambiguate.

**Behavior**
- Board filtering emits `project in (P1, P2, …)` JQL built from the board's cached project keys — the board is not a server-side filter, it expands locally to a project list.
- `default_board` (profile config, see below) applies implicitly to `issue list` whenever `--board`/`--board-id` is omitted. The flag wins over the default; the default wins exclusively over `default_project` on commands that consume `--board` (no intersection, no union).
- `default_board` is validated **at use-time only** — `config set` accepts any string without checking the cache (which may not exist yet). When the configured `default_board` doesn't resolve, you get `default_board "X" not found in boards cache — run "jira cache boards --refresh" or unset with "jira config set profiles.<profile>.default_board ''"`.

# `default_board` profile config
- Set: `jira config set profiles.default.default_board "Engineering Sprint"`
- Inspect: `jira config get profiles.default.default_board`
- Unset: `jira config set profiles.default.default_board ""`

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `3`, `boards cache is empty` | First-time board use without a primed cache | → `cache_metadata` (run `jira cache boards`) then retry |
| Exit `3`, `candidates[]` of board matches | Ambiguous `--board NAME` across projects | Re-run with `--board-id <id>` from the candidates list |
| Exit `3`, both `--board` and `--board-id` set | Mutex violation | Pass exactly one |
| Exit `3`, `default_board "X" not found in boards cache` | Stale or missing cache vs configured default | `jira cache boards --refresh`, or unset the default |

**Next**
- Then: → `read_issue` on any captured `key` for the full typed envelope.
- Then: → `edit_issue`, → `transition_issue`, → `add_comment` on captured keys.
- Alternative: → `search_jql` for direct JQL or saved-query execution.
- Composes: → `discover_board` to enumerate boards and → `cache_metadata` to prime the boards cache before `--board` use.

## search_jql
Goal: Run a JQL query — hand-authored, flag-built, or saved on disk — and capture matching issue keys.

**Decide**

# query source
- Hand-authored string: `jira search jql 'JQL'`.
- File-saved query under `~/.config/jira-cli/queries/<name>.jql`: `jira search saved <name>`.
- Built from flags (no hand-quoting): `jira jql build <flags>` — emits the JQL string; pipe into `search jql` or use with `issue list --jql`.

# field set
- Default summary set per row.
- Wire-shape `fields:["*all"]`: `--full`.
- Explicit selector: `--fields key,summary,customfield_10010`.
- Note `--detail` is NOT accepted on `search jql` / `search saved` — that flag belongs to → `list_issues`.

# preview without calling Jira
- `jira issue list --as-jql --output=json` returns the builder output without an API call.

**Run**
- Hand JQL: `jira search jql 'assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC' --output=json`
- Saved query: `jira search saved my-open-bugs --output=json`
- Build then run:
  ```sh
  JQL=$(jira jql build --project KAN --assignee me --output=json | jq -r '.data.jql')
  jira search jql "$JQL" --output=json
  ```
- Builder examples:
  ```sh
  jira jql build --project KAN --status Done --assignee me --output=json
  # → {"jql": "project = KAN AND assignee = currentUser() AND status = Done ORDER BY updated DESC"}

  jira jql build --project KAN --label regression --label hotfix --type Bug --type Task --output=json
  # → {"jql": "project = KAN AND labels in (regression, hotfix) AND issuetype in (Bug, Task) ORDER BY updated DESC"}

  jira jql build --project KAN --order-by updated --desc --output=json
  # → {"jql": "project = KAN ORDER BY updated DESC"}
  ```

**Save**
> Requires `--output=json`.
- `data.issues[].key` [string, required] — feed to → `read_issue`, → `edit_issue`, → `transition_issue`, etc.
- `data.jql` [string, required on `jql build`] — the constructed JQL string; pipe to `search jql` or pass to `issue list --jql`.
- `meta.pagination.startAt` / `.maxResults` / `.total` / `.isLast` [int / int / int / bool] — paginate until `isLast=true`.

**Behavior**
- Builder flag translations (so you don't hand-quote):

| Flag                        | Translates to                          |
|-----------------------------|----------------------------------------|
| `--assignee me`             | `assignee = currentUser()`             |
| `--reporter me`             | `reporter = currentUser()`             |
| Repeated `--label X`        | `labels in (X, Y, Z)`                  |
| Repeated `--type X`         | `issuetype in (X, Y, Z)`               |
| Repeated `--status X`       | `status in (X, Y, Z)`                  |
| `--order-by F --desc`       | `ORDER BY F DESC` (default)            |
| `--order-by F --desc=false` | `ORDER BY F ASC`                       |
| no flags                    | `updated >= -365d ORDER BY updated DESC` |

- Saved queries live as files with optional YAML frontmatter:
  ```text
  ---
  name: my-open-bugs
  description: Bugs assigned to me, not done
  project: KAN
  ---
  project = KAN AND issuetype = Bug AND assignee = currentUser() AND statusCategory != Done
  ORDER BY priority DESC, updated DESC
  ```

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `3`, `invalid order-by field` | `--order-by` value not on the allow-list (or contains shell metachars like `'updated; DROP TABLE x'`) | Use a vetted field name; see → `jql_reference` |
| Exit `3` at flag-parse on `--label`/`--type`/`--status` | Unbalanced quotes in the value | Strip the bad quote before re-running |
| Exit `3`, Jira `400` on unknown function/field | Hand-authored JQL references a field/function this instance does not expose | Cross-check operators, keywords, functions in → `jql_reference` |
| Zero `data.issues[]` | Query is well-formed but matches nothing | Loosen the JQL; reconfirm `project`/`assignee` values |

**Next**
- Then: → `read_issue` on any captured key for the typed envelope.
- Then: → `edit_issue`, → `transition_issue`, → `add_comment` on captured keys.
- Alternative: → `list_issues` for the default-project / board-filtered convenience surface.
- Reference: → `jql_reference` for operators, keywords, functions, and high-yield recipes.

## discover_board
Goal: Enumerate the agile boards visible to the active profile so `--board` filters in → `list_issues` and `jira jql build` resolve to a known id.

**Decide**
- First time on this profile (cache empty): prime it with `jira cache boards`. `boards list` also primes transparently on the first run when the cache is empty.
- Already primed, just want the listing: `jira boards list`.
- Cache exists but you suspect it's stale (new board, renamed board): `jira boards list --refresh` to force a re-prime.
- Need to start over: `jira cache clear boards` drops the cache file; the next call re-primes.
- Very large instance hitting the default safety bound: `--unbounded` to disable.

**Run**
- Explicit prime: `jira cache boards --output=json`
- Listing (envelope or table): `jira boards list --output=json`
- Force refresh: `jira boards list --refresh --output=json`
- Drop the cache: `jira cache clear boards`
- Remove the pagination bound: `jira boards list --unbounded --output=json`

**Save**
> Requires `--output=json`.
- `data.boards[].id` [int, required] — pass as `--board-id <id>` on → `list_issues` (unambiguous escape).
- `data.boards[].name` [string, required] — pass as `--board NAME` on → `list_issues` (exact case-insensitive match only).
- `data.boards[].type` [string, required] — verbatim Jira value: `scrum`, `kanban`, `simple`, `agility`, or any future Atlassian board type (round-trips through the cache without modification).
- `data.boards[].project_keys[]` [string array, required] — the project keys the board expands to in `project in (...)` JQL.
- `data.cache_state` [string] — `fresh`, `missing`, `stale`, `malformed`, `refresh`, or `empty`.
- `data.cache_source_state` [string] — the cache state before any fetch this call performed.
- `data.cache_empty` [bool] — true when the fetched or cached board list is empty.
- `data.from_cache` [bool] — true when the response came from disk.
- `data.fetched_at` [string] — RFC3339 timestamp of the most recent fetch.
- `data.truncated` [bool] / `data.truncated_reason` [string] — set when the safety bound fired.
- `meta.pagination.total` / `.start_at` / `.max_results` / `.is_last` / `.next_page_token` — standard pagination.

Envelope shape:

```json
{
  "data": {
    "boards": [
      {"id": 42, "name": "Engineering Sprint", "type": "scrum",
       "project_keys": ["ENG", "PLAT"]}
    ],
    "pagination": {
      "total": 12, "start_at": 0, "max_results": 12,
      "is_last": true, "next_page_token": null
    },
    "from_cache": true,
    "fetched_at": "2026-05-06T18:30:00Z",
    "truncated": false,
    "truncated_reason": "",
    "cache_state": "fresh",
    "cache_source_state": "fresh",
    "cache_empty": false
  }
}
```

**Behavior**
- Cache primer paginates the full set with safety bounds (default 100 pages / 10 000 boards). Truncation emits a `cache-truncated` warning naming the bound that fired and sets `data.truncated` / `data.truncated_reason`.
- Tab completion on `--board` reads the same cache:
  ```text
  $ jira issue list --board <TAB>
  Engineering Sprint  (scrum, ENG, PLAT)
  Platform Roadmap    (kanban, PLAT)
  ```
- The `default_board` profile setting (see → `list_issues`) is also resolved against this cache.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit `3`, `boards cache is empty — run "jira cache boards"` | `--board NAME` used before priming | → `cache_metadata` then retry, or pass `--board-id` |
| Exit `3` with `candidates[]` listing matches | Ambiguous board name across projects | Re-run with `--board-id <id>` from the listed candidates (each entry carries `id`, `name`, `project_keys`) |
| `data.truncated=true`, `cache-truncated` warning | Default page / total bound fired on a very large instance | Re-run with `--unbounded` |
| `default_board "X" not found in boards cache` | Configured default doesn't resolve | `jira cache boards --refresh`, or unset with `jira config set profiles.<profile>.default_board ''` |

**Next**
- Then: → `list_issues` with `--board` or `--board-id`.
- Composes: → `cache_metadata` for the general cache-primer pattern (TTL, refresh, clear, concurrency).

## cache_metadata
Goal: Prime, refresh, or clear the per-profile local caches so repeated reads and client-side validation don't hit Jira every call.

**Decide**

# what to prime
- `labels` — autocomplete `--label` values and validate client-side without a round-trip.
- `projects` — validate `project_key` in payloads; list project options without GET-ing every issue.
- `epics` — set `parent.key` to an epic without listing issues to find one.
- `fields` — **required** before authoring custom-field values; this is how you discover `customfield_10010` is "Story Points" and what type it expects.
- `issuetypes` — validate `issue_type` in payloads; tells you which types are subtasks.
- `linktypes` — drives `--type` completion on `jira issue link` and pins the canonical names per instance.
- `boards` — drives `--board` completion; resolves names to project lists for the `project in (...)` JQL clause (see → `discover_board`).

# when to use cache vs live API
- Multiple writes / repeated reads in the same session → cache.
- One-shot read, or you specifically need fresh-from-server data (you just created a label and want to see it) → skip the cache, hit live.

# refresh signal
- Cache is **never auto-refreshed**. Force with `--refresh`, age-gate with `--ttl-minutes N`, or wipe with `jira cache clear`.

**Run**
- Per-resource prime: `jira cache labels --output=json`, `jira cache projects --output=json`, `jira cache epics --output=json`, `jira cache fields --output=json`, `jira cache issuetypes --output=json`, `jira cache linktypes --output=json`, `jira cache boards --output=json`.
- Force refresh: `jira cache fields --refresh --output=json`
- TTL gate (refetch if older than N minutes): `jira cache fields --ttl-minutes 5 --output=json`
- Wipe one: `jira cache clear labels`
- Wipe everything for the active profile: `jira cache clear`
- Recommended once-per-session prime for agents:
  ```sh
  jira cache fields     --refresh --output=json   # so you can map customfield_NNNN → name
  jira cache projects   --refresh --output=json   # so you can validate project keys
  jira cache issuetypes --refresh --output=json   # so you can validate issue_type
  ```
- Re-use without spending tokens on Jira:
  ```sh
  jira cache labels --output=json | jq -r '.data.labels[]'
  ```

**Save**
> Requires `--output=json`.
- `data.<resource>[]` [array, required] — the cached list (`data.labels[]`, `data.projects[]`, `data.fields[]`, `data.issuetypes[]`, `data.epics[]`, `data.link_types[]`, `data.boards[]`).
- `data.from_cache` [bool] — true when read from disk, false when this call hit Jira.
- `data.fetched_at` [string] — RFC3339 timestamp of the most recent fetch.
- `data.count` [int, where applicable] — number of items in the cached list.
- `data.cache_state` [string] — `fresh`, `missing`, `stale`, `malformed`, `refresh`, or `empty`.
- `data.cache_source_state` [string] — the cache state before any fetch this call performed.
- `data.cache_empty` [bool] — true when the fetched or cached resource list is empty.
- `data.profile` [string] — emitted on cache-primer envelopes to confirm which profile's cache was touched.

Envelope shape (using `linktypes` as an example):

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

**Preconditions**
- Per-profile cache lives under `${XDG_CACHE_HOME:-~/.cache}/jira-cli/<profile>/`. Each subcommand prints the data AND writes it to disk.
- `jira cache fields --output=json` is the canonical way to discover `customfield_NNNN` IDs on a Jira instance — agents should run this once per session before authoring custom-field values.

**Behavior**
- Refresh after these events:

| Event                                                | Refresh        |
|------------------------------------------------------|----------------|
| You just created / renamed / deleted a label         | `labels`       |
| You created / renamed / archived a project           | `projects`     |
| Admin added a new custom field or changed a schema   | `fields`       |
| Admin added / renamed / disabled an issue type       | `issuetypes`   |
| You created / closed an epic                         | `epics`        |
| First call of a fresh session (recommended for `fields`) | as needed   |
| You hit a "not found" on something you know exists   | the relevant resource — your cache is stale |

- Concurrency: both the config/site/profile cache namespace and the config TOML use atomic temp-file + rename writes. Two `jira` invocations running in parallel against the same profile will not corrupt each other's state — concurrent writes serialize cleanly at the filesystem level.
- The `boards` cache primer paginates with safety bounds; see → `discover_board` for `--unbounded` and truncation details.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `not found` on a label / project / type you know exists | Cache is stale | Re-run the relevant `jira cache <resource> --refresh` |
| `data.cache_empty=true` after a fresh prime | The instance genuinely has none of that resource for this profile | Verify with a different profile or via the Jira UI |
| `cache-truncated` warning on `boards` | Default 100-page / 10 000-board bound fired | See → `discover_board` (re-run with `--unbounded`) |
| Custom-field author errors referencing unknown `customfield_NNNN` | `fields` cache never primed this session | `jira cache fields --refresh --output=json`, then re-author |

**Next**
- Then: → `list_issues` and → `discover_board` consume the `boards` cache for `--board` filtering.
- Then: → `create_issue` and → `edit_issue` consume the `fields` / `projects` / `issuetypes` caches for client-side validation.
- Composes: → `core_contract` (cache envelopes follow the same `ok`/`meta`/`data` shape).

## create_issue
Goal: Create a Jira issue from a structured payload and capture the new key for follow-up mutations.

**Decide**

# target
- Project: `project_key` (alias) or `project.key` — required unless the profile carries a default.
- Type: `issue_type` (alias) or `issuetype.name` — required unless the profile carries a default.

# body
- Recommended: `--json-input payload.json` with native ADF for `description` (round-trips losslessly).
- Convenience one-shots: `--summary "..."` (optionally `--assignee me|none|<accountId>`) — bypasses `--json-input`.
- Lossy human shortcut: `description_markdown` in the payload (converted to ADF; GFM features beyond the supported set degrade).

# guard
- `--dry-run` runs every validation stage (parse → ADF compat → field schema → customfield encoding) but stops before the API call.
- `--adf-strict` rejects any lossy step with exit 3; `--adf-best-effort` degrades silently with warnings.

**Run**
- Canonical: `jira issue create --no-input --json-input payload.json --output=json`
- Stdin variant: `cat payload.json | jira issue create --no-input --json-input - --output=json`
- Quick one-shot: `jira issue create --no-input --summary "Refactor auth middleware" --assignee me --output=json`
- Preview only: `jira issue create --dry-run --no-input --json-input payload.json --output=json`

Minimal payload:

```json
{
  "summary": "Refactor auth middleware",
  "issue_type": "Task",
  "project_key": "KAN",
  "description": {
    "type": "doc", "version": 1, "content": [
      {"type": "paragraph", "content": [{"type": "text", "text": "Description body."}]}
    ]
  }
}
```

Richer payload (every key past the aliases is forwarded verbatim into Jira's `fields` object):

```json
{
  "labels": ["regression", "stress-test"],
  "priority": {"name": "Highest"},
  "duedate": "2026-06-01",
  "customfield_10015": "2026-05-15",
  "environment": {"type": "doc", "version": 1, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "production / linux-amd64"}]}]},
  "components": [{"name": "ui"}],
  "fixVersions": [{"name": "1.1.0"}],
  "assignee_account_id": "712020:ff38cf6b-faa6-42ae-aa4b-20a2108cfc0f"
}
```

**Save**
> Requires `--output=json`.
- `data.key` [string, required] — the new issue key (e.g. `KAN-104`); feed into `→ ` `read_issue`, `→ ` `edit_issue`, `→ ` `add_comment`, `→ ` `transition_issue`.
- `data.self` [string, optional] — REST URL of the new issue.
- `meta.command` [string] — `issue.create`; on `--dry-run` the payload is validated and no Jira call is made.

**Preconditions**
- Native ADF is the canonical wire shape. Use the **bare Jira field name** (`description`, `environment`, `customfield_NNNN`) when passing an ADF document — there is no `*_adf` convention; the CLI does not rename keys, and Jira rejects unknown keys.
- Aliases the CLI translates server-side:

  | Alias                  | Translates to               |
  |------------------------|-----------------------------|
  | `project_key`          | `project.key`               |
  | `issue_type`           | `issuetype.name`            |
  | `description_markdown` | `description` (ADF, lossy)  |
  | `assignee_account_id`  | `assignee.accountId`        |

- Headless minimum under `--no-input`: `summary` + `project_key` + `issue_type` (or defaults from the profile).
- Prime custom-field metadata before authoring values: → `cache_metadata` (`cache fields`, `cache projects`, `cache issuetypes`).

**Behavior**
- Detection of ADF in the payload is **by value shape, not key suffix** — the CLI walks the payload, finds any value whose root matches `{type: "doc", version: N, content: [...]}`, and validates it. Strict mode rejects with the offending node/mark name; best-effort preserves and emits `unknown_adf_node` / `unknown_adf_mark` warnings.
- `--body-markdown` and `description_markdown` are human convenience layers and are lossy — use them only when you can tolerate the loss.
- For the full ADF document shape and supported nodes/marks, see → `adf_reference`.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `Operation value must be an Atlassian Document` on `environment` | Passed `environment` as a plain string; on most modern Jira instances it is an ADF field | Re-run with a full ADF doc value for `environment` (same shape as `description`) |
| `Operation value must be an Atlassian Document` on `description` | Plain string for `description` | Wrap in `{type: "doc", version: 1, content: [...]}` or use `description_markdown` |
| `unknown_adf_node` / `unknown_adf_mark` warning | Best-effort run kept an unsupported node | Re-run with `--adf-strict` to surface and fix, or accept the degradation |
| exit 3 with missing-fields error | Required field absent under `--no-input` | Add `summary` + `project_key` + `issue_type` (or rely on profile defaults) |
| `unknown key` rejection from Jira | Used a `*_adf` suffix or any other non-Jira key name | Drop the suffix — pass `description`/`environment`/`customfield_NNNN` bare |

**Next**
- Then: → `read_issue` to confirm rendered fields, or → `transition_issue` to move it off the initial state.
- Subtask of an existing parent? → `create_subtask`.
- Adding context: → `add_comment`, → `attach_file`, → `link_issues`.

## create_subtask
Goal: Create a subtask under an existing parent issue and capture the new child key.

**Decide**

# parent
- Parent issue key (e.g. `KAN-104`) — must already exist; the CLI does not verify it client-side.

# body
- Same shape and options as → `create_issue` — same `--json-input`, same ADF rules, same alias table, same convenience flags.

# guard
- `--dry-run` validates the payload (including the `parent.key` field schema) without calling Jira.

**Run**
- Canonical: `jira issue create --no-input --json-input subtask.json --output=json`
- Preview only: `jira issue create --dry-run --no-input --json-input subtask.json --output=json`

Subtask payload (note `issue_type: "Subtask"` and `parent.key`):

```json
{
  "summary": "REL: Subtask 1 of KAN-104",
  "issue_type": "Subtask",
  "project_key": "KAN",
  "parent": {"key": "KAN-104"},
  "description": {"type": "doc", "version": 1, "content": [
    {"type": "paragraph", "content": [{"type": "text", "text": "Detail of subtask 1."}]}
  ]}
}
```

**Save**
> Requires `--output=json`.
- `data.key` [string, required] — the new subtask key; feed into `→ ` `read_issue` or downstream mutations.
- `meta.command` [string] — `issue.create` (subtasks share the create envelope).

**Preconditions**
- `issue_type` must be a subtask-style type in the target project (typically `"Subtask"`; some projects rename it). If unsure, prime → `cache_metadata` (`cache issuetypes`) first.
- `parent.key` is required for subtask-style types and must reference an issue in the same project.
- All `create_issue` preconditions apply — same alias table, same bare-field-name rule for ADF.

**Behavior**
- There is **no CLI-side verification of the subtask list** today. The typed `issue view` envelope does not project `subtasks`, so after creation you can confirm the child exists with → `read_issue` on the new key, but you cannot list a parent's subtasks via the typed envelope.
- ADF detection, alias translation, dry-run stages, and warning behavior all match → `create_issue` exactly.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Jira rejects `parent.key` as unknown field | Parent type not configured for subtasks in this project | Verify the project workflow allows subtasks; pick a different `issue_type` |
| `Parent issue does not exist` | Bad `parent.key` (typo, deleted, or wrong project) | Re-run → `read_issue` against the intended parent first |
| Any ADF / alias / required-field error | Same as → `create_issue` | See the `create_issue` Recover table |

**Next**
- Then: → `read_issue` on the returned `data.key` (the parent's subtask list is not exposed via the typed envelope).
- Then: → `link_issues` to add non-parent/child relationships.
- Composes: → `create_issue` (subtask creation is a specialization of the create workflow).

## edit_issue
Goal: Update one or more fields on an existing issue without opening an editor.

**Decide**

# target
- Issue key positional argument (e.g. `KAN-104`).

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

## transition_issue
Goal: Move an issue to a new workflow state by picking an available transition ID for that issue.

**Decide**

# step 1 — list
- Always list first. Transition IDs are **workflow-specific** — they vary per project and per workflow, so the IDs you saw on `KAN-104` are not necessarily valid on `OTHER-1`.

# step 2 — execute
- Pick an `id` from the listed transitions and pass it to `--transition`.

# guard
- `--dry-run` validates the request (issue exists, transition ID present on this issue) without changing state.

**Run**
- List available transitions for an issue: `jira issue transition KEY --output=json`
- Execute the chosen transition: `jira issue transition KEY --transition <id> --output=json`
- Preview only: `jira issue transition KEY --transition <id> --dry-run --output=json`

**Save**
> Requires `--output=json`.
- `data.transitions[].id` [string, required] — pass to `--transition` to execute.
- `data.transitions[].name` [string, required] — human label (e.g. `"In Progress"`, `"Done"`).
- `data.transitions[].to.name` [string, optional] — target status the transition lands in.
- `meta.command` [string] — `issue.transition` on execute; the list call returns the same envelope shape with `data.transitions[]` populated.

**Preconditions**
- Headless minimum under `--no-input`: `--transition <id>` is required to execute.
- The transition must be currently valid for this issue's state — Jira rejects transitions that aren't reachable from the issue's current status, even if they exist elsewhere in the workflow.

**Behavior**
- Listing and executing are the same subcommand (`jira issue transition KEY`); the presence of `--transition <id>` switches modes.
- `--dry-run` runs validation but never sends the state-change request.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Empty `data.transitions[]` from list | Issue has no valid transitions from its current state (terminal status or permissions) | → `read_issue` to confirm status; check permissions on the project workflow |
| `Transition <id> is not available` on execute | ID was correct for a different issue / project / workflow | Re-list with `jira issue transition KEY --output=json` and pick from this issue's actual set |
| exit 3 with missing `--transition` | Tried to execute under `--no-input` without `--transition <id>` | Add `--transition <id>` from a prior list call |

**Next**
- Then: → `read_issue` to confirm the new status.
- Then: → `add_comment` to document why the state changed.
- Composes: → `safe_mutation` (`--dry-run` validates without mutating, matching the rest of the mutation surface).

## add_comment
Goal: Post a comment on a Jira issue, capturing the persisted comment shape (id, authors, timestamps, visibility) for follow-up edits.

**Decide**

# body shape
- Native ADF (preferred for agents — lossless): `--json-input <file>` or `--json-input -` with the ADF doc.
- Markdown convenience (lossy — see → `adf_reference` for what survives): `--body-markdown "<markdown>"`.
- `--json-input` and `--body-markdown` are mutually exclusive on `comment add`.

# guards
- `--no-input` is required in agent / non-TTY mode; with no body flags it would otherwise open an editor and exit 3.
- `--dry-run` validates and short-circuits before any Jira call.

**Run**
- ADF (file): `jira issue comment KEY --json-input adf.json --no-input --output=json`
- ADF (stdin): `cat adf.json | jira issue comment KEY --json-input - --no-input --output=json`
- Markdown: `jira issue comment KEY --body-markdown "**heads up**" --no-input --output=json`

`adf.json` shape — either the full body wrapped in `{"body": {...}}` or just the ADF doc itself:

```json
{
  "body": {
    "type": "doc", "version": 1, "content": [
      {"type": "heading", "attrs": {"level": 3}, "content": [{"type": "text", "text": "Update"}]},
      {"type": "paragraph", "content": [
        {"type": "text", "text": "Status: "},
        {"type": "text", "text": "blocked", "marks": [{"type": "strong"}]}
      ]}
    ]
  }
}
```

**Save**
> Requires `--output=json`.

`comment add KEY` (and `comment edit KEY ID`) return the persisted comment shape:

```json
{
  "data": {
    "comment": {
      "id": "10042",
      "body": "Updated body…",
      "author": {"account_id": "<original>", "display_name": "Alice"},
      "update_author": {"account_id": "<caller>", "display_name": "Matt"},
      "created": "2026-04-01T10:00:00.000+0000",
      "updated": "2026-05-05T11:22:33.000+0000",
      "visibility": {"type": "role", "value": "Developers"}
    }
  }
}
```

- `data.comment.id` [string, required] — feed to `comment edit KEY <id>` or → `list_comments` `comment delete KEY <id> --force`.
- `data.comment.body` [string, required] — rendered text after Jira persistence.
- `data.comment.author` / `data.comment.update_author` [object, optional] — original author vs the caller who last edited; `update_author` is `null` on initial create.
- `data.comment.created` / `data.comment.updated` [string, required] — RFC3339 timestamps.
- `data.comment.visibility` [object, optional] — `{type, value}` when role- or group-restricted; `null` for public.

**Preconditions**
- ADF doc shape must satisfy → `adf_reference`; unknown fields are rejected upstream by Jira.
- `--body-markdown` is converted client-side; constructs without an ADF mapping degrade (see → `adf_reference`).

**Behavior**
- The two body flags are mutually exclusive — passing both fails locally with exit 3 before any Jira call.
- ADF payloads round-trip without loss; markdown payloads are best-effort.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, `code: required_flag_missing` | `--no-input` with no body flag (editor flow refused) | Re-run with `--json-input` or `--body-markdown` |
| exit 3, `code: flag_value_invalid` | Both `--json-input` and `--body-markdown` set | Pick one body flag |
| exit 4 from upstream | Invalid ADF doc (unknown field, bad structure) | Validate against → `adf_reference` and re-run |

**Next**
- Then: → `list_comments` to read the updated thread back, or `comment edit KEY <id>` with the returned `data.comment.id` to revise.
- Composes: → `read_issue` (most comment work happens inside an issue review loop).

## list_comments
Goal: Walk an issue's comment thread oldest-first, paginating through every page, and detect lossy-ADF surfaces before quoting comment bodies.

**Decide**

# scope
- One page (default `max_results`): `comment list KEY`.
- Specific page size: `--limit N`.
- All pages drained in one call: `--all`.

# delete (force-gated under `--no-input`)
- `comment delete KEY <id> --force --output=json` removes a specific comment; the `<id>` comes from `data.comments[].id` of this workflow or from → `add_comment` `data.comment.id`.

**Run**
- Single page: `jira issue comment list KEY --output=json`
- Sized page: `jira issue comment list KEY --limit 50 --output=json`
- Drain all: `jira issue comment list KEY --all --output=json`
- Delete: `jira issue comment delete KEY 10042 --force --no-input --output=json`

**Save**
> Requires `--output=json`.

`comment list KEY` envelope (oldest-first):

```json
{
  "data": {
    "comments": [
      {"id": "10101", "body": "Markdown rendered text…",
       "author": {"account_id": "...", "display_name": "Alice"},
       "update_author": null,
       "created": "2026-04-01T10:00:00.000+0000",
       "updated": "2026-04-01T10:00:00.000+0000",
       "visibility": null}
    ],
    "pagination": {"total": 142, "start_at": 0, "max_results": 50, "is_last": false, "next_page_token": "50"}
  },
  "warnings": [
    {"type": "adf-lossy-comment", "comment_id": "10103", "lossy_constructs": ["inlineCard", "panel:custom"]}
  ]
}
```

- `data.comments[].id` [string, required] — pass to `comment edit` or `comment delete`.
- `data.comments[].body` [string, required] — markdown rendering; check `warnings[]` for any lossy entries before treating as canonical.
- `data.comments[].author` / `data.comments[].update_author` [object, optional] — `update_author` is `null` when the comment has never been edited.
- `data.comments[].visibility` [object, optional] — `null` for public, otherwise `{type, value}` (role/group restriction).
- `data.pagination.is_last` [bool, required] — stop when `true`.
- `data.pagination.next_page_token` [string, optional] — feed back as paging cursor until `is_last=true`.
- `warnings[].comment_id` + `warnings[].lossy_constructs[]` [array, optional] — if a comment id appears here, its `body` is a degraded markdown projection — re-read with native ADF tooling when fidelity matters.

`comment delete KEY ID --force`:

```json
{"data": {"comment_id": "10042", "deleted": true}}
```

- `data.comment_id` [string, required] — echo of the deleted id.
- `data.deleted` [bool, required] — `true` on success.

**Preconditions**
- Comments are returned oldest-first; agent loops that want "latest" must take the last entry of the last page, not the first.
- `comment delete --force` is mandatory under `--no-input`; without `--force` the command exits 3.

**Behavior**
- `warnings[]` does not change the exit code — the response is still successful; treat lossy markers as a signal to switch to ADF reads, not as failure.
- Pagination is cursor-based via `next_page_token`; do not assume `start_at + max_results` is enough.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `warnings[].type = adf-lossy-comment` | Comment contained an ADF construct without a markdown mapping | Re-fetch with ADF tooling (see → `adf_reference`) before quoting |
| exit 3 on delete | `--no-input` without `--force` | Re-run with `--force` |
| exit 2 (`not_found`) on delete | Wrong issue key or comment id | Re-list and copy `data.comments[].id` verbatim |

**Next**
- Then: → `add_comment` to reply, or `comment edit KEY <id>` with a captured id.
- Composes: → `read_issue` (full-issue review loops fold in this thread walk).

## attach_file
Goal: List, upload, download, or remove file attachments on a Jira issue with size-cap and clobber/force guards honored.

**Decide**

# direction
- List: `jira issue attachment list KEY` — oldest-first.
- Add: `jira issue attachment add KEY --file <path>` — multipart upload.
- Download: `jira issue attachment download KEY <id> --to <path>` — writes to disk, never stdout.
- Delete: `jira issue attachment delete KEY <id> --force` — force-gated under `--no-input`.

# guards
- Download is clobber-protected: an existing target file blocks the write unless explicitly overwritten.
- Delete requires `--force` in `--no-input` / agent mode.

**Run**
- List: `jira issue attachment list KEY --output=json`
- Add: `jira issue attachment add KEY --file ./trace.log --output=json`
- Download (named target): `jira issue attachment download KEY 10042 --to ./local.pdf --output=json`
- Download (current dir): `jira issue attachment download KEY 10042 --output=json`
- Delete: `jira issue attachment delete KEY 10043 --force --output=json`

**Save**
> Requires `--output=json`.

`attachment list` envelope:

```json
{
  "data": {
    "attachments": [
      {"id": "10042", "filename": "screenshot.png", "mime_type": "image/png", "size": 84211,
       "author": {"account_id": "...", "display_name": "Matt Craven"},
       "created": "2026-05-04T18:30:00.000+0000"}
    ],
    "pagination": {"total": 1, "start_at": 0, "max_results": 50, "is_last": true, "next_page_token": null}
  }
}
```

`attachment add`:

```json
{
  "data": {
    "attachments": [
      {"id": "10043", "filename": "trace.log", "mime_type": "text/plain", "size": 4012,
       "author": {"account_id": "...", "display_name": "..."}, "created": "..."}
    ]
  }
}
```

`attachment download` reports the written path and bytes:

```json
{"data": {"attachment_id": "10042", "written_to": "./local.pdf", "bytes": 124521, "mode": "output"}}
```

`attachment delete`:

```json
{"data": {"attachment_id": "10043", "deleted": true}}
```

- `data.attachments[].id` [string, required] — feed to `attachment download` and `attachment delete`.
- `data.attachments[].filename` / `.mime_type` / `.size` [string / string / int, required] — server-side metadata.
- `data.attachments[].author` [object, required] — uploader.
- `data.pagination.is_last` / `data.pagination.next_page_token` [bool / string] — paginate `attachment list` until `is_last=true`.
- `data.attachment_id` [string, required on download/delete] — echo of the target id.
- `data.written_to` [string, required on download] — actual disk path written.
- `data.bytes` [int, required on download] — file size on disk after write.
- `data.mode` [string, required on download] — `output` when `--to PATH` was given, else `current-dir`.
- `data.deleted` [bool, required on delete] — `true` on success.

**Preconditions**
- The binary always writes downloads to a file; there is no stdout streaming mode.
- `attachment delete` under `--no-input` requires `--force` (else exit 3).

**Behavior**
- Download is clobber-protected — an existing file at the target path is not overwritten silently.
- Each project can pin its own upload size cap; the per-project cap is enforced by Jira, not the CLI.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 5, upstream 413 | Upload exceeded the per-project attachment size cap; upstream message is preserved verbatim in `errors[].message` | Shrink/split the file; cap is server-set, not CLI-configurable |
| exit 3 on delete | `--force` missing under `--no-input` | Re-run with `--force` |
| exit 3 on download | Target file already exists (clobber guard) | Choose a fresh `--to` path or remove the stale file first |
| exit 2 (`not_found`) | Wrong attachment id | Re-list and copy `data.attachments[].id` verbatim |

**Next**
- Then: → `add_comment` to reference the new attachment in the issue thread.
- Composes: → `read_issue` (attachments are part of the issue evidence trail).

## manage_watchers
Goal: Resolve, add, remove, or audit watchers on a Jira issue while handling ambiguous user input via structured `candidates[]`.

**Decide**

# direction
- List: `jira issue watchers list KEY`.
- Add: `jira issue watchers add KEY --user <user>` (alias `jira issue watch KEY`).
- Remove: `jira issue watchers remove KEY --user <user>` (alias `jira issue unwatch KEY`).

# user spec
- Account id (always locally resolvable): `--user accountId:<id>`.
- Self: `--user me` (locally resolvable when the active profile carries an account id).
- Display name or email: needs a remote `/user/search` to resolve — see Behavior for dry-run rules.

# guards
- `--dry-run` is local-only — it contacts Jira for nothing unless paired with `--validate-remote`.
- `--validate-remote` alongside `--dry-run` opts into a read-only resolve (still no watcher `POST`/`DELETE`).

**Run**
- List: `jira issue watchers list KEY --output=json`
- Add: `jira issue watchers add KEY --user me --output=json`
- Remove (alias): `jira issue unwatch KEY --user accountId:712020:abc --output=json`
- Dry-run preview: `jira issue watchers add KEY --user me --dry-run --output=json`
- Dry-run + remote resolve: `jira issue watchers add KEY --user alice --dry-run --validate-remote --output=json`

**Save**
> Requires `--output=json`.

`watchers list` envelope — `is_watching` / `watch_count` mirror Atlassian's native shape:

```json
{
  "data": {
    "watchers": [
      {"account_id": "...", "display_name": "Alice", "email_address": "alice@example.com", "active": true}
    ],
    "is_watching": true,
    "watch_count": 3
  }
}
```

`watchers add` / `watch` (default readback):

```json
{
  "data": {
    "watchers": [{"account_id": "...", "display_name": "Alice", "active": true}],
    "was_already_watching": false
  }
}
```

`watchers add --dry-run` (locally resolvable input):

```json
{"data": {"key": "KAN-1", "user": "accountId:712020:abc", "account_id_resolved": "712020:abc", "user_resolved": true, "dry_run": true}}
```

- `data.watchers[].account_id` [string, required] — stable identity; pass back as `accountId:<id>` to subsequent calls.
- `data.watchers[].display_name` / `.email_address` [string, optional] — display fields; `email_address` may be `null` on privacy-restricted directories.
- `data.watchers[].active` [bool, required] — Jira account active flag.
- `data.is_watching` [bool, required] — whether the calling identity is in the list.
- `data.watch_count` [int, required] — total watcher count (may exceed `len(data.watchers)` when truncated).
- `data.was_already_watching` [bool, required on add] — `true` makes the call effectively a no-op.
- `data.user_resolved` [bool, required on dry-run] — `false` when the input needs a remote lookup that dry-run skipped.
- `data.account_id_resolved` [string, optional on dry-run] — only present when local resolution succeeded.
- `data.dry_run` [bool, required on dry-run] — always `true` for previews.

**Preconditions**
- Bare names/emails cannot be locally resolved; without `--validate-remote`, dry-run echoes them back with `user_resolved: false` and no `account_id_resolved`.
- `--user me` resolves locally only when the active profile carries an account id (see → `auth_setup` and → `identity_setup`).

**Behavior**
- `watchers add` and `watchers remove` perform the mutation and then read back the watcher list (so `data.watchers[]` reflects post-state).
- Aliases `watch` / `unwatch` are sugar — identical envelopes, identical exit codes.
- Every error entry carries `type`, `code` (stable snake_case — branch on this, never on `message`), `message`, `hint`, and `retryable`. Optional fields appear when relevant: `flag`, `field`, `http_status`, `retry_after_seconds`, `provider`, `upstream_code`, `upstream_status`. For Jira API errors `upstream_code` is empty — Jira exposes no stable machine error code.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, `code: validation_failed`, `candidates[]` populated | `--user` matched >1 directory entry | Pick a `candidates[].account_id` and re-run with `--user accountId:<id>` |
| exit 2, `code: not_found` | `--user` matched zero directory entries; input echoed in `errors[0].message` | Refine the spelling or use `accountId:<id>` |
| exit 3 on a flag (`flag_unknown`, `flag_value_invalid`, `required_flag_missing`, `arg_count_invalid`, `command_unknown`, ...) | Malformed invocation — fails before any Jira call | See → `core_contract` for the canonical command-line input error table; `hint` may carry a "Did you mean …?" |

Ambiguous-user resolution → exit 3 with structured candidates so the agent can re-run with `--user accountId:<id>`. This envelope is the canonical template for every command that surfaces ambiguous user / option resolution:

```json
{
  "ok": false,
  "meta": {"command": "issue.watchers.add", "exit_code": 3, "timestamp": "..."},
  "data": null,
  "warnings": [],
  "errors": [
    {
      "type": "validation",
      "code": "validation_failed",
      "message": "ambiguous user 'alice' — 3 candidates",
      "hint": "Re-run with --user accountId:<id>.",
      "retryable": false,
      "candidates": [
        {"account_id": "1", "display_name": "Alice Smith", "email_address": "alice.smith@example.com"},
        {"account_id": "2", "display_name": "Alice Jones", "email_address": "alice.jones@example.com"},
        {"account_id": "3", "display_name": "Alice Brown", "email_address": null}
      ]
    }
  ]
}
```

**Next**
- Then: → `read_issue` to confirm the resulting watcher state in the issue context.
- Alternative: → `add_comment` to ping someone instead of subscribing them (watchers are notification side-effects, not @-mentions).

## link_issues
Goal: Create, audit, list, or delete typed issue-to-issue links, respecting Jira's inward/outward semantics and the bulk-edit exclusion.

**Decide**

# direction & semantics
- `KEY` is the inward side, `--to` the outward. "KAN-72 blocks KAN-73" means `KEY=KAN-73 --to KAN-72 --type Blocks`.
- A blocks B (B is blocked by A): `jira issue link <BLOCKED> --to <BLOCKER> --type Blocks`.
- A and B related (no direction): `jira issue link KAN-73 --to KAN-72 --type Relates`.
- A is a duplicate of canonical B: `jira issue link <DUP> --to <CANONICAL> --type Duplicate`.
- A is a clone of B: `jira issue link <CLONE> --to <ORIGINAL> --type Cloners`.

# subcommand
- Create: `jira issue link KEY --to OTHER --type <Name>` (with optional `--dry-run`).
- List existing links on an issue: `jira issue link list KEY`.
- Delete by link id: `jira issue link delete KEY <link-id> --force`.
- Discover available link types: `jira issue link types` (cached; primable via `jira cache linktypes`).

# guards
- `link delete` is force-gated under `--no-input`.
- `--dry-run` previews creation without contacting Jira.

**Run**
- Create blocker: `jira issue link KAN-73 --to KAN-72 --type Blocks --output=json`
- Create undirected: `jira issue link KAN-73 --to KAN-72 --type Relates --output=json`
- Preview: `jira issue link KAN-73 --to KAN-72 --type Blocks --dry-run --output=json`
- List on issue: `jira issue link list KEY --output=json`
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
       "other_issue": {"key": "KAN-200", "summary": "downstream service work", "status": "In Progress"}},
      {"id": "9002",
       "type": {"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
       "direction": "inward",
       "other_issue": {"key": "KAN-100", "summary": "upstream API contract", "status": "Done"}}
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
- `data.link_types[].name` [string, required] — what to pass as `--type` on `link create`. Custom types may exist; always discover before assuming.
- `data.from_cache` / `data.cache_state` / `data.cache_empty` [bool / string / bool] — cache-primer convention; see → `cache_metadata`.
- `data.link_id` / `data.deleted` [string / bool, required on delete] — echo + success flag.

**Preconditions**
- Bulk edit cannot update `issuelinks` — Jira refuses with `"Field does not support update 'issuelinks'"`. This command is the only path; do NOT attempt → `edit_issue` to set links.
- Link type names are instance-specific (admins add custom ones). Discover before hard-coding.

**Behavior**
- `link delete --force` is required under `--no-input` (force-gated).
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

## add_weblink
Goal: Attach a remote URL (with display title) to a Jira issue via the remote-link endpoint, rejecting non-web schemes client-side.

**Decide**

# inputs
- `--url <http(s) URL>` — required.
- `--title <text>` — optional display label; what the user sees in the issue UI.

# scheme guard
- `--url` must use `http://` or `https://` (case-insensitive). Other schemes are rejected before any HTTP call.

**Run**
- `jira issue weblink KEY --url "https://example.com/spec" --title "Spec doc" --output=json`

**Save**
> Requires `--output=json`.
- Standard envelope; `data` carries the persisted remote-link record. Use the response only to confirm the call succeeded — agents typically don't need to dereference the remote-link id.

**Preconditions**
- Calls `POST /rest/api/3/issue/{KEY}/remotelink` — a different endpoint from → `link_issues`. Do not confuse with issue-to-issue links.
- `--url` is required.

**Behavior**
- The CLI rejects any non-`http(s)` scheme client-side before reaching Jira. Disallowed schemes include: `javascript:`, `file:`, `ftp:`, `data:`, `mailto:` (and any other non-web scheme).
- If you need a non-web link target, use a regular issue comment instead (see → `add_comment`).

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, `code: flag_value_invalid` on `--url` | Non-`http(s)` scheme | Re-run with a `http://` or `https://` URL, or use → `add_comment` to record the non-web reference |
| exit 3, `code: required_flag_missing` | Missing `--url` | Provide `--url` |

**Next**
- Then: → `read_issue` to confirm the remote link surface, or → `add_comment` to call attention to it in the thread.
- Alternative: → `link_issues` when the target is another Jira issue (different endpoint).

## log_work
Goal: Record time spent against a Jira issue using durations that resolve via the profile's `workday_seconds`, and read the typed worklog list back.

**Decide**

# duration
- `--time-spent <duration>` — accepts `1d 2h 30m`-style strings; `1d` resolves via the per-profile `workday_seconds` (default 28,800 = 8h).

# optional metadata
- `--started <RFC3339>` — backdate or pin a start time; otherwise Jira stamps "now".
- `--comment-markdown "<text>"` — short worklog comment; markdown is lossy (see → `adf_reference`).
- `--json-input <file>` — full payload, including ADF-bodied comment, for the lossless path.

**Run**
- Quick log: `jira worklog add KEY --time-spent 1h30m --output=json`
- Backdated: `jira worklog add KEY --time-spent 2h --started 2026-05-04T09:00:00.000+0000 --output=json`
- With markdown comment: `jira worklog add KEY --time-spent 45m --comment-markdown "fixed bug X" --output=json`
- Full payload (ADF comment supported): `jira worklog add KEY --json-input wl.json --output=json`
- Read back: `jira worklog list KEY --output=json`

**Save**
> Requires `--output=json`.
- `worklog add` returns the persisted entry envelope; capture the entry id for follow-up edits.
- `worklog list KEY` returns the worklog list as the typed envelope — iterate to compute totals or surface recent entries.

**Preconditions**
- `1d` resolution depends on the active profile's `workday_seconds`; a profile configured for 6h days will resolve `1d` to 21,600 seconds, not 28,800.
- ADF worklog comments must satisfy → `adf_reference`.

**Behavior**
- Durations are concatenable (`1d 2h 30m`); whitespace is permitted between units.
- `--comment-markdown` and the `comment` field inside `--json-input` are mutually exclusive on the same call — pick one body path.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| exit 3, `code: flag_value_invalid` on `--time-spent` | Duration string didn't parse (typo, unknown unit) | Re-run with a `d/h/m/s` combination |
| Unexpected `1d` totals | Profile `workday_seconds` differs from 8h | Check active profile (see → `auth_setup`) and recompute |

**Next**
- Then: → `read_issue` (worklog totals influence the issue summary).
- Composes: → `add_comment` when the worklog comment alone isn't enough context.

## clone_issue
Goal: Duplicate an existing issue (same project or different) by reading its fields, sanitizing lifecycle/timing noise, and POSTing a fresh issue — optionally with caller-supplied overrides.

**Decide**
- Same project, no field changes? Straight clone — `--force` only.
- Same project but want a different summary, assignee, or stripped fields? Merge a `--json-input` override; caller fields win over source fields.
- Different project? Override `project.key` (and any required-field changes the target project mandates).
- Clear an inherited field on the copy? Set it to `null` in the override.
- Need a preview that runs the full validation pipeline but never POSTs? `--dry-run` (still requires `--force` in agent context — see → `safe_mutation`).

**Run**
- Canonical: `jira issue clone KAN-1 --force --output=json`
- With overrides: `jira issue clone KAN-1 --force --json-input /tmp/over.json --output=json`
- Preview (full validation, no POST): `jira issue clone KAN-1 --force --dry-run --output=json`

Override file shape (override fields merge on top of carried source fields):

```json
{"fields": {"summary": "Triage copy of KAN-1", "assignee": {"accountId": "<your-id>"}}}
```

Different-project clone:

```json
{"fields": {"project": {"key": "OTHER"}, "summary": "Ported from KAN-1"}}
```

**Save**
> Requires `--output=json`.
- `data.key` [string, required] — key of the new issue (feed to → `read_issue`, → `edit_issue`, → `transition_issue`).
- `data.id` [string, required] — numeric id of the new issue.
- `data.self` [string] — REST URL of the new issue.

**Behavior**

`issue clone` is a GET → sanitize → POST round-trip. The set of fields it carries vs drops is fixed:

- **Carries**: `summary`, `description`, `issuetype`, `project`, `priority`, `assignee`, `labels`, `components`, `fixVersions`, `affectedVersions`, `duedate`, all `customfield_*` (except lexorank-shaped values — Jira Software's Rank field is auto-assigned on the new issue).
- **Drops**: identifiers (`id`, `key`, `self`), lifecycle (`status`, `statusCategory`, `statuscategorychangedate`, `resolution`, `resolutiondate`, `created`, `updated`, `creator`, `reporter`, `lastViewed`, `issuerestriction`), time-tracking (`timeestimate`, `timespent`, `timeoriginalestimate`, `workratio`, `progress`, `timetracking`, `aggregate*`), positioning (`rankBeforeIssue`, `rankAfterIssue`), and collections (`comment`, `worklog`, `subtasks`, `attachment`, `votes`, `watches`, `issuelinks`).

Override merge rule: caller `--json-input` fields overwrite the carried source value; explicit `null` strips a carried field.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `validation_error` requesting `--force` | Agent / non-TTY without `--force` | Add `--force`. See → `safe_mutation`. |
| `INVALID_INPUT (400)` after a clean `--dry-run` | Target project requires a customfield the source didn't carry | Add the missing field to the override, re-run. → `cache_metadata` to discover the field id. |
| Cloned issue is missing the Rank | Expected — Jira Software auto-assigns Rank on insert | No action; reorder via the board / `customfield_NNNN` if needed. |

**Next**
- Then: → `edit_issue` to tweak the new issue, → `link_issues` to relate it back to the source.
- Composes: → `safe_mutation` (destructive workflow contract).
- Alternative: → `move_issue` if you want to relocate the original rather than copy it.

## move_issue
Goal: Swap an existing issue's `project` and/or `issuetype` in place, providing any new required fields the destination demands — without creating a new issue or changing the issue key history.

**Decide**
- Just project change? Override `project.key`.
- Just type change? Override `issuetype.name` (or `.id`).
- Both? Pass both in the same override.
- Target project / type requires fields the source doesn't have? Include them in the same override (e.g. `customfield_10010`).
- Want to confirm the override is well-formed before submitting? `--dry-run`.

**Run**
- Canonical: `jira issue move KAN-1 --force --json-input /tmp/move.json --output=json`
- Preview: `jira issue move KAN-1 --force --json-input /tmp/move.json --dry-run --output=json`

Minimum override shape:

```json
{"fields": {"project": {"key": "OTHER"}, "issuetype": {"name": "Story"}}}
```

**Save**
> Requires `--output=json`.
- `data.key` [string, required] — the issue key after move (may or may not change depending on instance config; capture and feed to follow-up workflows).

**Preconditions**
- `--json-input` is required — `move` has no field flags. The destination shape is the entire contract.
- `--force` (or `--dry-run`) is required in agent context — see → `safe_mutation`.

**Behavior**
- No new issue is created; the original key (or its remapped successor on instances that renumber across projects) carries forward, along with comments, worklogs, and attachments.
- Required-field changes between projects / issuetypes must appear in the override. If the target project mandates a field the source didn't have, the submit fails with `INVALID_INPUT (400)`.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `INVALID_INPUT (400)` after `--dry-run` was clean | Target project requires a field not in the override | Add the missing field; → `cache_metadata` (`fields` cache) to discover its id. |
| `validation_error` requesting `--force` | Agent / non-TTY without `--force` | Add `--force`. See → `safe_mutation`. |

**Next**
- Then: → `read_issue` to confirm the post-move shape, → `transition_issue` if the new project's workflow needs a status sync.
- Composes: → `safe_mutation`.
- Alternative: → `clone_issue` if you want a copy in the new project while leaving the original in place.

## delete_issue
Goal: Permanently remove an issue (and optionally its subtasks) from Jira after confirming nothing downstream depends on it.

**Decide**
- Issue has subtasks? You MUST pass `--delete-subtasks` — Jira refuses to delete a parent otherwise. The flag drains parent + every subtask atomically.
- Want to confirm the call is shaped right without hitting Jira? `--dry-run` (still requires `--force` in agent context).
- Operating in a TTY without `--force`? Expect a `huh` prompt that requires typing `Yes, delete` verbatim.
- Operating in agent / `--no-input` mode? `--force` is mandatory — see → `safe_mutation`.

**Run**
- Canonical (agent): `jira issue delete KAN-1 --force --output=json`
- With subtasks: `jira issue delete KAN-1 --force --delete-subtasks --output=json`
- Preview (no Jira mutation): `jira issue delete KAN-1 --force --dry-run --output=json`

**Save**
> Requires `--output=json`.
- `data.key` [string, required] — echo of the deleted key (use as evidence).
- `data.deleted` [bool, required] — `true` once the delete returned 204.
- `data.deleted_subtasks` [array of strings] — keys removed alongside the parent when `--delete-subtasks` is set.

**Preconditions**
- `--force` is mandatory in agent / non-TTY / `--no-input` mode. Omitting it exits `3` with `validation_error`.
- The caller must have permission to delete in the project; otherwise Jira returns `FORBIDDEN (403)`.

**Behavior**
- Deletion is irreversible. There is no undo.
- ⚠ **Subtasks block deletion** — Jira refuses to delete a parent with subtasks unless `--delete-subtasks` is set. Without it, the call fails server-side; with it, the parent + every subtask are removed atomically.
- `--dry-run` is always allowed (TTY or not) and never touches Jira.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| `validation_error` (exit `3`) requesting `--force` | Agent / non-TTY without `--force` | Add `--force`. See → `safe_mutation`. |
| `INVALID_INPUT (400)` with subtask reference | Parent has subtasks and `--delete-subtasks` wasn't set | Re-run with `--delete-subtasks`. |
| `NOT_FOUND (404)` | Already deleted, or key doesn't exist | Treat as already-clean in bulk loops. |
| `FORBIDDEN (403)` | Caller lacks delete permission in the project | Switch profile (→ `auth_setup`) or accept the issue stays. |

**Next**
- Then: nothing — the issue is gone. Update any cached search results that referenced the key.
- Composes: → `safe_mutation`.
- Alternative: → `transition_issue` to a terminal `Done` / `Cancelled` state if you want the record preserved, → `edit_issue` to clear sensitive fields without deleting.

## adf_reference
When to use this: every long-form body the CLI sends to Jira is an ADF (Atlassian Document Format) doc — `description` and `environment` on → `create_issue` / → `edit_issue`, the body on → `add_comment`, the comment on → `log_work`, and any rich `customfield_*`. Use this section to look up node shapes, mark composition, the gotchas that bite (mention id, date timestamp, code block content), and the strict-vs-best-effort modes that decide whether a lossy step fails or warns.

ADF is canonical. The official spec is at
[developer.atlassian.com/cloud/jira/platform/apis/document](https://developer.atlassian.com/cloud/jira/platform/apis/document/).
The CLI's MVP support set is mirrored in
`jira agent adf-matrix --output=json` (per-row `official_url` points to the
Atlassian docs page for that node/mark).

Every ADF doc starts with the root:

```json
{ "type": "doc", "version": 1, "content": [ /* block nodes */ ] }
```

### Block nodes

```json
// paragraph (the simplest body)
{"type": "paragraph", "content": [{"type": "text", "text": "hello"}]}

// heading (level 1-6)
{"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "Section"}]}

// blockquote (wraps any block content)
{"type": "blockquote", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "quoted"}]}]}

// bulletList / orderedList (content is listItem[])
{"type": "bulletList", "content": [
  {"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "first"}]}]}
]}

// codeBlock (attrs.language is the syntax-highlight hint)
{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "func main() {}"}]}

// rule (horizontal divider, no content)
{"type": "rule"}

// panel (panelType = info | warning | error | success | note)
{"type": "panel", "attrs": {"panelType": "info"}, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "info panel"}]}]}

// table (content is tableRow[]; tableRow content is tableHeader[] / tableCell[])
{"type": "table", "attrs": {"isNumberColumnEnabled": false, "layout": "default"}, "content": [
  {"type": "tableRow", "content": [
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Col"}]}]}
  ]},
  {"type": "tableRow", "content": [
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "val"}]}]}
  ]}
]}

// expand (collapsible — non-MVP but Jira accepts it)
{"type": "expand", "attrs": {"title": "Click to expand"}, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "hidden"}]}]}

// nestedExpand (only valid INSIDE tableCell or tableHeader)
```

### Inline nodes (live inside a paragraph's content[])

```json
// text — the only required attr is .text
{"type": "text", "text": "plain"}

// hardBreak — line break inside a paragraph
{"type": "text", "text": "line one"}, {"type": "hardBreak"}, {"type": "text", "text": "line two"}

// mention — id MUST be the user's accountId
{"type": "mention", "attrs": {"id": "712020:ff38cf6b-...", "text": "@Matt Craven"}}

// emoji — shortName + id (unicode codepoint) + text (the actual unicode)
{"type": "emoji", "attrs": {"shortName": ":rocket:", "id": "1f680", "text": "🚀"}}

// date — timestamp is epoch milliseconds AS A STRING
{"type": "date", "attrs": {"timestamp": "1769817600000"}}

// status — text is the label, color is named (green/red/yellow/blue/purple/...)
{"type": "status", "attrs": {"text": "READY", "color": "green"}}

// inlineCard — smart link
{"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/KAN-1"}}
```

### Marks (annotate text nodes)

```json
{"type": "text", "text": "bold",       "marks": [{"type": "strong"}]}
{"type": "text", "text": "italic",     "marks": [{"type": "em"}]}
{"type": "text", "text": "struck",     "marks": [{"type": "strike"}]}
{"type": "text", "text": "underlined", "marks": [{"type": "underline"}]}
{"type": "text", "text": "code()",     "marks": [{"type": "code"}]}
{"type": "text", "text": "link",       "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]}
{"type": "text", "text": "red",        "marks": [{"type": "textColor", "attrs": {"color": "#ff0000"}}]}
{"type": "text", "text": "highlight",  "marks": [{"type": "backgroundColor", "attrs": {"color": "#fffacd"}}]}
{"type": "text", "text": "2",          "marks": [{"type": "subsup", "attrs": {"type": "sub"}}]}
```

Multiple marks on one text node compose:

```json
{"type": "text", "text": "loud", "marks": [{"type": "strong"}, {"type": "underline"}, {"type": "textColor", "attrs": {"color": "#ff0000"}}]}
```

### Composition recipes

Drop these straight into `--json-input` for `issue create`, `issue edit`,
or `issue comment`. As substructures of an `issue create` payload,
assign them to the bare Jira field name (`description`, `environment`,
the relevant `customfield_NNNN`) — that is what Jira's API accepts and
what the CLI's ADF validator detects.

**Heading + paragraph + link:**

```json
{"type": "doc", "version": 1, "content": [
  {"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "Investigation"}]},
  {"type": "paragraph", "content": [
    {"type": "text", "text": "See "},
    {"type": "text", "text": "PR #482", "marks": [{"type": "link", "attrs": {"href": "https://github.com/org/repo/pull/482"}}]},
    {"type": "text", "text": " for the fix."}
  ]}
]}
```

**Bulleted list with mixed marks per item:**

```json
{"type": "bulletList", "content": [
  {"type": "listItem", "content": [{"type": "paragraph", "content": [
    {"type": "text", "text": "DB write path: ", "marks": [{"type": "strong"}]},
    {"type": "text", "text": "blocked"}
  ]}]},
  {"type": "listItem", "content": [{"type": "paragraph", "content": [
    {"type": "text", "text": "Inline code "},
    {"type": "text", "text": "user.last_login", "marks": [{"type": "code"}]},
    {"type": "text", "text": " not updating."}
  ]}]}
]}
```

**Numbered list — same shape, swap `bulletList` for `orderedList`:**

```json
{"type": "orderedList", "attrs": {"order": 1}, "content": [/* listItem[]... */]}
```

**Code block with language:**

```json
{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "func main() {\n  fmt.Println(\"hi\")\n}"}]}
```

**Panel for callouts (info / warning / error / success / note):**

```json
{"type": "panel", "attrs": {"panelType": "warning"}, "content": [
  {"type": "paragraph", "content": [
    {"type": "text", "text": "Don't forget to bump the schema version."}
  ]}
]}
```

**Inline mention of a user:**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "cc "},
  {"type": "mention", "attrs": {"id": "712020:ff38cf6b-...", "text": "@Matt Craven"}},
  {"type": "text", "text": " — heads up"}
]}
```

The `id` MUST be the user's `accountId` (get it from
`jira me --output=json` for yourself, or the assignee field on any issue
they own). The `text` is the display label and can be anything.

**Status pill (named color: green / red / yellow / blue / purple / grey / neutral):**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "Build: "},
  {"type": "status", "attrs": {"text": "GREEN", "color": "green"}}
]}
```

**Inline date (epoch milliseconds, as a string):**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "Target: "},
  {"type": "date", "attrs": {"timestamp": "1769817600000"}}
]}
```

**Smart link (Jira renders as a card if the URL is recognised):**

```json
{"type": "paragraph", "content": [
  {"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/KAN-72"}}
]}
```

**Quote block (any block content can nest inside):**

```json
{"type": "blockquote", "content": [
  {"type": "paragraph", "content": [
    {"type": "text", "text": "From the postmortem: "},
    {"type": "text", "text": "the migration ran twice", "marks": [{"type": "em"}]},
    {"type": "text", "text": "."}
  ]}
]}
```

**Two-column table with header row:**

```json
{"type": "table", "attrs": {"isNumberColumnEnabled": false, "layout": "default"}, "content": [
  {"type": "tableRow", "content": [
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Field"}]}]},
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Value"}]}]}
  ]},
  {"type": "tableRow", "content": [
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "duration"}]}]},
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "2h30m"}]}]}
  ]}
]}
```

**Horizontal divider between sections:**

```json
{"type": "rule"}
```

### ADF gotchas

- **Marks live on `text` nodes only** — putting `marks` on a
  `paragraph` or `heading` is invalid; strict mode rejects with
  the path of the offending node.
- **`bulletList` / `orderedList` content is `listItem[]`** — and
  every `listItem` content is `paragraph` (or another list to
  nest). Putting raw `text` directly inside a `listItem` is invalid.
- **`mention.attrs.id` is the accountId, not the email or display name.**
  Wrong id → mention renders as plain text in Jira.
- **`date.attrs.timestamp` is a string of milliseconds**, not a
  number and not seconds. `1769817600000` not `1769817600`.
- **`codeBlock` content is a single `text` node** containing the
  whole code with embedded `\n`s — not a list of lines.
- **`emoji.attrs.id` is the unicode codepoint** (e.g. `1f680`),
  `shortName` is the `:rocket:`-style alias, `text` is the actual
  unicode glyph (`🚀`). All three should be set for portability.
- **`tableCell` / `tableHeader` content must be wrapped in
  `paragraph`** — bare text inside a cell is invalid.

### ADF strict vs best-effort

| Path                          | Default mode |
|-------------------------------|--------------|
| Read / render                 | best-effort  |
| `--output=human` extract      | best-effort  |
| Mutation submit               | strict       |
| `--dry-run` preview           | strict       |

Override per call:

```sh
jira issue create ... --adf-strict        # any lossy step → exit 3
jira issue create ... --adf-best-effort   # degrade silently with warnings
```

Or globally: `JIRA_ADF_STRICT=true` env, or `adf_strict = true` in the
profile TOML. Precedence: flag > env > profile > per-path default.

### Opaque preservation

Unknown ADF nodes/marks (anything outside the CLI's MVP set) round-trip
through the CLI byte-equivalently — the typed model preserves them via
opaque passthrough. **However**: Jira's create endpoint validates the
posted document against its own ADF schema and will reject truly unknown
node types with `INVALID_INPUT (400)`. The opaque path is for
preserving fidelity on read; submit paths are bounded by what Jira
itself accepts.

## jql_reference
When to use this: JQL is the query language behind → `search_jql` and → `list_issues`. Use this section as a lookup for the operators, keywords, functions, and recipes that compose into a query string. For the `jira jql build` flag-driven constructor and `jira search saved`, see → `search_jql`.

Authoritative Atlassian docs (cite these — link rendering is honest):

- [JQL fields](https://support.atlassian.com/jira-service-management-cloud/docs/jql-fields/)
- [JQL operators](https://support.atlassian.com/jira-service-management-cloud/docs/jql-operators/)
- [JQL keywords](https://support.atlassian.com/jira-service-management-cloud/docs/jql-keywords/)
- [JQL functions](https://support.atlassian.com/jira-service-management-cloud/docs/jql-functions/)
- [JQL developer status](https://support.atlassian.com/jira-service-management-cloud/docs/jql-developer-status/)
- [JQL advanced-roadmap fields](https://support.atlassian.com/jira-service-management-cloud/docs/search-for-advanced-roadmaps-custom-fields-in-jql/)

### Common operators

| Operator                | Meaning                                  |
|-------------------------|------------------------------------------|
| `=`  /  `!=`            | exact match                              |
| `in (a, b, c)`          | match any of                             |
| `not in (a, b, c)`      | match none of                            |
| `~` / `!~`              | text match (string fields)               |
| `>`  `>=`  `<`  `<=`    | numeric / date comparison                |
| `is empty` / `is not empty` | null check (some fields use `EMPTY`) |
| `was`                   | historical value (combined with `during(...)`) |
| `changed`               | value transitioned (combined with `from`/`to`/`by`/`during`) |

### Common keywords

| Keyword | Meaning                              |
|---------|--------------------------------------|
| `AND`   | all conditions must match            |
| `OR`    | any condition may match              |
| `NOT`   | invert the condition                 |
| `ORDER BY <field> ASC|DESC` | sort the result set      |

### Common functions

```text
currentUser()              # the calling user's accountId
now()                      # current timestamp
startOfDay() / endOfDay()  # boundary helpers (also Week/Month/Year)
membersOf("group-name")    # members of a Jira group
componentsLeadByUser()     # components led by current user
projectsLeadByUser()
linkedIssues(KEY [, "blocks"])   # find issues linked to KEY by a link type
issuesWithText("phrase")
```

### High-yield recipes

```sh
# Everything assigned to me, not done
jira search jql 'assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC' --output=json

# In-flight issues in a specific project
jira search jql 'project = KAN AND status = "In Progress"' --output=json

# Bugs reported in the last sprint
jira search jql 'project = KAN AND issuetype = Bug AND created > startOfMonth()' --output=json

# Issues in any of my epics
jira search jql 'project = KAN AND parent in (linkedIssues(currentUser()))' --output=json

# Recently updated, with a specific label
jira search jql 'project = KAN AND labels = "regression" AND updated > -7d' --output=json

# Issues blocked by a specific issue
jira search jql 'issue in linkedIssues("KAN-72", "is blocked by")' --output=json

# Subtasks of a parent
jira search jql 'parent = KAN-69' --output=json

# Status-history check (was = 'In Progress' some time recently)
jira search jql 'status was "In Progress" during ("2026-04-01", "2026-05-01")' --output=json
```
