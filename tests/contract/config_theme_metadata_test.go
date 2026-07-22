package contract

import (
	"slices"
	"testing"
)

func TestConfigThemePublishesClibMetadata(t *testing.T) {
	root := loadAgentSchema(t)
	cmd := findSchemaCommand(root, "jira config theme")
	if cmd == nil {
		t.Fatalf("schema missing jira config theme command")
	}

	name := findClibView(cmd.Flags, "--name")
	if name == nil {
		t.Fatalf("schema missing --name flag: %+v", cmd.Flags)
	}
	if name.Group != "Theme" || name.Placeholder != "NAME" || name.EnumDefault != "auto" {
		t.Fatalf("--name metadata = %+v, want Theme group, NAME placeholder, auto enum", *name)
	}
	for _, want := range []string{"auto", "dark", "tokyo-night"} {
		if !slices.Contains(name.Enum, want) {
			t.Fatalf("--name enum = %v, want %q", name.Enum, want)
		}
	}
	if slices.Contains(name.Enum, "default") {
		t.Fatalf("--name enum advertises legacy theme name \"default\": %v", name.Enum)
	}

	path := findClibView(cmd.Flags, "--path")
	if path == nil {
		t.Fatalf("schema missing --path flag: %+v", cmd.Flags)
	}
	if path.Group != "Theme" || path.Placeholder != "PATH" || path.ValueHint != "file" {
		t.Fatalf("--path metadata = %+v, want Theme group, PATH placeholder, file hint", *path)
	}
}
