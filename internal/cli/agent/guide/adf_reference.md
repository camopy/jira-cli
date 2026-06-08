## adf_reference
When to use this: every long-form body the CLI sends to Jira is an ADF (Atlassian Document Format) doc — `description` and `environment` on → `create_issue` / → `edit_issue`, the body on → `add_comment`, the comment on → `log_work`, and any rich `customfield_*`. Use this section to look up node shapes, mark composition, the gotchas that bite (mention id, date timestamp, code block content), and the strict-vs-best-effort modes that decide whether a lossy step fails or warns.

ADF is canonical. The official spec is at
[developer.atlassian.com/cloud/jira/platform/apis/document](https://developer.atlassian.com/cloud/jira/platform/apis/document/).
The CLI's MVP support set is mirrored in
`jira agent adf-matrix --output=json` (per-row `official_url` points to the
Atlassian docs page for that node/mark).

Every ADF doc starts with the root:

```json
{ "type": "doc", "version": 1, "content": [ /* block nodes */ ] }
```

### Markdown vs native ADF — which to send

Body flags come in two forms: native ADF via `--json-input` (the `description`
/ comment / `*_markdown`-free payload), and the Markdown convenience
(`--body-markdown`, or a `*_markdown` key in a JSON payload). They are not
equivalent.

**Default to native ADF.** It is the canonical wire format and round-trips
without loss. Build the ADF doc directly whenever the content is more than
plain prose — the Markdown path converts client-side through a limited grammar
and silently drops anything outside it.

Markdown converts faithfully for: paragraphs, headings, **bold** / *italic* /
`code` / ~~strike~~ / links, bullet and ordered lists, fenced code blocks,
blockquotes, horizontal rules, and tables. If your message is only these,
`--body-markdown` is fine and saves you hand-writing ADF.

Markdown **cannot express** — these have no Markdown spelling, so the converter
omits them entirely. Author them as native ADF (every one is in the supported
set below):

- `mention` (`@user`, needs an `accountId`), `date` (needs a timestamp),
  `status` lozenges, `emoji`, `inlineCard`
- `panel` (info / note / success / warning / error)
- the `underline`, `subsup`, `textColor`, and `backgroundColor` marks

The data is lost in translation, not flagged inline — reach for any of these and
you must send ADF.

Safety net, not a substitute: on mutation submit ADF-strict is the default, so a
Markdown body whose conversion *degrades a recognised construct* fails with
exit 3 rather than degrading silently (see *ADF strict vs best-effort* below).
But strict mode only catches lossy steps the converter sees — content Markdown
can't represent at all never enters the pipeline to be caught. Prefer ADF up
front for any rich body; use `--dry-run` to confirm the exact doc before
committing.

### The full supported set

These are every node and mark the CLI can author, validate, and submit — the
complete menu when you build ADF by hand. `jira agent adf-matrix --output=json`
is the machine-readable source of truth (per-row capabilities and the official
spec URL); each shape is documented in the sections below. Anything not on this
list is out of the MVP set: on author/submit it is rejected (strict) or carried
through opaquely (best-effort) — see *Opaque preservation* — so do not rely on
nodes such as `expand`, `mediaSingle`, `taskList`, `decisionList`, or `layout`.

- **Structure:** `doc` (root), `paragraph`, `heading` (level 1-6), `text`,
  `hardBreak`, `rule`
- **Lists:** `bulletList`, `orderedList`, `listItem`
- **Blocks:** `blockquote`, `codeBlock`, `panel`
- **Tables:** `table`, `tableRow`, `tableHeader`, `tableCell`
- **Inline nodes:** `mention`, `emoji`, `date`, `status`, `inlineCard`
- **Marks:** `strong`, `em`, `strike`, `code`, `link`, `underline`, `subsup`,
  `textColor`, `backgroundColor`

### Block nodes

```json
// paragraph (the simplest body)
{"type": "paragraph", "content": [{"type": "text", "text": "hello"}]}

// heading (level 1-6)
{"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "Section"}]}

// blockquote (wraps any block content)
{"type": "blockquote", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "quoted"}]}]}

// bulletList / orderedList (content is listItem[])
{"type": "bulletList", "content": [
  {"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "first"}]}]}
]}

// codeBlock (attrs.language is the syntax-highlight hint)
{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "func main() {}"}]}

// rule (horizontal divider, no content)
{"type": "rule"}

// panel (panelType = info | warning | error | success | note)
{"type": "panel", "attrs": {"panelType": "info"}, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "info panel"}]}]}

// table (content is tableRow[]; tableRow content is tableHeader[] / tableCell[])
{"type": "table", "attrs": {"isNumberColumnEnabled": false, "layout": "default"}, "content": [
  {"type": "tableRow", "content": [
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Col"}]}]}
  ]},
  {"type": "tableRow", "content": [
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "val"}]}]}
  ]}
]}

// expand (collapsible — non-MVP but Jira accepts it)
{"type": "expand", "attrs": {"title": "Click to expand"}, "content": [{"type": "paragraph", "content": [{"type": "text", "text": "hidden"}]}]}

// nestedExpand (only valid INSIDE tableCell or tableHeader)
```

### Inline nodes (live inside a paragraph's content[])

```json
// text — the only required attr is .text
{"type": "text", "text": "plain"}

// hardBreak — line break inside a paragraph
{"type": "text", "text": "line one"}, {"type": "hardBreak"}, {"type": "text", "text": "line two"}

// mention — id MUST be the user's accountId
{"type": "mention", "attrs": {"id": "712020:00000000-0000-0000-0000-000000000000", "text": "@Test User"}}

// emoji — shortName + id (unicode codepoint) + text (the actual unicode)
{"type": "emoji", "attrs": {"shortName": ":rocket:", "id": "1f680", "text": "🚀"}}

// date — timestamp is epoch milliseconds AS A STRING
{"type": "date", "attrs": {"timestamp": "1769817600000"}}

// status — text is the label, color is named (green/red/yellow/blue/purple/...)
{"type": "status", "attrs": {"text": "READY", "color": "green"}}

// inlineCard — smart link
{"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/<ISSUE_KEY>"}}
```

### Marks (annotate text nodes)

```json
{"type": "text", "text": "bold",       "marks": [{"type": "strong"}]}
{"type": "text", "text": "italic",     "marks": [{"type": "em"}]}
{"type": "text", "text": "struck",     "marks": [{"type": "strike"}]}
{"type": "text", "text": "underlined", "marks": [{"type": "underline"}]}
{"type": "text", "text": "code()",     "marks": [{"type": "code"}]}
{"type": "text", "text": "link",       "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]}
{"type": "text", "text": "red",        "marks": [{"type": "textColor", "attrs": {"color": "#ff0000"}}]}
{"type": "text", "text": "highlight",  "marks": [{"type": "backgroundColor", "attrs": {"color": "#fffacd"}}]}
{"type": "text", "text": "2",          "marks": [{"type": "subsup", "attrs": {"type": "sub"}}]}
```

Multiple marks on one text node compose:

```json
{"type": "text", "text": "loud", "marks": [{"type": "strong"}, {"type": "underline"}, {"type": "textColor", "attrs": {"color": "#ff0000"}}]}
```

### Composition recipes

Drop these straight into `--json-input` for `issue create`, `issue edit`,
or `issue comment`. As substructures of an `issue create` payload,
assign them to the bare Jira field name (`description`, `environment`,
the relevant `customfield_NNNN`) — that is what Jira's API accepts and
what the CLI's ADF validator detects.

**Heading + paragraph + link:**

```json
{"type": "doc", "version": 1, "content": [
  {"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "Investigation"}]},
  {"type": "paragraph", "content": [
    {"type": "text", "text": "See "},
    {"type": "text", "text": "PR #482", "marks": [{"type": "link", "attrs": {"href": "https://github.com/org/repo/pull/482"}}]},
    {"type": "text", "text": " for the fix."}
  ]}
]}
```

**Bulleted list with mixed marks per item:**

```json
{"type": "bulletList", "content": [
  {"type": "listItem", "content": [{"type": "paragraph", "content": [
    {"type": "text", "text": "DB write path: ", "marks": [{"type": "strong"}]},
    {"type": "text", "text": "blocked"}
  ]}]},
  {"type": "listItem", "content": [{"type": "paragraph", "content": [
    {"type": "text", "text": "Inline code "},
    {"type": "text", "text": "user.last_login", "marks": [{"type": "code"}]},
    {"type": "text", "text": " not updating."}
  ]}]}
]}
```

**Numbered list — same shape, swap `bulletList` for `orderedList`:**

```json
{"type": "orderedList", "attrs": {"order": 1}, "content": [/* listItem[]... */]}
```

**Code block with language:**

```json
{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "func main() {\n  fmt.Println(\"hi\")\n}"}]}
```

**Panel for callouts (info / warning / error / success / note):**

```json
{"type": "panel", "attrs": {"panelType": "warning"}, "content": [
  {"type": "paragraph", "content": [
    {"type": "text", "text": "Don't forget to bump the schema version."}
  ]}
]}
```

**Inline mention of a user:**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "cc "},
  {"type": "mention", "attrs": {"id": "712020:00000000-0000-0000-0000-000000000000", "text": "@Test User"}},
  {"type": "text", "text": " — heads up"}
]}
```

The `id` MUST be the user's `accountId` (get it from
`jira me --output=json` for yourself, or the assignee field on any issue
they own). The `text` is the display label and can be anything.

**Status pill (named color: green / red / yellow / blue / purple / grey / neutral):**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "Build: "},
  {"type": "status", "attrs": {"text": "GREEN", "color": "green"}}
]}
```

