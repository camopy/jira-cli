package schema

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type commandSchema struct {
	Name                   string          `json:"name"`
	CommandPath            string          `json:"command_path"`
	Use                    string          `json:"use"`
	Short                  string          `json:"short"`
	Flags                  []flagSchema    `json:"flags,omitempty"`
	MutuallyExclusiveFlags [][]string      `json:"mutually_exclusive_flags,omitempty"`
	RequiredTogetherFlags  [][]string      `json:"required_together_flags,omitempty"`
	OneRequiredFlags       [][]string      `json:"one_required_flags,omitempty"`
	InputSchema            string          `json:"input_schema,omitempty"`
	OutputSchema           string          `json:"output_schema,omitempty"`
	Subcommands            []commandSchema `json:"subcommands,omitempty"`
}

type flagSchema struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Usage       string   `json:"usage"`
	Shorthand   string   `json:"shorthand,omitempty"`
	Default     string   `json:"default,omitempty"`
	Group       string   `json:"group,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Completion  string   `json:"completion,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	EnumDefault string   `json:"enum_default,omitempty"`
	EnumTerse   []string `json:"enum_terse,omitempty"`
	Terse       string   `json:"terse,omitempty"`
	ValueHint   string   `json:"value_hint,omitempty"`
	Required    bool     `json:"required,omitempty"`
}

// WriteSchema emits the CLI command schema. The envelope vs compact vs
// human output shape is decided by the resolved --output mode.
func WriteSchema(cmd *cobra.Command) error {
	root := cmd.Root()
	outputs := outputSchemas()
	inputs := inputSchemas()
	data := map[string]any{
		"commands":       []commandSchema{schemaForCommand(root, inputs, outputs)},
		"global_flags":   flagSchemas(root.PersistentFlags()),
		"output_schemas": outputs,
		"input_schemas":  inputs,
	}
	if cmdutil.UseCompactOutput(cmd) {
		return cli.WriteCompact(cmd.OutOrStdout(), data)
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
	if cmdutil.UseHumanJSONOutput(cmd) {
		return cli.WriteHumanJSON(cmd.OutOrStdout(), env, cmdutil.HumanJSONPrintTheme(cmd))
	}
	if cmdutil.UsePlainOutput(cmd) {
		return cli.WritePlain(cmd.OutOrStdout(), data)
	}
	return cli.WriteEnvelope(cmd.OutOrStdout(), env)
}

func schemaForCommand(cmd *cobra.Command, inputs, outputs map[string]any) commandSchema {
	key := schemaKeyForCommand(cmd)
	schema := commandSchema{
		Name:                   cmd.Name(),
		CommandPath:            cmd.CommandPath(),
		Use:                    cmd.Use,
		Short:                  cmd.Short,
		Flags:                  flagSchemas(cmd.LocalFlags()),
		MutuallyExclusiveFlags: flagGroups(cmd.LocalFlags(), cobraMutuallyExclusiveAnnotation),
		RequiredTogetherFlags:  flagGroups(cmd.LocalFlags(), cobraRequiredAsGroupAnnotation),
		OneRequiredFlags:       flagGroups(cmd.LocalFlags(), cobraOneRequiredAnnotation),
	}
	if _, ok := inputs[key]; ok {
		schema.InputSchema = key
	}
	if _, ok := outputs[key]; ok {
		schema.OutputSchema = key
	}
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		schema.Subcommands = append(schema.Subcommands, schemaForCommand(child, inputs, outputs))
	}
	return schema
}

const (
	cobraRequiredAsGroupAnnotation   = "cobra_annotation_required_if_others_set"
	cobraOneRequiredAnnotation       = "cobra_annotation_one_required"
	cobraMutuallyExclusiveAnnotation = "cobra_annotation_mutually_exclusive"
	cobraRequiredFlagAnnotation      = "cobra_annotation_bash_completion_one_required_flag"
	clibExtraAnnotationKey           = "clib.extra"
)

type clibFlagExtraSchema struct {
	Complete    string   `json:"complete"`
	Enum        []string `json:"enum"`
	EnumDefault string   `json:"enumDefault"`
	EnumTerse   []string `json:"enumTerse"`
	Group       string   `json:"group"`
	Hint        string   `json:"hint"`
	Placeholder string   `json:"placeholder"`
	Terse       string   `json:"terse"`
}

