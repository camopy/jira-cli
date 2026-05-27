# Contributing

Notes for working on jira itself, building from a checkout, running the
local CI gauntlet, and the tooling expectations on commits.

End users should not need anything on this page. If you just want a working
binary, see [Installation](installation.md).

## Building from source

A source checkout is the right path when you need any of:

*   The `1password` credential backend (requires CGO, which release archives
    don't ship). See [auth › Backends](auth.md#backends).
*   Accurate `jira version` output (`go install` doesn't supply the
    compile-time ldflags that the version package reads).
*   To hack on the code.

```sh
git clone https://github.com/matcra587/jira-cli
cd jira-cli
mise install                  # provision the pinned toolchain
mise run install              # go install ./cmd/jira with full version metadata
jira version
```

`mise install` reads `mise.toml` and installs the pinned Go toolchain, the
linters, and everything else the repo expects. `mise run install` runs
`go install ./cmd/jira` with the version ldflags set, so `jira version`
reports a real build.

!!! tip "Release-style artifact"
    `mise run build` writes a release-shaped binary to
    `./dist/jira-<goos>-<goarch>`. Use it when you want to inspect the
    shape of the artifact the release pipeline produces.

## Local CI

A single task runs the full local equivalent of CI:

```sh
mise run ci
```

It chains `check` (fmt, lint, rumdl, vet, unit tests), `test:integration`,
`go mod tidy`, the GitHub Actions workflow linter, and the security scan.
Run it before pushing, protected branches enforce it on the remote.

For tighter feedback loops during edits:

| Task | What it does |
|---|---|
| `mise run check` | fmt + lint + rumdl + vet + unit tests |
| `mise run test` | unit tests only (fast) |
| `mise run test:integration` | integration tests |
| `mise run test:live` | live-tagged end-to-end suite against a real Jira tenant (excluded from `go test ./...`) |
| `mise run fix` | auto-apply fmt + lint + rumdl fixes |
| `mise run rumdl` | markdown lint (check) |
| `mise run rumdl:fix` | markdown lint (fix) |

## Pre-commit hooks

The repo ships a [`hk`](https://github.com/jdx/hk) pre-commit hook
configuration in `hk.pkl`. On every commit it runs `gofumpt`, `go vet`,
`golangci-lint`, `rumdl`, a secret scanner, and a conventional-commit-format
check. The hook globs the working tree (not just staged files), so a stale
file elsewhere in the repo can fail the commit.

Install hooks once per checkout:

```sh
hk install
```

## Docs site

Source pages live in `docs/`. The command reference under `docs/reference/`
is generated at build time and `.gitignore`d, never edit it by hand.

```sh
mise run docs:serve   # live-reload at http://localhost:8000
mise run docs:build   # strict zensical build (matches CI)
```

`docs:build` regenerates `docs/reference/` from the live Cobra command tree
before each build, so the published reference always matches `main`.

## Commit hygiene

*   **Conventional commits.** `feat(scope): …`, `fix(scope): …`,
    `docs(scope): …`, `chore(scope): …`. The pre-commit hook rejects
    anything else.
*   **Signed commits.** Use `git commit -S` (the repo expects GPG or SSH
    signatures on `main`).
*   **No spec-kit artifacts in messages.** Don't reference task IDs
    (`T046`), requirement codes (`FR-011`), or workflow jargon
    (`RED→GREEN`). A `git log` reader shouldn't need the plan or spec to
    understand the change.
