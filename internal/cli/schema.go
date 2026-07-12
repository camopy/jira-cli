package cli

import (
	"maps"
	"slices"
)

// CommandSchema is one command's machine-readable description: its dotted
// command name, its flags, and the JSON Schema for its envelope data payload.
type CommandSchema struct {
	Command      string         `json:"command"`
	Flags        []FlagSchema   `json:"flags,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

// FlagSchema names one flag and its type for a CommandSchema.
type FlagSchema struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SchemaRegistry maps a command name to its CommandSchema.
type SchemaRegistry struct {
	commands map[string]CommandSchema
}

// NewSchemaRegistry returns an empty registry ready for Register.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{commands: make(map[string]CommandSchema)}
}

// Register stores schema under its Command name, replacing any prior entry.
func (r *SchemaRegistry) Register(schema CommandSchema) {
	r.commands[schema.Command] = schema
}

// Get returns the schema registered for command and whether one exists.
func (r *SchemaRegistry) Get(command string) (CommandSchema, bool) {
	s, ok := r.commands[command]
	return s, ok
}

// Commands returns the registered command names in sorted order.
func (r *SchemaRegistry) Commands() []string {
	return slices.Sorted(maps.Keys(r.commands))
}
