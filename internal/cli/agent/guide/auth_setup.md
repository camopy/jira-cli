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
- One of `token`, `basic`, `pat`, `mtls`. Anything else returns exit 3 — no fake authenticated profile is stored.

# command shape
- First-time TTY: bare `jira auth login` walks profile name → base URL → email → auth type → backend → credential prompt (reads stdin without echoing).
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
- `auth login` in a TTY pre-fills profile metadata from the active/configured profile, including `base_url`, `email`, and backend-specific 1Password fields.
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
