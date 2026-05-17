# Custom Fields

Jira custom fields are addressed by their API IDs, such as
`customfield_10010`. Use the field cache and agent registry to map IDs to
names and input shapes.

## Field Cache

```sh
jira cache fields --refresh --output=json
jira cache fields --output=json
```

The field cache helps completion and lets agents map human field names to
`customfield_NNNN` keys before editing payloads.

## Field-Type Registry

The CLI recognizes these custom-field types and encodes a bare value into the
wire shape Jira expects. An explicit object (`{"id":...}` or `{"value":...}`)
is always passed through unchanged.

| Type | Bare input | Wire shape |
|------|-----------|------------|
| `string` | `"text"` | raw value |
| `number` | `42` / `1.5` | raw numeric value |
| `date` | `"2026-05-17"` | `YYYY-MM-DD` string |
| `datetime` | ISO-8601 with offset | datetime string |
| `labels` | `["a","b"]` | array of strings |
| `select` | `"High"` | `{"value":"High"}` |
| `multiselect` | `["A","B"]` | `[{"value":"A"},{"value":"B"}]` |
| `user` | `"<accountId>"` | `{"accountId":"<id>"}` |
| `multiuser` | `["<id1>","<id2>"]` | array of `{"accountId":...}` |
| `group` / `multigroup` | group name(s) | `{"name":...}` / array |
| `components` | component name(s) | array of `{"name":...}` |
| `version` / `fixversions` | version name(s) | version array |
| `versionpicker` / `multiversionpicker` | version name(s) | `{"name":...}` / array |
| `projectpicker` | project key | `{"key":"<project>"}` |
| `parent` | `"PROJ-123"` | parent issue link |
| `cascadingselect` | — | `{"value":"<top>","child":{"value":"<sub>"}}` |

```sh
jira agent fieldtypes --output=json
```

`agent fieldtypes` emits the live registry as JSON, including the encoding
notes above, so an agent can resolve a field shape without parsing prose.

## JSON Input

```json
{
  "fields": {
    "customfield_10010": "Platform",
    "customfield_10011": ["frontend", "backend"]
  }
}
```

```sh
jira issue edit PROJ-1 --json-input fields.json --output=json
```

Unknown customfield types are forwarded and reported with structured warnings
where the CLI cannot validate the shape locally.
