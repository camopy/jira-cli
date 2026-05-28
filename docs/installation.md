# Installation

`jira` ships as a single static Go binary. Pick the tab for your platform in
[Quick start](#quick-start), or jump straight to a method:

*   [One-line install](#one-line-install), `curl | sh` on Linux & Apple Silicon macOS.
*   [Homebrew](#homebrew), recommended on macOS (Apple Silicon) and Linux.
*   [Pre-built binaries](#pre-built-binaries), checksummed, signed tarballs.
*   [`go install`](#go-install), any Go platform supported by jira's dependencies.

Working on jira itself? Build from a checkout instead, see
[Contributing › Building from source](contributing.md#building-from-source).

## System requirements

jira runs on:

*   **Operating system**:
    *   macOS 13.0+ on Apple Silicon (arm64).
    *   Any glibc-based Linux on amd64 or arm64 (Ubuntu, Debian, Fedora,
        RHEL, Arch, openSUSE, …).
    *   WSL 2 (treat as Linux for install purposes).
*   **Hardware**: any x86-64 or arm64 CPU. The release binary is a
    CGO-disabled static Go build under 30 MB.
*   **Shell**: Bash, Zsh, or Fish.

!!! warning "Native Windows is not supported"
    The upstream `gechr/clog` dependency uses Unix-only signals
    (`syscall.SIGWINCH`), so all install paths fail on `GOOS=windows`.
    Use [WSL](https://learn.microsoft.com/windows/wsl/) and follow the
    Linux instructions.

!!! note "musl-based Linux (Alpine)"
    Release archives are statically linked (CGO disabled), so the
    binary runs on Alpine and other musl distros. Confirmed by spot
    checks; if it ever stops working, please open an issue.

## Platform support

Pre-built archives and the Homebrew formula cover Linux (amd64/arm64) and
Apple Silicon macOS. Intel macOS installs through `go install` or a source
build.

| Platform | Homebrew | Pre-built tarball | `go install` |
|----------|:--------:|:-----------------:|:------------:|
| Linux · amd64 | ✓ | ✓ | ✓ |
| Linux · arm64 | ✓ | ✓ | ✓ |
| macOS · Apple Silicon (arm64) | ✓ | ✓ | ✓ |
| macOS · Intel (amd64) | - | - | ✓ |

!!! note "No Intel-macOS binary"
    The release pipeline builds `linux/amd64`, `linux/arm64`, and
    `darwin/arm64` only. On Intel macOS, use [`go install`](#go-install)
    or `brew install --HEAD matcra587/tap/jira` (which compiles from source
    via the Homebrew Go toolchain).

## Quick start

=== "Linux"

    **Recommended**, Homebrew on amd64/arm64. You get `brew upgrade jira`
    for managed updates:

    ```sh
    brew install matcra587/tap/jira
    ```

    No Homebrew? The one-line installer drops the binary into
    `$HOME/.local/bin`. Re-run it to upgrade in place:

    ```sh
    curl -fsSL https://matcra587.github.io/jira-cli/install.sh | sh
    ```

=== "macOS"

    **Recommended**, Homebrew on Apple Silicon. You get `brew upgrade jira`
    for managed updates:

    ```sh
    brew install matcra587/tap/jira
    ```

    No Homebrew? The one-line installer drops the binary into
    `$HOME/.local/bin`. Re-run it to upgrade in place:

    ```sh
    curl -fsSL https://matcra587.github.io/jira-cli/install.sh | sh
    ```

    !!! note "Intel macs"
        No `darwin/amd64` bottle or tarball is published. Use [`go
        install`](#go-install) or `brew install --HEAD
        matcra587/tap/jira`.

=== "Windows"

    Native Windows is not supported, see
    [System requirements](#system-requirements). Use
    [WSL](https://learn.microsoft.com/windows/wsl/) and follow the Linux
    tab above.

## Verify the install

```sh
jira version
```

Prints the installed version, commit, branch, build time, and `built by`
(one of `goreleaser`, `homebrew`, or the local user for a dev build).

If `jira: command not found`, the binary isn't on `PATH`. Homebrew handles
this for you; the one-line installer prints a PATH-fix hint; for `go install`
make sure `$(go env GOBIN)` (or `$(go env GOPATH)/bin`) is on your `PATH`.

The install methods below cover the alternatives to Quick start, pick one if
Homebrew or the one-line installer don't fit your setup.

## Homebrew

Recommended on macOS (Apple Silicon) and Linux (amd64/arm64).

```sh
brew install matcra587/tap/jira
```

The formula lives in [`matcra587/homebrew-tap`](https://github.com/matcra587/homebrew-tap)
and tracks the latest GitHub release. Upgrades come through `brew upgrade
jira`.

!!! tip "Source build via Homebrew"
    `brew install --HEAD matcra587/tap/jira` compiles the latest `main`
    with the Go toolchain and embeds version metadata. This is the one
    Homebrew path that works on Intel macOS.

## go install

```sh
go install github.com/matcra587/jira-cli/cmd/jira@latest
```

Installs the latest tagged release into `$(go env GOBIN)` (or
`$(go env GOPATH)/bin`).

!!! warning "Version metadata"
    This path embeds limited version metadata, `jira version` reports `dev`
    or git-derived values because the version package relies on compile-time
    `-X` ldflag overrides that `go install` does not supply. For accurate
    `jira version` output, use Homebrew or pre-built binaries, or build from
    a checkout (see
    [Contributing › Building from source](contributing.md#building-from-source)).

## One-line install

For Linux (amd64/arm64) and Apple Silicon macOS, a POSIX-sh installer hosted
on this site does the download, verify, and install in one step. It
auto-detects OS and architecture, resolves the release tag, verifies the
SHA-256 against the published `checksums.txt`, optionally verifies the cosign
signature when `cosign` is on `PATH`, and installs the binary into
`$HOME/.local/bin`, printing a PATH-fix hint if that directory isn't already
on your shell's `PATH`.

To install the **latest** release:

=== "macOS · Linux · WSL"

    ```sh
    curl -fsSL https://matcra587.github.io/jira-cli/install.sh | sh
    ```

=== "Windows (native)"

    Native Windows is not supported (the upstream `gechr/clog` dependency
    uses Unix-only signals). Use [WSL](https://learn.microsoft.com/windows/wsl/)
    and follow the macOS · Linux · WSL tab.

To install a **specific version**, pass the release tag positionally:

=== "macOS · Linux · WSL"

    ```sh
    curl -fsSL https://matcra587.github.io/jira-cli/install.sh | sh -s v0.5.9
    ```

=== "Windows (native)"

    See above, use WSL.

!!! tip "Prefer Homebrew for managed upgrades"
    The one-line installer has no upgrade command of its own, re-run it to
    pick up the latest release. If you want `brew upgrade jira` style
    managed updates, install with [Homebrew](#homebrew) instead.

??? info "Environment overrides"

    The positional version argument is the common case; everything else is
    controlled by environment variables.

    | Variable | Effect |
    |---|---|
    | `JIRA_CLI_VERSION` | Pin a release tag (same effect as the positional argument). The positional argument wins when both are set. |
    | `JIRA_CLI_INSTALL_DIR` | Install directory. Default: `$HOME/.local/bin`. Use `/usr/local/bin` for a system-wide install; see the example below. |
    | `JIRA_CLI_NO_VERIFY` | Set to `1` to skip cosign verification when cosign is unavailable. SHA-256 still runs. |

    Example, system-wide install:

    ```sh
    curl -fsSL https://matcra587.github.io/jira-cli/install.sh | JIRA_CLI_INSTALL_DIR=/usr/local/bin sudo -E sh
    ```

!!! warning "What `curl | sh` actually runs"
    The installer's source lives at [`docs/install.sh`](https://github.com/matcra587/jira-cli/blob/main/docs/install.sh), about 130 lines of POSIX sh.
    Inspect it before piping into `sh` if you don't already trust the source:

    ```sh
    curl -fsSL https://matcra587.github.io/jira-cli/install.sh | less
    ```

## Pre-built binaries

GitHub Releases ship checksummed tarballs for Linux on amd64/arm64 and macOS
on arm64 (Apple Silicon). The [one-line installer](#one-line-install) wraps
the download/verify/install dance. If you'd rather do it by hand,
for a scripted pipeline, a stricter audit trail, or because you don't want
to pipe `curl` into `sh`, the manual recipe is below.

The archive name format is:

```text
jira_<version>_<os>_<arch>.tar.gz
```

??? note "Manual download + SHA-256 verify"

    === "bash · zsh"

        ```bash
        VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
            https://github.com/matcra587/jira-cli/releases/latest | sed 's#.*/tag/##')
        OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

        curl -fsSLO "https://github.com/matcra587/jira-cli/releases/download/${VERSION}/jira_${VERSION#v}_${OS}_${ARCH}.tar.gz"
        curl -fsSLO "https://github.com/matcra587/jira-cli/releases/download/${VERSION}/checksums.txt"
        grep "_${OS}_${ARCH}.tar.gz$" checksums.txt | sha256sum -c

        tar xzf "jira_${VERSION#v}_${OS}_${ARCH}.tar.gz"
        sudo install jira /usr/local/bin/
        ```

    === "fish"

        ```fish
        set VERSION (curl -fsSLI -o /dev/null -w '%{url_effective}' \
            https://github.com/matcra587/jira-cli/releases/latest | string replace -r '.*/tag/' '')
        set OS (uname -s | string lower)
        set ARCH (uname -m | string replace x86_64 amd64 | string replace aarch64 arm64)
        set NUM (string replace -r '^v' '' $VERSION)
        set TARBALL (printf 'jira_%s_%s_%s.tar.gz' $NUM $OS $ARCH)

        curl -fsSLO "https://github.com/matcra587/jira-cli/releases/download/$VERSION/$TARBALL"
        curl -fsSLO "https://github.com/matcra587/jira-cli/releases/download/$VERSION/checksums.txt"
        grep (printf '_%s_%s.tar.gz$' $OS $ARCH) checksums.txt | sha256sum -c

        tar xzf $TARBALL
        sudo install jira /usr/local/bin/
        ```

??? note "Cosign signature verification"

    `checksums.txt` is signed with [cosign](https://github.com/sigstore/cosign)
    keyless signing, a `checksums.txt.sigstore.json` bundle ships next to
    the release artifacts. The [one-line installer](#one-line-install)
    verifies it automatically when `cosign` is on `PATH`. To verify by hand:

    === "bash · zsh"

        ```bash
        curl -fsSLO "https://github.com/matcra587/jira-cli/releases/download/${VERSION}/checksums.txt.sigstore.json"
        cosign verify-blob \
            --bundle checksums.txt.sigstore.json \
            --certificate-identity-regexp "https://github.com/matcra587/jira-cli/" \
            --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
            checksums.txt
        ```

    === "fish"

        ```fish
        curl -fsSLO "https://github.com/matcra587/jira-cli/releases/download/$VERSION/checksums.txt.sigstore.json"
        cosign verify-blob \
            --bundle checksums.txt.sigstore.json \
            --certificate-identity-regexp "https://github.com/matcra587/jira-cli/" \
            --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
            checksums.txt
        ```

    The archives themselves are not individually signed; the checksum bundle
    covers them transitively.

!!! tip "1Password-backed profiles need CGO"
    Release archives, the one-line installer, Homebrew, and `go install` all
    produce CGO-disabled binaries. The `1password` credential backend needs
    CGO at compile time, to use it, [build from source](contributing.md#building-from-source)
    or set `JIRA_TOKEN_<PROFILE>` and skip the backend.

## Authenticate

```sh
jira config init --base-url https://example.atlassian.net --email john.doe@example.com
jira auth login         # stores the API token (interactive on a TTY)
jira auth status        # confirms the credential is valid
```

See [auth](auth.md) for non-interactive setup, the 1Password backend, the
`JIRA_TOKEN_<PROFILE>` environment override, and the full subcommand
reference.

## Update jira

| Install path | Upgrade |
|---|---|
| Homebrew | `brew upgrade jira` |
| One-line installer | Re-run the installer command from [One-line install](#one-line-install). |
| go install | `go install github.com/matcra587/jira-cli/cmd/jira@latest` |
| Pre-built | Re-download from [releases](https://github.com/matcra587/jira-cli/releases) |

!!! info "No self-updater"
    jira does not currently ship a `jira update` self-updater and none of
    the install paths auto-update in the background. Upgrade through
    whichever path you installed with.

## Uninstall jira

Removing the binary doesn't drop stored credentials or your local config.
Clean those up explicitly if you're walking away from jira entirely.

=== "Homebrew"

    ```sh
    brew uninstall jira
    ```

=== "One-line installer / pre-built"

    ```sh
    rm -f "$HOME/.local/bin/jira"
    # or, if you installed with JIRA_CLI_INSTALL_DIR=/usr/local/bin:
    sudo rm -f /usr/local/bin/jira
    ```

=== "go install"

    ```sh
    rm -f "$(go env GOBIN)/jira" 2>/dev/null \
      || rm -f "$(go env GOPATH)/bin/jira"
    ```

!!! warning "Credentials and config are not removed"
    The binary uninstall leaves your stored API tokens in the OS keyring
    (or 1Password vault) and your config TOML on disk. To remove them:

    ```sh
    # 1. Drop the stored credential for every profile (one at a time):
    jira auth logout <profile>

    # 2. Remove the config file:
    rm -rf "${XDG_CONFIG_HOME:-$HOME/.config}/jira-cli"

    # 3. Remove the local cache:
    rm -rf "${XDG_CACHE_HOME:-$HOME/.cache}/jira-cli"
    ```

    If you've already uninstalled the binary, the keyring or 1Password
    item must be removed by hand through the OS keychain app or
    `op item delete`.

## Further reading

*   [README](https://github.com/matcra587/jira-cli#readme), quick start.
*   [`auth`](auth.md), once installed, authenticate a profile.
*   [`agent`](agent.md), embedded runbooks and schema for agent consumers.