func flagSchemas(flags *pflag.FlagSet) []flagSchema {
	if flags == nil {
		return nil
	}
	out := []flagSchema{}
	seen := map[string]struct{}{}
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Name == "help" {
			return
		}
		if _, ok := seen[flag.Name]; ok {
			return
		}
		seen[flag.Name] = struct{}{}
		extra := clibExtraForFlag(flag)
		out = append(out, flagSchema{
			Name:        "--" + flag.Name,
			Type:        flag.Value.Type(),
			Usage:       flag.Usage,
			Shorthand:   flag.Shorthand,
			Default:     flag.DefValue,
			Group:       extra.Group,
			Placeholder: extra.Placeholder,
			Completion:  extra.Complete,
			Enum:        extra.Enum,
			EnumDefault: extra.EnumDefault,
			EnumTerse:   extra.EnumTerse,
			Terse:       extra.Terse,
			ValueHint:   extra.Hint,
			Required:    len(flag.Annotations[cobraRequiredFlagAnnotation]) > 0,
		})
	})
	return out
}

func clibExtraForFlag(flag *pflag.Flag) clibFlagExtraSchema {
	if flag == nil || len(flag.Annotations[clibExtraAnnotationKey]) == 0 {
		return clibFlagExtraSchema{}
	}
	var extra clibFlagExtraSchema
	_ = json.Unmarshal([]byte(flag.Annotations[clibExtraAnnotationKey][0]), &extra)
	return extra
}

func flagGroups(flags *pflag.FlagSet, annotation string) [][]string {
	if flags == nil {
		return nil
	}
	seen := map[string][]string{}
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		for _, raw := range flag.Annotations[annotation] {
			if raw == "" {
				continue
			}
			// Hidden members (deprecated aliases) stay out of the schema:
			// a group is only worth advertising for the flags an agent can
			// discover, and a single survivor is no longer a relationship.
			group := visibleFlagGroup(flags, normalizeFlagGroup(raw))
			if len(group) > 1 {
				seen[strings.Join(group, "\x00")] = group
			}
		}
	})
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([][]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

// visibleFlagGroup drops group members that are hidden on flags (or absent
// from it entirely), so schema consumers only see relationships between
// flags the schema itself lists.
func visibleFlagGroup(flags *pflag.FlagSet, group []string) []string {
	out := make([]string, 0, len(group))
	for _, name := range group {
		f := flags.Lookup(strings.TrimPrefix(name, "--"))
		if f == nil || f.Hidden {
			continue
		}
		out = append(out, name)
	}
	return out
}

func normalizeFlagGroup(raw string) []string {
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		out = append(out, "--"+strings.TrimPrefix(part, "--"))
	}
	return out
}

