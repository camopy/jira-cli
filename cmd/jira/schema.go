package main

import (
	"time"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type commandSchema struct {
	Name        string          `json:"name"`
	Use         string          `json:"use"`
	Short       string          `json:"short"`
	Flags       []flagSchema    `json:"flags,omitempty"`
	Subcommands []commandSchema `json:"subcommands,omitempty"`
}

type flagSchema struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Usage     string `json:"usage"`
	Shorthand string `json:"shorthand,omitempty"`
	Default   string `json:"default,omitempty"`
}

// writeSchema emits the CLI command schema. The envelope vs compact vs
// human output shape is decided by the resolved --output mode.
func writeSchema(cmd *cobra.Command) error {
	root := cmd.Root()
	data := map[string]any{
		"commands":       []commandSchema{schemaForCommand(root)},
		"output_schemas": outputSchemas(),
		"input_schemas":  inputSchemas(),
	}
	if useCompactOutput(cmd) {
		return cli.WriteCompact(cmd.OutOrStdout(), data)
	}
	if usePlainOutput(cmd) {
		return cli.WritePlain(cmd.OutOrStdout(), data)
	}
	env := cli.Envelope{
		OK: true,
		Meta: cli.Meta{
			Command:   "schema",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: cli.NewRequestID(),
		},
		Data:     data,
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	return cli.WriteEnvelope(cmd.OutOrStdout(), env)
}

func schemaForCommand(cmd *cobra.Command) commandSchema {
	schema := commandSchema{
		Name:  cmd.Name(),
		Use:   cmd.Use,
		Short: cmd.Short,
	}
	seen := map[string]struct{}{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if _, ok := seen[flag.Name]; ok {
			return
		}
		seen[flag.Name] = struct{}{}
		schema.Flags = append(schema.Flags, flagSchema{
			Name:      "--" + flag.Name,
			Type:      flag.Value.Type(),
			Usage:     flag.Usage,
			Shorthand: flag.Shorthand,
			Default:   flag.DefValue,
		})
	})
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		schema.Subcommands = append(schema.Subcommands, schemaForCommand(child))
	}
	return schema
}

func outputSchemas() map[string]any {
	errorSchema := map[string]any{
		"type":     "object",
		"required": []string{"type", "code", "message", "hint", "retryable"},
		"properties": map[string]any{
			"type":                map[string]any{"type": "string"},
			"code":                map[string]any{"type": "string"},
			"message":             map[string]any{"type": "string"},
			"hint":                map[string]any{"type": "string"},
			"retryable":           map[string]any{"type": "boolean"},
			"flag":                map[string]any{"type": "string"},
			"field":               map[string]any{"type": "string"},
			"path":                map[string]any{"type": "string"},
			"http_status":         map[string]any{"type": "integer"},
			"retry_after_seconds": map[string]any{"type": "integer"},
			"provider":            map[string]any{"type": "string"},
			"upstream_code":       map[string]any{"type": "string"},
			"upstream_status":     map[string]any{"type": "integer"},
		},
	}
	envelope := map[string]any{
		"type":     "object",
		"required": []string{"ok", "meta", "data", "errors", "warnings"},
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"},
			"meta": map[string]any{
				"type":     "object",
				"required": []string{"command", "timestamp"},
				"properties": map[string]any{
					"command":    map[string]any{"type": "string"},
					"exit_code":  map[string]any{"type": "integer", "description": "Present only on failure envelopes."},
					"timestamp":  map[string]any{"type": "string", "format": "date-time"},
					"request_id": map[string]any{"type": "string"},
					"pagination": map[string]any{
						"type":     "object",
						"required": []string{"startAt", "maxResults", "isLast"}, // pagination-exempt: documents output-shape only
						"properties": map[string]any{
							"startAt":    map[string]any{"type": "integer"}, // pagination-exempt: output-shape only
							"maxResults": map[string]any{"type": "integer"},
							"total":      map[string]any{"type": "integer"},
							"isLast":     map[string]any{"type": "boolean"},
							"nextCursor": map[string]any{"type": "string"},
						},
					},
				},
			},
			"data": map[string]any{"type": []string{"object", "array", "null"}},
			"errors": map[string]any{
				"type":  "array",
				"items": errorSchema,
			},
			"warnings": map[string]any{"type": "array"},
		},
	}
	return map[string]any{
		"envelope": envelope,
		"error":    errorSchema,
		"issue.list": map[string]any{
			"type":     "object",
			"required": []string{"issues"},
			"properties": map[string]any{
				"issues": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"key", "summary", "status", "updated"},
						"properties": map[string]any{
							"key":     map[string]any{"type": "string"},
							"summary": map[string]any{"type": "string"},
							"status":  map[string]any{"type": "string"},
							"assignee": map[string]any{
								"type": []string{"object", "null"},
								"properties": map[string]any{
									"account_id":   map[string]any{"type": "string"},
									"display_name": map[string]any{"type": "string"},
								},
							},
							"priority": map[string]any{"type": []string{"string", "null"}},
							"updated":  map[string]any{"type": "string", "format": "date-time"},
						},
					},
				},
				"detail": map[string]any{"type": "boolean"},
			},
		},
		"issue.create": map[string]any{
			"type":     "object",
			"required": []string{"dry_run"},
			"properties": map[string]any{
				"issue":   map[string]any{"type": "object", "description": "Present on non-dry-run success."},
				"preview": map[string]any{"type": "object", "description": "Present on --dry-run; carries the would-be payload after Markdown→ADF."},
				"dry_run": map[string]any{"type": "boolean"},
			},
		},
		"issue.edit": map[string]any{
			"type":     "object",
			"required": []string{"issue", "dry_run", "fields"},
			"properties": map[string]any{
				"issue":   map[string]any{"type": "string"},
				"dry_run": map[string]any{"type": "boolean"},
				"fields":  map[string]any{"type": "object"},
				"result":  map[string]any{"type": "object"},
			},
		},
		"worklog.add": map[string]any{
			"type":     "object",
			"required": []string{"issue", "worklog", "dry_run"},
		},
	}
}

