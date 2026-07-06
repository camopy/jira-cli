## auth_setup
Goal: Wire a Jira profile to a valid credential — backend chosen, profile populated, secret never on disk in plaintext — before any other workflow can call Jira.
When: a fresh profile, a missing keyring entry, or a rotated token blocks any other call from succeeding (`auth_failed`, `credential not found`).

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
- Jira Cloud only, so `auth_type` is always `token` (set automatically — there is no `--auth-type` flag). This one type covers both classic and scoped (granular) Atlassian API tokens.

# token flavor (auto-detected — no flags)
- **Classic** token: authenticates at the site `base_url` (`https://<site>.atlassian.net/...`).
- **Scoped (granular)** token: authenticates the same way (HTTP Basic email+token) but the site host rejects it; it works only through the Atlassian gateway `https://api.atlassian.com/ex/jira/<cloud_id>/...`.
- The token string carries no type marker, so `auth login` detects it by trying it: verify against the site; on a 401/403, discover the `cloud_id` (unauthenticated `<base_url>/_edge/tenant_info`) and re-verify against the gateway. A gateway success means scoped — the `cloud_id` is stored on the profile and all later requests route through the gateway. There are NO `--scoped`/`--cloud-id` flags.

# command shape
- First-time TTY: bare `jira auth login` walks profile name → base URL → email → backend → credential prompt (reads stdin without echoing). Classic vs scoped is detected automatically.
- Headless (CI / agent): `--no-input` with explicit flags for every field. Secret feeds via `--secret-stdin` (keyring) or `--vault` + `--item` (1Password).

# guard
- Partial flags **merge** into the existing profile — fields not supplied retain their current values. To replace cleanly, pass every field.
- Bare interactive `jira auth login` pre-fills from the active/configured profile; explicit flags still win.

**Run**
- Preflight: `jira auth status --output=json`
- Interactive (TTY): `jira auth login`
- Headless, keyring backend:
  ```sh
  echo "$JIRA_TOKEN" | jira auth login --no-input \
    --profile-name work \
    --base-url https://company.atlassian.net \
    --email dev@example.com \
    --backend keyring \
    --secret-stdin
  ```
- Scoped (granular) token: identical command — the same headless invocation above auto-detects scoped (site 401 → tenant_info → gateway) and stores `cloud_id`. No extra flags.
- Headless, 1Password backend:
  ```sh
  jira auth login --no-input \
    --profile-name work \
    --base-url https://company.atlassian.net \
    --email dev@example.com \
    --backend 1password \
    --vault Engineering \
    --item jira-cli-work
  ```
- Switch active profile: `jira auth switch <profile>`
- Re-resolve credential from backend: `jira auth refresh`
- Move credential between backends: `jira auth migrate --backend 1password`
- Remove credential (keeps TOML metadata): `jira auth logout <profile>`
- Purge a credential whose profile was already deleted from config: `jira auth logout <profile> --base-url <site>` — the keychain entry is keyed by site host + profile name, so the flag supplies the half config no longer holds; without it a deleted/unknown profile is refused (`profile_not_found`).
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
- Desktop-app SDK authorization is per account and per process. Separate `jira` invocations may prompt separately even when the app is unlocked; use the system keychain backend for prompt-free day-to-day commands.
- For 1Password desktop-app integration, see Further reading below.

Further reading:

- [1Password SDK desktop app integration](https://www.1password.dev/sdks#1password-desktop-app)

**Behavior**
- `auth login --no-input` with **partial flags merges** into the existing profile. This protects against mistyped one-flag updates wiping unrelated fields like `email` or `account_id`. To replace cleanly, pass every field.
- `auth login` in a TTY pre-fills profile metadata from the active/configured profile, including `base_url`, `email`, `cloud_id`, and backend-specific 1Password fields.
- `auth_type` is always `token` (Jira Cloud); it covers classic and scoped tokens. The flavor is auto-detected at login (site probe, then gateway fallback) and recorded as the profile's `cloud_id` when scoped. `auth status` / `auth login` report `token_type` (`classic` | `scoped`). There are no scoped-specific flags.
- Secret hygiene contract (HTTP-transport-level — enforced once, not per command):
  - Secrets are **never** stored in the TOML config — only metadata (backend selector, vault, item ref).
  - All logging, including `--debug`, redacts `Authorization` headers and any field named `secret` / `token` / `api_token` / `cookie`.
  - CLI-written files containing credential metadata are mode `0600`.
  - `jira auth token` deliberately does NOT print the raw token — only length, prefix, and backend identity.

**Recover**
| Symptom | Cause | Next |
|---|---|---|
| Exit 1 on every call | Credential missing or expired | `jira auth status` → identify failing backend → `jira auth login --profile-name <name>` |
| Scoped token: 401/403 on `/myself` despite a valid token | Granular token missing required scopes | Add the read scopes Atlassian requires for `/myself` (`read:jira-user`, `read:user:jira`, `read:application-role:jira`, `read:group:jira`, `read:avatar:jira`) to the token at id.atlassian.com |
| Scoped token rejected at login (site 401, gateway not reached) | `_edge/tenant_info` blocked, so scoped can't be auto-detected | Set the id manually: `jira config set profiles.<name>.cloud_id <id>` (find it at `https://<site>.atlassian.net/_edge/tenant_info`) |
| `credential not found` | Backend has no entry for this profile | `jira auth login --profile-name <name>` |
| `OP_SERVICE_ACCOUNT_TOKEN not set` | 1Password service-account env missing | Export it, or fall back to keyring backend via `jira auth migrate --backend keyring` |
| Exit 3, `1Password backend requires a vault` / `requires an item` | `--backend 1password` headless without `--vault`/`--item` | Pass both `--vault` and `--item` — they form the secret reference and are validated up front, before any network call |
| 401 on a previously-working profile | Token revoked / rotated | `jira auth login --profile-name <name>` to replace |

**Next**
- Then: → `identity_setup` (run `jira auth whoami --save` to persist `account_id`)
- Then: → `configure_editor` (for the `jira issue edit` TTY flow, if relevant)
- Composes: → every other workflow inherits the active credential.
