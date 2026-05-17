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

## Discovery

```sh
jira agent adf-matrix --output=json
```

The matrix reports which ADF nodes and marks can be authored, rendered,
preserved, validated, and submitted.
