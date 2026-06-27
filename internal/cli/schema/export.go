package schema

import "github.com/spf13/cobra"

// OutputSchemaForCommand returns the registered JSON Schema for the `data`
// payload of cmd's response envelope, and whether a command-specific schema is
// registered. Commands without one return the standard envelope only, so the
// caller renders nothing rather than an empty shape.
//
// It exposes the same registry that `jira schema` emits, so the docs generator
// can render an "Output fields" table from it. The table is generated, not
// hand-written, so it cannot drift from the real output.
func OutputSchemaForCommand(cmd *cobra.Command) (map[string]any, bool) {
	key := schemaKeyForCommand(cmd)
	raw, ok := outputSchemas()[key]
	if !ok {
		return nil, false
	}
	m, ok := raw.(map[string]any)
	return m, ok
}