**Inline date (epoch milliseconds, as a string):**

```json
{"type": "paragraph", "content": [
  {"type": "text", "text": "Target: "},
  {"type": "date", "attrs": {"timestamp": "1769817600000"}}
]}
```

**Smart link (Jira renders as a card if the URL is recognised):**

```json
{"type": "paragraph", "content": [
  {"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/<ISSUE_KEY>"}}
]}
```

**Quote block (any block content can nest inside):**

```json
{"type": "blockquote", "content": [
  {"type": "paragraph", "content": [
    {"type": "text", "text": "From the postmortem: "},
    {"type": "text", "text": "the migration ran twice", "marks": [{"type": "em"}]},
    {"type": "text", "text": "."}
  ]}
]}
```

**Two-column table with header row:**

```json
{"type": "table", "attrs": {"isNumberColumnEnabled": false, "layout": "default"}, "content": [
  {"type": "tableRow", "content": [
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Field"}]}]},
    {"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Value"}]}]}
  ]},
  {"type": "tableRow", "content": [
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "duration"}]}]},
    {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "2h30m"}]}]}
  ]}
]}
```

**Horizontal divider between sections:**

```json
{"type": "rule"}
```

### ADF gotchas

- **Marks live on `text` nodes only** — putting `marks` on a
  `paragraph` or `heading` is invalid; strict mode rejects with
  the path of the offending node.