// inputSchemas publishes the canonical input shapes the mutation
// commands accept via --json-input. The `adf_document` shape is the one
// canonical ADF document form: every command that takes a rich-text
// body — `issue create` (description), `issue edit` (fields.description),
// `issue comment` (body), and `worklog add` (comment) — accepts exactly
// this shape, and the dry-run preview and live submit both validate it.
//
// $ref pointers resolve against the envelope root: `adf_document` is
// emitted at data.input_schemas.adf_document, so the JSON pointer is
// "#/data/input_schemas/adf_document". A standard JSON-pointer resolver
// can dereference it directly against the command's envelope output.
func inputSchemas() map[string]any {
	adfDocument := map[string]any{
		"type":        "object",
		"description": "Canonical Atlassian Document Format (ADF) document. Accepted by every rich-text body field across the CLI.",
		"required":    []string{"type", "version", "content"},
		"properties": map[string]any{
			"type":    map[string]any{"type": "string", "enum": []string{"doc"}},
			"version": map[string]any{"type": "integer", "enum": []int{1}},
			"content": map[string]any{"type": "array", "description": "Block-level ADF nodes. May be empty."},
		},
		"additionalProperties": false,
	}
	return map[string]any{
		"adf_document": adfDocument,
		"issue.create": map[string]any{
			"type":        "object",
			"description": "issue create --json-input payload. The description may be supplied as raw ADF or as Markdown.",
			"properties": map[string]any{
				"summary":              map[string]any{"type": "string"},
				"project_key":          map[string]any{"type": "string"},
				"issue_type":           map[string]any{"type": "string"},
				"description":          map[string]any{"$ref": "#/data/input_schemas/adf_document"},
				"description_markdown": map[string]any{"type": "string", "description": "Markdown converted to ADF; mutually exclusive with description."},
			},
		},
		"issue.edit": map[string]any{
			"type":        "object",
			"description": "issue edit --json-input payload. ADF-shaped values inside fields (e.g. fields.description) are validated as canonical ADF documents.",
			"properties": map[string]any{
				"fields": map[string]any{"type": "object"},
			},
		},
		"issue.comment": map[string]any{
			"type":        "object",
			"description": "issue comment --json-input body. The top-level object is a canonical ADF document (or {\"body\": <adf_document>}).",
			"$ref":        "#/data/input_schemas/adf_document",
		},
		"worklog.add": map[string]any{
			"type":        "object",
			"description": "worklog add --json-input payload.",
			"properties": map[string]any{
				"time_spent":       map[string]any{"type": "string"},
				"started":          map[string]any{"type": "string"},
				"comment":          map[string]any{"$ref": "#/data/input_schemas/adf_document"},
				"comment_markdown": map[string]any{"type": "string", "description": "Markdown converted to ADF; mutually exclusive with comment."},
			},
		},
	}
}
