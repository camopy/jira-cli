---
title: Release notes
description: Read jira-cli's own changelog from the terminal with jira release-notes (alias rn) — full history, a single release, or the latest.
icon: material/note-text-outline
---

# :memo: Release notes

`jira release-notes` (alias `jira rn`) prints jira-cli's own changelog — the same
notes published on this site — so you can see what changed in the tool without
leaving the terminal. The changelog is embedded in the binary, so the command
reads nothing over the network and works offline.

This is jira-cli's changelog. It is unrelated to a Jira project's releases; to
list the issues in a Jira fixVersion, use [`search`](search.md).

## View the changelog

With no arguments, the command prints the full changelog, newest release first:

```sh
jira release-notes
jira rn
```

Narrow it to a single release by passing a version, or to the newest release
with `--latest`:

```sh
jira release-notes 0.7.7
jira release-notes --latest
```

A leading `v` is accepted — `0.7.7` and `v0.7.7` both resolve. An unknown version
fails and lists the versions that are available.

On an interactive terminal, a full history taller than the screen pages like
`git log`: a built-in pager by default (`q` quits), or your own via
`JIRA_PAGER`/`PAGER`. Pass `--no-pager` to print directly. Piped output,
machine modes, and agent sessions always stream straight through and never
wait on a pager.

## Output

Human output is Markdown. On an interactive terminal it is rendered through
glamour and styled to your theme, with each heading level in a distinct colour so
releases and change kinds are easy to tell apart. When the output is piped or
redirected it stays raw Markdown, so it drops straight into a file or a GitHub
release body:

```sh
jira release-notes --latest > NOTES.md
```

`--output=json` returns the notes as structured data instead of Markdown: a list
of releases, each with its `version`, `tag`, `url`, `date`, and `sections` of
change kinds. The example below shows the `data` block only — the envelope
wrapper and exit codes live on [Output](output.md).

```sh
jira release-notes --latest --output=json
```

```json
{
  "version": "0.7.7",
  "releases": [
    {
      "version": "0.7.7",
      "tag": "v0.7.7",
      "url": "https://github.com/matcra587/jira-cli/releases/tag/v0.7.7",
      "date": "2026-07-05",
      "sections": [
        { "kind": "Changed", "changes": ["Detect grapheme-clustering terminals …"] },
        { "kind": "Fixed", "changes": ["Keep issue-table columns aligned …"] }
      ]
    }
  ]
}
```

When the full changelog is requested (no version and no `--latest`), `version` is
omitted and `releases` holds every version, newest first.

The full flag and output-field tables live on the
[`release-notes` reference page](reference/jira/release-notes.md).
