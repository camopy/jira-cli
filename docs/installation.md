# Installation

`jira` ships as a Go binary. Homebrew and GoReleaser release archives embed
release version metadata; `go install` builds from source and may report
`dev` or git-derived metadata in `jira version`.

## Homebrew

```sh
brew install matcra587/tap/jira
```

The release workflow publishes the Homebrew formula to
`matcra587/homebrew-tap`.

## Go Install

```sh
go install github.com/matcra587/jira-cli/cmd/jira@latest
```

This installs the latest source build into `$(go env GOBIN)` or
`$(go env GOPATH)/bin`.

## Release Archives

GoReleaser builds macOS and Linux archives for amd64 and arm64. The archive
name format is:

```text
jira_<version>_<os>_<arch>.tar.gz
```

The release checksum file is signed with cosign.

## From Source

```sh
git clone https://github.com/matcra587/jira-cli
cd jira-cli
mise install
mise run build
./dist/jira-$(go env GOOS)-$(go env GOARCH) version
```

Use `mise run release:preflight` before publishing. It runs CI checks,
GoReleaser validation, and a snapshot release build.
