---
slug: write-rich-text
title: Author rich text without losing content
description: Markdown-to-ADF conversion, the strict and best-effort modes, and when to write native ADF JSON instead.
when_to_use: Writing a description, comment, or worklog body with formatting, or when a conversion warns about lossy content.
commands: [jira adf convert, jira adf render]
order: 9
---

## Decide

Jira stores rich text as ADF (Atlassian Document Format), a JSON tree.
Three ways in, in order of effort:

*   Plain paragraphs, lists, code blocks, links → `--markdown` on the
    mutation itself. Fine for most bodies.
*   Unsure what your Markdown becomes → convert it first with
    `jira adf convert` and inspect the JSON.
*   Content Markdown cannot express (panels, mentions, complex tables) →
    write native ADF and pass it with `--json-input`.

Pasted Jira wiki markup is accepted too: the converter detects and
normalizes it, recording a `markdown_dialect_normalized` warning so you
know the input was not CommonMark.

Conversion is lossy by design. Two modes decide what happens to content
that does not survive: strict fails the command, best-effort keeps going
and records a warning. Mutations default to strict, reads to best-effort —
the safe direction each way. Override with `--adf-strict` /
`--adf-best-effort` or `JIRA_ADF_STRICT`.

## Run

```sh
# See exactly what ADF your Markdown becomes — no Jira call
jira adf convert --input body.md

# Read a stored ADF body back as Markdown (lossy)
jira adf render --input body.json

# Force the failure mode you want on a mutation
jira issue comment add PROJ-123 --markdown-file body.md --adf-strict --no-input
```

`jira agent adf-matrix` lists every ADF node the CLI can author, render,
or round-trip, when you need the support table itself.

## Save

*   The converted ADF document — reusable as a `--json-input` body.
*   `lossy_adf_conversion` warnings — each names the content that would be
    dropped or flattened.

## Preconditions

None for convert and render; they are local. The mutation carrying the
body has its own gates (`safe-mutation`).

## Recover

*   Strict conversion failed → the error names the unsupported construct.
    Either simplify the Markdown or author that part as native ADF.
*   Best-effort produced a mangled body → the warning said what was lost;
    round-trip with `jira adf render` to see what survived.
*   Hand-written ADF rejected → validate locally first: `jira adf convert`
    output is known-good ADF to compare shapes against, and every mutation
    validates the document before submission — a `--dry-run` catches
    invalid nodes and mark combinations without contacting Jira.

## Next

*   `shape-issues`, `annotate-issues` — the mutations these bodies feed.
