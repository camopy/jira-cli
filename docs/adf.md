---
title: ADF
description: Atlassian Document Format — the rich-text input for descriptions and comments, its validation modes, and the supported node and mark set.
icon: material/file-document-outline
---

# :page_facing_up: ADF

ADF is the canonical rich-text input for issue
descriptions, comments, and worklog comments.

## Native ADF

```json
{
  "type": "doc",
  "version": 1,
  "content": [
    {
      "type": "paragraph",
      "content": [
        { "type": "text", "text": "Description body." }
      ]
    }
  ]
}
```

Use it through JSON payloads:

```sh
jira issue create --no-input --json-input payload.json --output=json
jira issue comment add PROJ-123 --json-input adf.json --no-input --output=json
```

## Markdown input

Every rich-text surface also accepts GFM Markdown as a convenience layer
(`--markdown` inline, `--markdown-file` from a file or stdin, or a
`description_markdown` payload key) and converts it client-side.
Paragraphs, headings, bold/italic/strike, inline code, links, bullet and
ordered lists, task lists (`- [ ]` / `- [x]` become real Jira checkboxes),
fenced code blocks, blockquotes, tables, and horizontal rules convert
faithfully. A few constructs degrade by design:

| Markdown | Converts to | Strict mode |
|---|---|---|
| `![alt](url)` image | Alt-text link to the URL | Passes (non-lossy downgrade) |
| Blockquote inside a list item (`- >text`) | Quoted content hoisted into the item | Passes (non-lossy downgrade) |
| Decorative mark on inline code (``**`code`**``) | Code mark kept, decoration dropped | Passes (non-lossy downgrade) |
| Table inside a list item or blockquote | Table moved after the enclosing block | Passes (non-lossy downgrade) |
| Raw HTML | Dropped | **Fails** with the offending line |

Constructs with no Markdown spelling at all — mentions, panels, status
lozenges, cards — never enter the converter; author them as native ADF.

Conversion diagnostics are source-mapped: an error or warning names the
Markdown line and column and quotes the offending snippet, and failures carry
the stable `markdown_lossy_conversion` code.

### Jira wiki markup

Content pasted from Jira itself — old descriptions, Server/DC exports,
colleagues' comments — is often written in Jira's wiki markup rather than
Markdown. The Markdown paths detect that dialect and normalize it before
conversion, so `h2.` headings, `# item` ordered lists, `*bold*`,
`[text|url]` links, `{{monospace}}`, `||header||` tables, and
`{noformat}` / `{code:lang}` blocks all convert to their intended ADF
shapes, and emoticon shortcuts such as `(/)`, `(x)`, and `(!)` become real
emoji nodes.

Detection is all-or-nothing per document and Markdown always wins a tie: any
unambiguous Markdown construct — a code fence, a `##` heading, `**bold**`, a
`[text](url)` link — pins the whole input as Markdown and nothing is
rewritten, so pure Markdown converts exactly as it always has. When a
normalization does run, the envelope announces it with a non-lossy
`markdown_dialect_normalized` warning naming the rewritten constructs.
Wiki syntax inside code blocks and code spans always stays literal.

## Convert and lint

`jira adf convert` runs the same converter, normalizer, and validator the
mutation pipeline runs — without touching Jira — so rich text can be authored
and linted in isolation, then submitted anywhere `--json-input` is accepted.
`jira adf render` is the reverse projection for *reviewing* existing ADF as
Markdown; it is lossy by design and its output is never meant to be submitted
back.

Lint on its own — exit `0` means the submit cannot fail conversion — or chain
author → convert → submit in one pipe:

```sh
jira adf convert --input notes.md --output=json
jira adf convert --input notes.md --output=compact | jira issue comment add PROJ-123 --json-input -
```

Review an existing body as Markdown (lossy, read-only):

```sh
jira adf render --input body.json --output=json
```

Both subcommands are local-only — no profile, no network — and honor the same
`--adf-strict` / `--adf-best-effort` modes as mutations.

## Validation modes

`--adf-strict` treats lossy conversion as an error. `--adf-best-effort`
preserves unknown nodes or marks where possible and reports warnings.

```sh
jira --adf-strict issue create --no-input --json-input payload.json
```

The default depends on the operation: submitting a mutation — and a `--dry-run`
preview of one — defaults to **strict**, so a lossy conversion fails before it
reaches Jira; reads and renders default to **best-effort**. Override the default
with the `JIRA_ADF_STRICT` environment variable or the profile's `adf_strict`
setting; an explicit `--adf-strict` / `--adf-best-effort` flag wins over both.

## Supported nodes and marks

The CLI knows every node and mark of the pinned ADF schema: all of them
validate, submit, and round-trip as native ADF. A curated core additionally
authors from Markdown and renders in `issue view`; the rest are
preserve-only. `jira agent adf-matrix` lists the per-row capabilities.

Core nodes: `doc`, `paragraph`, `text`, `heading` (level 1-6), `bulletList`,
`orderedList`, `listItem`, `codeBlock`, `blockquote`, `hardBreak`, `rule`,
`mention`, `emoji`, `date`, `status`, `inlineCard`, `panel`, `table`,
`tableRow`, `tableCell`, `tableHeader`, `taskList`, `taskItem`,
`blockTaskItem`, `decisionList`, `decisionItem`.

Marks: `strong`, `em`, `strike`, `code`, `link`, `textColor`,
`backgroundColor`, `subsup`, `underline`. The `code` mark combines only with
`link`; any other mark on code text is invalid.

A few nodes carry required attributes: `heading` needs `level` (1-6),
`panel` needs `panelType` (`info`/`warning`/`error`/`success`/`note`),
`mention` needs `id` and `text`, `date` needs an epoch-millisecond
`timestamp`, and the task/decision family needs a unique `localId` per node
(generated for you when authoring from Markdown). `listItem`, `tableRow`,
`tableCell`, and `tableHeader` are authored as part of their parent list or
table; `taskItem` as part of its `taskList`.

For the full ADF specification, see Atlassian's
[node reference](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/).

## Discovery

```sh
jira agent adf-matrix --output=json
```

`agent adf-matrix` emits the live support set as JSON, one entry per node
and mark, with its capabilities and the Atlassian documentation link.

## Further reading

*   [Atlassian ADF node reference](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/):
    the canonical structure spec for every node and mark.
*   [ADF JSON Schema](https://go.atlassian.com/adf-json-schema):
    the machine-readable schema Atlassian publishes for validation.
*   [`agent adf-matrix`](agent.md): the CLI's live coverage matrix.
*   [`adf convert` reference](reference/jira/adf/convert.md) and
    [`adf render` reference](reference/jira/adf/render.md): full flag tables.
*   [`custom-fields`](custom-fields.md): rich-text custom fields that accept ADF.
