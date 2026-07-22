---
slug: bootstrap
title: Go from a fresh machine to a working profile
description: Configure authentication, verify the identity, and set the default project so later commands need no targeting flags.
when_to_use: First run on a machine, auth or profile errors (exit 1), or switching tenants or accounts.
commands: [jira auth login, jira auth status, jira auth whoami, jira me, jira config set, jira alias set]
order: 2
---

## Decide

Pick the secret backend before logging in: `keyring` (OS keyring, the
default), `1password`, or `env` (credential read from an environment
variable at run time — nothing stored). Agents on shared or ephemeral
machines usually want `env`.

The token itself is a Jira Cloud API token tied to an account email.
Classic and scoped (granular) tokens both work — the flavor is detected
automatically at login, no flag needed.

## Run

```sh
# Headless login, credential piped on stdin
printf '%s' "$JIRA_TOKEN" | jira auth login --profile-name work \
  --base-url https://example.atlassian.net --email dev@example.com \
  --backend keyring --secret-stdin --no-input

# Or point the profile at an environment variable instead of storing anything
jira auth login --profile-name work --base-url https://example.atlassian.net \
  --email dev@example.com --backend env --credential-env JIRA_API_TOKEN --no-input

# Verify the profile resolves to the expected account
jira me

# Persist the account id once — `me` shorthands and ADF mentions resolve
# against it
jira auth whoami --save

# Stop repeating --project on every command
jira config set default_project PROJ

# Shorten invocations you repeat; aliases expand locally, never shadow
# a built-in, and never contact Jira
jira alias set mine "issue mine --status '<Done'"
```

## Save

From `jira me`: the `account_id` and display name — proof the credential
maps to the intended user, and the account id some payloads need.

## Preconditions

A Jira Cloud base URL, account email, and API token. Nothing else — guide
and schema discovery work before any of this exists.

## Recover

*   Exit 1 from any command → `jira auth status` says which profile and
    backend were tried; re-run `jira auth login` for that profile.
*   Login verifies the credential against `/myself` by default; a failure
    there means a bad token or email, not a storage problem
    (`--skip-verify` defers that check).
*   Wrong tenant or account → `jira auth switch` changes the active
    profile; `-P <name>` overrides per invocation.

## Next

*   `discover` — learn the tenant's projects, types, and fields.
*   `core-contract` — how auth failures surface in the envelope.
