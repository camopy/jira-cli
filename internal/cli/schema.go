package cli

import "sort"

type CommandSchema struct {
	Command      string         `json:"command"`
	Flags        []FlagSchema   `json:"flags,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

type FlagSchema struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type SchemaRegistry struct {
	commands map[string]CommandSchema
}

func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{commands: make(map[string]CommandSchema)}
}

func (r *SchemaRegistry) Register(schema CommandSchema) {
	r.commands[schema.Command] = schema
}

func (r *SchemaRegistry) Get(command string) (CommandSchema, bool) {
	s, ok := r.commands[command]
	return s, ok
}

func (r *SchemaRegistry) Commands() []string {
	out := make([]string, 0, len(r.commands))
	for command := range r.commands {
		out = append(out, command)
	}
	sort.Strings(out)
	return out
}
