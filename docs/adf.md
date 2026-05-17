# ADF

Atlassian Document Format is the canonical rich-text input for issue
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
jira issue comment add PROJ-1 --json-input adf.json --no-input --output=json
```

## Validation Modes

`--adf-strict` treats lossy conversion as an error. `--adf-best-effort`
preserves unknown nodes or marks where possible and reports warnings.

```sh
jira --adf-strict issue create --no-input --json-input payload.json
```

## Supported nodes and marks

The CLI authors and renders a core subset of ADF. Unknown nodes are still
validated and either rejected (`--adf-strict`) or preserved with a warning
(`--adf-best-effort`).

Nodes: `doc`, `paragraph`, `text`, `heading` (level 1-6), `bulletList`,
`orderedList`, `listItem`, `codeBlock`, `blockquote`, `hardBreak`, `rule`,
`mention`, `emoji`, `date`, `status`, `inlineCard`, `panel`, `table`,
`tableRow`, `tableCell`, `tableHeader`.

Marks: `strong`, `em`, `strike`, `code`, `link`, `textColor`,
`backgroundColor`, `subsup`, `underline`.

A few nodes carry required attributes: `heading` needs `level` (1-6),
`panel` needs `panelType` (`info`/`warning`/`error`/`success`/`note`),
`mention` needs `id` and `text`, `date` needs an epoch-millisecond
`timestamp`. `listItem`, `tableRow`, `tableCell`, and `tableHeader` are
authored as part of their parent list or table.

For the full ADF specification, see Atlassian's
[node reference](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/).

## Discovery

```sh
jira agent adf-matrix --output=json
```

`agent adf-matrix` emits the live support set as JSON — one entry per node
and mark, with its capabilities and the Atlassian documentation link.
