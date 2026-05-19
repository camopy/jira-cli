package contract

import (
	"encoding/json"
	"os/exec"
	"slices"
	"testing"
)

type configThemeSchemaEnvelope struct {
	Data struct {
		Commands []configThemeSchemaCommand `json:"commands"`
	} `json:"data"`
}

type configThemeSchemaCommand struct {
	CommandPath string                     `json:"command_path"`
	Flags       []configThemeSchemaFlag    `json:"flags"`
	Subcommands []configThemeSchemaCommand `json:"subcommands"`
}

type configThemeSchemaFlag struct {
	Name        string   `json:"name"`
	Group       string   `json:"group"`
	Placeholder string   `json:"placeholder"`
	ValueHint   string   `json:"value_hint"`
	Enum        []string `json:"enum"`
	EnumDefault string   `json:"enum_default"`
}

func TestConfigThemePublishesClibMetadata(t *testing.T) {
	out, err := exec.Command("go", "run", "../../cmd/jira", "--output=json", "agent", "schema").CombinedOutput()
	if err != nil {
		t.Fatalf("agent schema error = %v\n%s", err, out)
	}
	var schema configThemeSchemaEnvelope
	if err := json.Unmarshal(out, &schema); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	cmd := findConfigThemeSchemaCommand(schema.Data.Commands, "jira config theme")
	if cmd == nil {
		t.Fatalf("schema missing jira config theme command:\n%s", out)
	}

	name := findConfigThemeSchemaFlag(cmd.Flags, "--name")
	if name == nil {
		t.Fatalf("schema missing --name flag: %+v", cmd.Flags)
	}
	if name.Group != "Theme" || name.Placeholder != "NAME" || name.EnumDefault != "default" {
		t.Fatalf("--name metadata = %+v, want Theme group, NAME placeholder, default enum", *name)
	}
	for _, want := range []string{"default", "dracula", "tokyo-night"} {
		if !slices.Contains(name.Enum, want) {
			t.Fatalf("--name enum = %v, want %q", name.Enum, want)
		}
	}

	path := findConfigThemeSchemaFlag(cmd.Flags, "--path")
	if path == nil {
		t.Fatalf("schema missing --path flag: %+v", cmd.Flags)
	}
	if path.Group != "Theme" || path.Placeholder != "PATH" || path.ValueHint != "file" {
		t.Fatalf("--path metadata = %+v, want Theme group, PATH placeholder, file hint", *path)
	}
}

func findConfigThemeSchemaCommand(commands []configThemeSchemaCommand, path string) *configThemeSchemaCommand {
	for i := range commands {
		cmd := &commands[i]
		if cmd.CommandPath == path {
			return cmd
		}
		if found := findConfigThemeSchemaCommand(cmd.Subcommands, path); found != nil {
			return found
		}
	}
	return nil
}

func findConfigThemeSchemaFlag(flags []configThemeSchemaFlag, name string) *configThemeSchemaFlag {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}
