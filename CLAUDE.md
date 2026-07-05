# jira-cli

Terminal-first Go/Cobra CLI for Jira Cloud, built for developer and agent
workflows. The headless commands are the stable surface; `jira tui` is the
alpha full-screen dashboard.

## Commands

*   `mise run check` (fmt + lint + rumdl + vet + unit) · `mise run test` ·
    `mise run test:integration` · `mise run fix` (auto-fixes)
*   `mise run ci` before proposing a change (macOS/Linux/WSL only — it fails
    on Windows)
*   `mise run test:live` — end-to-end against a real tenant's probe project
    (`JIRA_LIVETEST_PROJECT`)

## Architecture

*   **`internal/cli/<domain>/`** — cobra commands (issue, epic, auth, config,
    …). Flags register through `cmdutil.Add*` so clib metadata is mandatory;
    success output returns through `cmdutil.WriteEnvelope`.
*   **Output contract** — one `--output` flag (`auto|human|json|compact`);
    `auto` resolves agent env → compact, non-TTY → json. Envelopes on stdout
    via `cli.WriteEnvelope` only; human status on stderr through clog.
*   **`internal/jira/`** — REST client + typed services; read-only mode blocks
    writes at the transport. **`internal/adf/`** — registry-driven ADF
    parse/validate/render.
*   **Mutations** route through the validate-and-encode pipeline; `--dry-run`
    is a local-only preview and never contacts Jira.
*   The embedded agent guide and schema (`jira agent guide`,
    `jira agent schema`) are the authoritative CLI behavior spec — update
    them with any behavior change; do not restate them elsewhere.

Area-specific conventions auto-load from `.claude/rules/`.

## Git

*   Conventional-commit PR titles (squash-merged); signed commits; `gh` CLI
    for PRs.
*   **Changelog:** a user-facing `feat`/`fix` needs a changie fragment, created
    **non-interactively** (you can't answer prompts) and staged before commit:
    `changie new -k <added|changed|fixed> -b "<user outcome>" --interactive=false`
    (lowercase kind key; also `breaking`, `removed`, `deprecated`, `security`,
    `dependencies`).
    The commit-msg hook enforces it; not user-facing → a `Changelog: skip`
    commit trailer. Details in [.claude/rules/changelog.md](.claude/rules/changelog.md).
