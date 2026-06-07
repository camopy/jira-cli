package contract

import (
	"encoding/json"
	"os/exec"
	"slices"
	"testing"
)

type clibMetadataSchemaEnvelope struct {
	Data struct {
		Commands    []clibMetadataSchemaCommand `json:"commands"`
		GlobalFlags []clibMetadataSchemaFlag    `json:"global_flags"`
	} `json:"data"`
}

type clibMetadataSchemaCommand struct {
	CommandPath string                      `json:"command_path"`
	Flags       []clibMetadataSchemaFlag    `json:"flags"`
	Subcommands []clibMetadataSchemaCommand `json:"subcommands"`
}

type clibMetadataSchemaFlag struct {
	Name        string   `json:"name"`
	Group       string   `json:"group"`
	Placeholder string   `json:"placeholder"`
	Completion  string   `json:"completion"`
	Enum        []string `json:"enum"`
	EnumDefault string   `json:"enum_default"`
	ValueHint   string   `json:"value_hint"`
}

type clibMetadataFlagWant struct {
	group        string
	placeholder  string
	valueHint    string
	completion   string
	enumContains []string
	enumExcludes []string
}

func TestClibMetadataForHighValueCommands(t *testing.T) {
	schema := loadClibMetadataBaselineSchema(t)

	requireClibMetadataFlag(t, schema.Data.GlobalFlags, "--no-input", clibMetadataFlagWant{group: "Runtime"})
	requireClibMetadataFlag(t, schema.Data.GlobalFlags, "--adf-strict", clibMetadataFlagWant{group: "ADF"})
	requireClibMetadataFlag(t, schema.Data.GlobalFlags, "--adf-best-effort", clibMetadataFlagWant{group: "ADF"})

	requireClibCommandFlag(t, schema, "jira issue create", "--dry-run", clibMetadataFlagWant{group: "Safety"})
	requireClibCommandFlag(t, schema, "jira issue create", "--summary", clibMetadataFlagWant{group: "Fields", placeholder: "TEXT"})
	requireClibCommandFlag(t, schema, "jira issue create", "--json-input", clibMetadataFlagWant{group: "Input", placeholder: "FILE", valueHint: "file"})

	requireClibCommandFlag(t, schema, "jira issue attachment add", "--file", clibMetadataFlagWant{group: "Input", placeholder: "PATH", valueHint: "file"})
	requireClibCommandFlag(t, schema, "jira issue attachment add", "--dry-run", clibMetadataFlagWant{group: "Safety"})

	requireClibCommandFlag(t, schema, "jira config theme", "--name", clibMetadataFlagWant{
		group:        "Theme",
		placeholder:  "NAME",
		enumContains: []string{"auto", "dark", "tokyo-night"},
		enumExcludes: []string{"default"},
	})
	requireClibCommandFlag(t, schema, "jira config theme", "--path", clibMetadataFlagWant{group: "Theme", placeholder: "PATH", valueHint: "file"})

	requireClibCommandFlag(t, schema, "jira worklog add", "--json-input", clibMetadataFlagWant{group: "Input", placeholder: "FILE", valueHint: "file"})
}

func loadClibMetadataBaselineSchema(t *testing.T) clibMetadataSchemaEnvelope {
	t.Helper()

	bin := buildJiraBinary(t)
	out, err := exec.Command(bin, "--output=json", "agent", "schema").CombinedOutput()
	if err != nil {
		t.Fatalf("agent schema error = %v\n%s", err, out)
	}

	var schema clibMetadataSchemaEnvelope
	if err := json.Unmarshal(out, &schema); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	return schema
}

func requireClibCommandFlag(t *testing.T, schema clibMetadataSchemaEnvelope, path, name string, want clibMetadataFlagWant) {
	t.Helper()

	cmd, ok := findClibMetadataBaselineCommand(schema.Data.Commands, path)
	if !ok {
		t.Fatalf("schema missing command_path %q", path)
	}
	requireClibMetadataFlag(t, cmd.Flags, name, want)
}

func requireClibMetadataFlag(t *testing.T, flags []clibMetadataSchemaFlag, name string, want clibMetadataFlagWant) {
	t.Helper()

	for _, flag := range flags {
		if flag.Name != name {
			continue
		}
		if want.group != "" && flag.Group != want.group {
			t.Fatalf("%s group = %q, want %q", name, flag.Group, want.group)
		}
		if want.placeholder != "" && flag.Placeholder != want.placeholder {
			t.Fatalf("%s placeholder = %q, want %q", name, flag.Placeholder, want.placeholder)
		}
		if want.valueHint != "" && flag.ValueHint != want.valueHint {
			t.Fatalf("%s value_hint = %q, want %q", name, flag.ValueHint, want.valueHint)
		}
		if want.completion != "" && flag.Completion != want.completion {
			t.Fatalf("%s completion = %q, want %q", name, flag.Completion, want.completion)
		}
		for _, value := range want.enumContains {
			if !slices.Contains(flag.Enum, value) {
				t.Fatalf("%s enum = %v, want value %q", name, flag.Enum, value)
			}
		}
		for _, value := range want.enumExcludes {
			if slices.Contains(flag.Enum, value) {
				t.Fatalf("%s enum = %v, must not contain %q", name, flag.Enum, value)
			}
		}
		return
	}
	t.Fatalf("schema missing flag %s in %+v", name, flags)
}

func findClibMetadataBaselineCommand(commands []clibMetadataSchemaCommand, path string) (clibMetadataSchemaCommand, bool) {
	for _, cmd := range commands {
		if cmd.CommandPath == path {
			return cmd, true
		}
		if found, ok := findClibMetadataBaselineCommand(cmd.Subcommands, path); ok {
			return found, true
		}
	}
	return clibMetadataSchemaCommand{}, false
}
