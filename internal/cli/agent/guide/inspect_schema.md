## inspect_schema
Goal: Introspect the CLI's command tree, JSON output schemas, ADF support set, and customfield encoders without reading prose, so payload authoring is grounded in the binary's own contract.
When: payload shape, available subcommands, ADF node support, or customfield encoders need to be confirmed from the binary itself rather than docs.

**Decide**
- Need the full command tree + flag signatures + per-command JSON output schemas: `jira agent schema`.
- Need this guide as text (or a specific section by slug): `jira agent guide [<section>]`.
- Need to know which ADF nodes/marks the CLI authors, renders, preserves, validates, or submits: `jira agent adf-matrix --output=json`.
- Need to know which `customfield_NNNN` keys the encoder registry knows and what input shape they expect: `jira agent fieldtypes --output=json`.

**Run**
- Command tree (compact, jq-friendly): `jira agent schema --output=compact`
- This guide (full): `jira agent guide`
- This guide (one section): `jira agent guide <slug>` — e.g. `jira agent guide jql_reference`
- ADF support matrix: `jira agent adf-matrix --output=json`
- Customfield encoder registry: `jira agent fieldtypes --output=json`

**Save**
> Requires `--output=json` (or `--output=compact`).
- `data[].kind` [string, required] — `node`, `mark`, or `field-type`.
- `data[].name` [string, required] — e.g. `paragraph`, `strong`, `customfield_10010`.
- `data[].status` [string, required] — `mvp` or `preserve-only`.
- `data[].capabilities` [object, required] — booleans for `author`, `render`, `preserve`, `validate`, `submit`.
- `data[].input_shape` [object, required] — JSON Schema 2020-12 fragment for what the CLI accepts.
- `data[].output_shape` [object, required] — JSON Schema 2020-12 fragment for what the CLI returns.
- `data[].warnings` [array, required] — known degradation cases for this entry.
- `data[].official_url` [string, optional] — Atlassian docs page for the node/mark/field.
- `data[].notes` [string, optional] — free-form caveats.
- `data[].submit_description` [string, optional] — how this entry behaves on a mutation submit (e.g. "ADF: included in a Jira rich-text field payload after ADF validation passes.").

**Behavior**
- `adf-matrix --output=json` and `fieldtypes --output=json` emit arrays of the **same envelope shape** — a single agent parser handles both surfaces:
  ```json
  {
    "kind": "node|mark|field-type",
    "name": "paragraph",
    "status": "mvp|preserve-only",
    "capabilities": { "author": true, "render": true, "preserve": true, "validate": true, "submit": true },
    "input_shape": { /* JSON Schema 2020-12 fragment */ },
    "output_shape": { /* JSON Schema 2020-12 fragment */ },
    "warnings": [],
    "official_url": "https://developer.atlassian.com/...",
    "notes": "...",
    "submit_description": "ADF: included in a Jira rich-text field payload after ADF validation passes."
  }
  ```
- `agent guide <section>` accepts the slugs used throughout this guide — handy when you want a focused subset rather than the whole text.
- Use `agent fieldtypes` to learn what shape `customfield_NNNN` expects before authoring it; pair with the cache primer (`jira cache fields`) to map names ↔ ids on the live instance.

**Next**
- Then: → `auth_setup` (if `agent schema` reveals a command you need but the profile is unconfigured)
- Then: → `cache_metadata` (prime `fields` / `projects` / `issuetypes` for live id ↔ name mapping)
- Composes: → any write workflow (`create_issue`, `edit_issue`, `add_comment`) consumes the schemas to validate payloads before submit.