- **`bulletList` / `orderedList` content is `listItem[]`** — and
  every `listItem` content is `paragraph` (or another list to
  nest). Putting raw `text` directly inside a `listItem` is invalid.
- **`mention.attrs.id` is the accountId, not the email or display name.**
  Wrong id → mention renders as plain text in Jira.
- **`date.attrs.timestamp` is a string of milliseconds**, not a
  number and not seconds. `1769817600000` not `1769817600`.
- **`codeBlock` content is a single `text` node** containing the
  whole code with embedded `\n`s — not a list of lines.
- **`emoji.attrs.id` is the unicode codepoint** (e.g. `1f680`),
  `shortName` is the `:rocket:`-style alias, `text` is the actual
  unicode glyph (`🚀`). All three should be set for portability.
- **`tableCell` / `tableHeader` content must be wrapped in
  `paragraph`** — bare text inside a cell is invalid.

### ADF strict vs best-effort

| Path                          | Default mode |
|-------------------------------|--------------|
| Read / render                 | best-effort  |
| `--output=human` extract      | best-effort  |
| Mutation submit               | strict       |
| `--dry-run` preview           | strict       |

Override per call:

```sh
jira issue create ... --adf-strict        # any lossy step → exit 3
jira issue create ... --adf-best-effort   # degrade silently with warnings
```

Or globally: `JIRA_ADF_STRICT=true` env, or `adf_strict = true` in the
profile TOML. Precedence: flag > env > profile > per-path default.

### Opaque preservation

Unknown ADF nodes/marks (anything outside the CLI's MVP set) round-trip
through the CLI byte-equivalently — the typed model preserves them via
opaque passthrough. **However**: Jira's create endpoint validates the
posted document against its own ADF schema and will reject truly unknown
node types with `INVALID_INPUT (400)`. The opaque path is for
preserving fidelity on read; submit paths are bounded by what Jira
itself accepts.
