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

```sh
jira agent fieldtypes --output=json
```

The registry describes supported customfield encoders and the JSON shape each
expects.

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