func schemaKeyForCommand(cmd *cobra.Command) string {
	path := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), "jira"))
	if path == "" {
		return "root"
	}
	return strings.ReplaceAll(path, " ", ".")
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
						"type":        "object",
						"description": "Canonical pagination block, identical on every paginated command: meta.pagination on single-target reads, and the same shape at results[].data.pagination inside keyed multi-key results. isLast and nextCursor are the authoritative walk signals. Mutation envelopes omit the block entirely.",
						"required":    []string{"startAt", "maxResults", "isLast"}, // pagination-exempt: documents output-shape only
						"properties": map[string]any{
							"startAt":    map[string]any{"type": "integer"}, // pagination-exempt: output-shape only
							"maxResults": map[string]any{"type": "integer"},
							"total":      map[string]any{"type": "integer", "description": "Present only when the endpoint reports an authoritative total; token-paged endpoints (enhanced search) never do."},
							"isLast":     map[string]any{"type": "boolean"},
							"nextCursor": map[string]any{"type": "string", "description": "Pass back via --cursor (search jql, issue list) to fetch the next page."},
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
		"issue.view": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"issue": map[string]any{
					"type":        "object",
					"description": "Present for single-key issue view success.",
				},
				"results": map[string]any{
					"type":        "array",
					"description": "Present for multi-key issue view; ordered like the requested keys.",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"key", "ok"},
						"properties": map[string]any{
							"key":   map[string]any{"type": "string"},
							"ok":    map[string]any{"type": "boolean"},
							"issue": map[string]any{"type": "object", "description": "Present when ok is true."},
							"error": errorSchema,
						},
					},
				},
				"succeeded": map[string]any{"type": "integer"},
				"failed":    map[string]any{"type": "integer"},
			},
		},
		"issue.list": map[string]any{
			"type":     "object",
			"required": []string{"issues"},
			"properties": map[string]any{
				"issues": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"key", "summary", "status", "status_category", "updated"},
						"properties": map[string]any{
							"key":     map[string]any{"type": "string"},
							"summary": map[string]any{"type": "string"},
							"status":  map[string]any{"type": "string"},
							"status_category": map[string]any{
								"type":        "string",
								"description": "Workflow category key (new, indeterminate, done); empty when the status carries none.",
							},
							"status_color": map[string]any{
								"type":        "string",
								"description": "Status category color name; empty when the status carries none.",
							},
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
				"board_scope": map[string]any{
					"type":        "object",
					"description": "Present for board-scoped lists (--board / --board-id). Reports the resolved board and whether its scope was applied.",
					"properties": map[string]any{
						"applied":      map[string]any{"type": "boolean"},
						"project_keys": map[string]any{"type": "array"},
						"id":           map[string]any{"type": "integer"},
						"name":         map[string]any{"type": "string"},
						"type":         map[string]any{"type": "string"},
					},
				},
				"jql": map[string]any{
					"type":        "string",
					"description": "Present for board-scoped lists; the JQL the query resolved to.",
				},
				"precedence": map[string]any{
					"type":        "string",
					"description": "Present for board-scoped lists; which scope source won when several were set.",
				},
				"succeeded_key_chunks": map[string]any{
					"type":        "integer",
					"description": "Present when chunked --key reads partially fail.",
				},
				"failed_key_chunks": map[string]any{
					"type":        "array",
					"description": "Present when chunked --key reads partially fail.",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"key_expr", "error"},
						"properties": map[string]any{
							"key_expr": map[string]any{"type": "string"},
							"error":    errorSchema,
						},
					},
				},
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
			"description": "issue create --json-input payload. Accepts the flat convenience keys shown here or the Jira-native {\"fields\": {...}} object interchangeably (wire spellings like project/issuetype work in either). Object-valued system fields also accept a bare string, lifted to one fixed identity key: project/parent -> {\"key\": ...}, issuetype/priority -> {\"name\": ...}, assignee/reporter -> {\"accountId\": ...}, and string elements of components/fixVersions/versions -> {\"name\": ...}. Explicit wire objects pass through untouched; there is no digits-means-id guessing. The description may be supplied as raw ADF or as Markdown.",
			"properties": map[string]any{
				"summary":              map[string]any{"type": "string"},
				"project_key":          map[string]any{"type": "string"},
				"issue_type":           map[string]any{"type": "string"},
				"fields":               map[string]any{"type": "object", "description": "Jira-native field set; treated as the field set when present."},
				"description":          map[string]any{"$ref": "#/data/input_schemas/adf_document"},
				"description_markdown": map[string]any{"type": "string", "description": "Markdown converted to ADF; mutually exclusive with description."},
			},
		},
		"issue.edit": map[string]any{
			"type":        "object",
			"description": "issue edit --json-input payload. {\"fields\": {...}} is canonical; bare field keys at the top level are accepted as the field set. Object-valued system fields accept a bare string exactly as on create, lifted to one fixed identity key: project/parent -> {\"key\": ...}, issuetype/priority -> {\"name\": ...}, assignee/reporter -> {\"accountId\": ...}, and string elements of components/fixVersions/versions -> {\"name\": ...}; explicit wire objects pass through untouched. ADF-shaped values inside fields (e.g. fields.description) are validated as canonical ADF documents. A top-level update block — the native PUT /rest/api/3/issue body's sibling section of add/set/remove operations — is forwarded verbatim; Jira validates the operation verbs.",
			"properties": map[string]any{
				"fields": map[string]any{"type": "object"},
				"update": map[string]any{"type": "object", "description": "Native REST add/set/remove operation block, sent as a sibling of fields."},
			},
		},
		"issue.transition": map[string]any{
			"type":        "object",
			"description": "issue transition --json-input payload. Accepts the exact POST /rest/api/3/issue/{key}/transitions body: a transition section naming the target ({\"id\": ...} or {\"name\": ...}), a fields object, and an update operation block — plus the convenience comment key (an ADF document posted atomically with the status change). The target may come from the payload or the command line; setting both to different values is an error.",
			"properties": map[string]any{
				"transition": map[string]any{"type": "object", "description": "Target transition by id or name; equivalent to the positional STATUS."},
				"fields":     map[string]any{"type": "object"},
				"update":     map[string]any{"type": "object", "description": "Native REST operation block, forwarded verbatim; mutually exclusive with the comment key when it carries comment operations."},
				"comment":    map[string]any{"$ref": "#/data/input_schemas/adf_document"},
			},
		},
		"issue.link": map[string]any{
			"type":        "object",
			"description": "issue link --json-input payload: the exact POST /rest/api/3/issueLink body. inwardIssue may come from the payload or the positional KEY; setting both to different values is an error. The optional comment block's ADF body is validated before submission.",
			"required":    []string{"type", "outwardIssue"},
			"properties": map[string]any{
				"type":         map[string]any{"type": "object", "description": "Link type by name or id: {\"name\": \"Blocks\"} or {\"id\": \"10003\"}."},
				"inwardIssue":  map[string]any{"type": "object", "description": "{\"key\": ...}; optional when the KEY argument supplies it."},
				"outwardIssue": map[string]any{"type": "object", "description": "{\"key\": ...}."},
				"comment":      map[string]any{"type": "object", "description": "Native comment block; its body is a canonical ADF document."},
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
