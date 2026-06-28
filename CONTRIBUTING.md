# Contributing

Notes for working on jira itself: building from a checkout, running the local CI gauntlet, and the tooling expectations on commits.

End users don't need anything here. If you just want a working binary, see the [installation guide](https://matcra587.github.io/jira-cli/installation/).

## Before you start

For anything substantial, open an issue before you build. I may have opinions on priority, scope, or how it's implemented, and it's better to sort that out up front than after you've written the PR.

Even for an obvious fix (a typo, a doc correction, a clear bug), raise an issue and the PR that closes it.

Two things hold for every change: CI must pass (`mise run ci`), and review comments get addressed before merge.

## Prerequisites

*   [mise](https://mise.jdx.dev) provisions the pinned toolchain (Go, the linters, everything else). Nothing else to install by hand.
*   A C toolchain, **only** if you want the `1password` credential backend, which needs CGO. Everything else builds with `CGO_ENABLED=0`.

## Setup

```sh
gh repo clone matcra587/jira-cli
mise --cd jira-cli run install --locked
jira version
```

`--cd` runs in the freshly cloned directory, so there's no separate `cd`. `mise run install` auto-provisions the pinned toolchain before the task (`--locked` requires it to be pre-resolved in `mise.lock`), then runs `go install ./cmd/jira` with the version ldflags set, so `jira version` reports a real build that plain `go install` can't. A source build is also the only way to get the `1password` backend, which needs the CGO that release archives don't ship.

> [!TIP]
> `mise run build` writes a release-shaped binary to `./dist/jira-<goos>-<goarch>`. Use it to inspect the exact shape the release pipeline produces.

## Project layout

*   `cmd/jira/` holds the command wiring, the embedded agent guide, and the manpage source.
*   `internal/` is the runtime: config, credential backends, output rendering, the mutation pipeline, the editor, and the TUI.
*   `internal/jira/` is the Jira REST client and its typed services.
*   `internal/adf/` parses, validates, and renders Atlassian Document Format.
*   `tests/` holds the `contract`, `integration`, `unit`, and `guardrails` suites.
*   `tests/live/` is the `live`-tagged end-to-end suite, excluded from `go test ./...` and run with `mise run test:live`.

## Tests and checks

One task runs the full local equivalent of CI. Run it before you push or open a PR; the same checks run in GitHub Actions on every PR:

```sh
mise run ci
```

It chains `check` (fmt, lint, rumdl, vet, unit tests), `test:integration`, `go mod tidy`, the GitHub Actions workflow linter, and the security scan.

> [!WARNING]
> `mise run ci` is macOS/Linux only for now. The full `ci` task currently fails on Windows; run it on macOS, Linux, or WSL before pushing.

For tighter loops while editing:

| Task | What it does |
|---|---|
| `mise run check` | fmt-check + lint + rumdl + vet + unit tests |
| `mise run test` | unit tests with coverage (fast) |
| `mise run test:integration` | integration tests |
| `mise run test:live` | live end-to-end suite against a real tenant; needs a configured profile and `JIRA_LIVETEST_PROJECT` (excluded from `go test ./...`) |
| `mise run fix` | auto-apply fmt + lint + rumdl fixes |

Run a single package or test with `go test` directly:

```sh
go test ./internal/adf/... -run TestValidate
```

## Code style

`gofumpt` for formatting, `golangci-lint` for the rest, `rumdl` for Markdown. `mise run fix` applies every auto-fix in one go; `mise run check` is the read-only gate CI runs.

US spelling in code, comments, and identifiers: the linter's misspell rule is locale `US`, so British spellings (`behaviour`, `colour`) fail the build.

## Opening a pull request

1.  Fork the repo and branch off `main`.
2.  Make your change, with tests.
3.  Run `mise run ci` and make sure it passes (`mise run fix` first to auto-fix formatting and lint).
4.  Push and open a PR against `main`.

Before you open it:

*   **Use a conventional-commit PR title.** PRs are squash-merged, so the PR title becomes the single commit on `main`: `feat(issue): …`, `fix(adf): …`, `docs: …`, `chore: …`. (Branch commit messages get squashed away, but the pre-commit hook still checks their format.)
*   **Sign your commits.** `git commit -S` (SSH or GPG). Signed commits are required on every PR.
*   **Keep spec-tool jargon out of messages.** spec-kit, superpowers and the like generate internal IDs for plans and tasks (`T046`, `FR-011`, `RED→GREEN`). Those mean nothing in a `git log`, so leave them out of commit messages and the PR title.

By opening a PR, you agree your contribution is released under the project's [MIT licence](LICENSE).

## AI-assisted contributions

jira-cli is built for agents, so AI-assisted PRs are welcome. The bar is the same either way: you own what you submit. Read the diff, understand it, make sure it passes CI, and don't open a PR you couldn't explain yourself. Flag significant AI assistance in the PR description.

## Pre-commit hooks

**Optional**, but it catches what CI checks locally, before you raise the PR. The repo ships an [`hk`](https://github.com/jdx/hk) hook configuration in `hk.pkl`; on commit it runs `gofumpt`, `go vet`, `golangci-lint`, and `rumdl`, plus `actionlint` and `zizmor` over the GitHub Actions workflows, and checks the message is a conventional commit. Install it once per checkout:

```sh
hk install
```

The hook globs the working tree, not just staged files, so a stale file elsewhere in the repo can fail the commit.

## Docs site

Source pages live in `docs/`. The command reference under `docs/reference/` is generated from the Cobra command tree at build time and `.gitignore`d; never edit it by hand.

```sh
mise run docs:serve   # live-reload at http://localhost:8000
mise run docs:build   # strict build (fails on any broken link or anchor)
```

Both tasks run `docs:gen` first, so the reference always matches the current command tree. Pass `--dev-addr 0.0.0.0:8000` to `docs:serve` to reach it from another device on your LAN.

## Getting help

Stuck, or not sure whether something's a bug? Open an [issue](https://github.com/matcra587/jira-cli/issues). Questions are welcome, not just bug reports. For a bug, include your `jira version` output, your OS, and the steps to reproduce.
