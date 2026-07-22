package contract

import (
	"slices"
	"testing"
)

type clibMetadataFlagWant struct {
	group        string
	placeholder  string
	valueHint    string
	completion   string
	enumContains []string
	enumExcludes []string
}

func TestClibMetadataForHighValueCommands(t *testing.T) {
	root := loadAgentSchema(t)

	requireClibMetadataFlag(t, root.Flags, "--no-input", clibMetadataFlagWant{group: "Runtime"})
	requireClibMetadataFlag(t, root.Flags, "--adf-strict", clibMetadataFlagWant{group: "ADF"})
	requireClibMetadataFlag(t, root.Flags, "--adf-best-effort", clibMetadataFlagWant{group: "ADF"})

	requireClibCommandFlag(t, root, "jira issue create", "--dry-run", clibMetadataFlagWant{group: "Safety"})
	requireClibCommandFlag(t, root, "jira issue create", "--summary", clibMetadataFlagWant{group: "Fields", placeholder: "TEXT"})
	requireClibCommandFlag(t, root, "jira issue create", "--json-input", clibMetadataFlagWant{group: "Input", placeholder: "FILE", valueHint: "file"})

	requireClibCommandFlag(t, root, "jira issue attachment add", "--file", clibMetadataFlagWant{group: "Input", placeholder: "PATH", valueHint: "file"})
	requireClibCommandFlag(t, root, "jira issue attachment add", "--dry-run", clibMetadataFlagWant{group: "Safety"})

	requireClibCommandFlag(t, root, "jira config theme", "--name", clibMetadataFlagWant{
		group:        "Theme",
		placeholder:  "NAME",
		enumContains: []string{"auto", "dark", "tokyo-night"},
		enumExcludes: []string{"default"},
	})
	requireClibCommandFlag(t, root, "jira config theme", "--path", clibMetadataFlagWant{group: "Theme", placeholder: "PATH", valueHint: "file"})

	requireClibCommandFlag(t, root, "jira worklog add", "--json-input", clibMetadataFlagWant{group: "Input", placeholder: "FILE", valueHint: "file"})
}

func requireClibCommandFlag(t *testing.T, root docentSchema, path, name string, want clibMetadataFlagWant) {
	t.Helper()

	cmd := findSchemaCommand(root, path)
	if cmd == nil {
		t.Fatalf("schema missing path %q", path)
	}
	requireClibMetadataFlag(t, cmd.Flags, name, want)
}

func requireClibMetadataFlag(t *testing.T, flags []docentSchemaFlag, name string, want clibMetadataFlagWant) {
	t.Helper()

	flag := findClibView(flags, name)
	if flag == nil {
		t.Fatalf("schema missing flag %s", name)
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
}
