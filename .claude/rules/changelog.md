---
description: >
  Changelog fragments for agents: every user-facing feat/fix ships a changie
  fragment, created non-interactively; the commit-msg gate that enforces it;
  the Changelog: skip escape (CI/workflow and other non-user-facing changes);
  and how fragments become CHANGELOG.md. Applies to any change, since the gate
  fires on the commit type, not the files touched.
---

# Changelog

User-facing changes are recorded as [changie](https://changie.dev) fragments
under `.changes/unreleased/`, assembled into root `CHANGELOG.md` at release
time. `docs/changelog.md` is a one-line snippet include of `CHANGELOG.md`.

*   **Any user-facing `feat`/`fix` needs a fragment.** Create it
    **non-interactively** — you cannot answer `changie new`'s prompts — and
    stage it, as part of the change:

    ```sh
    changie new -k <added|changed|fixed> -b "<user-visible outcome>" --interactive=false
    git add .changes/unreleased/
    ```

    `-k` takes the lowercase kind **key** (not the label): `added`, `changed`,
    `fixed`, `breaking`, `removed`, `deprecated`, `security`, `dependencies`. The
    release notes render the label (`### Added`, `### Breaking Changes`). Use
    `dependencies` for dependency-version bumps (renders under `### Dependencies`).

    That is the changie binary, so it behaves the same under PowerShell, bash,
    or sh. The gate itself runs automatically at commit time (the `check-changie`
    mise task, a cross-platform Go program) — you don't invoke it yourself.
*   **Write the body for the release-notes reader**: the user-visible outcome,
    imperative, no emoji, no commit SHAs or PR numbers, scope prefix stripped.
    `CHANGELOG.md` and the `.changes/v*.md` files are the style exemplar.
*   **Not user-facing?** A GitHub Actions/CI workflow, an internal refactor, a
    test-only fix, a build or tooling tweak — none of these get a fragment.
    Prefer the matching non-`feat`/`fix` commit type (`ci`, `chore`, `docs`,
    `build`, `style`, `refactor`); the gate skips those automatically. Only if
    the change is genuinely a `feat`/`fix` yet still not user-facing do you add a
    `Changelog: skip` trailer. Changes under `.github/` almost never need one.
*   **Never hand-edit `CHANGELOG.md`** (changie assembles it), and never bypass
    the gate with `git commit --no-verify`. Releases at or before v0.7.7 are
    frozen as `.changes/v*.md` files; don't rewrite them.
