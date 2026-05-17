# Auth

Profiles live in `~/.config/jira-cli/config.toml`. The config stores metadata:
base URL, profile name, auth type, and backend references. It does not store the
Jira credential.

## Login

```sh
jira auth login
jira auth status
jira auth whoami --save
```

`auth whoami --save` calls Jira's `/myself` endpoint and stores the resolved
`account_id` on the active profile. That lets `--assignee me` resolve without a
fresh identity lookup.

## Headless Setup

```sh
echo "$JIRA_TOKEN" | jira auth login --no-input \
  --profile-name work \
  --base-url https://company.atlassian.net \
  --email dev@example.com \
  --auth-type token \
  --secret-stdin
```

`--credential-env NAME` reads a credential from an environment variable.
`--secret-stdin` and `--credential-env` are mutually exclusive.

## Backends

| Backend | Use |
|---------|-----|
| `keyring` | Default OS keyring storage. |
| `1password` | SDK-backed 1Password item storage. |
| `JIRA_TOKEN_<PROFILE>` | Environment override checked before stored credentials. |

The 1Password backend uses the official Go SDK. Release archives are built
without CGO, so 1Password-backed profiles require a CGO-enabled source build or
an environment override.

For desktop-app auth, install 1Password, sign in to the account that owns the
item, then enable Integrate with other apps under Settings > Developer. If you
want biometric approval, enable the OS unlock option under Settings > Security.
Without the desktop app integration setting, the SDK-backed backend cannot read
the item.

## Further reading

- [1Password SDK desktop app integration](https://www.1password.dev/sdks#1password-desktop-app)

## Scopes

The README lists the current Jira Cloud scopes for the command surface. Jira
Server and Data Center PATs inherit user permissions instead of granular Cloud
scopes.
