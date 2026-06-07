package contract

import (
	"encoding/json"
	"os/exec"
	"slices"
	"testing"
)

type issueCommentWorklogMetadataEnvelope struct {
	Data struct {
		Commands []issueCommentWorklogMetadataCommand `json:"commands"`
	} `json:"data"`
}

type issueCommentWorklogMetadataCommand struct {
	CommandPath string                               `json:"command_path"`
	Flags       []issueCommentWorklogMetadataFlag    `json:"flags"`
	Subcommands []issueCommentWorklogMetadataCommand `json:"subcommands"`
}

type issueCommentWorklogMetadataFlag struct {
	Name        string   `json:"name"`
	Group       string   `json:"group"`
	Placeholder string   `json:"placeholder"`
	Completion  string   `json:"completion"`
	ValueHint   string   `json:"value_hint"`
	Enum        []string `json:"enum"`
	EnumTerse   []string `json:"enum_terse"`
	Terse       string   `json:"terse"`
}

type issueCommentWorklogMetadataWant struct {
	commandPath  string
	flagName     string
	group        string
	placeholder  string
	completion   string
	valueHint    string
	enumContains []string
	enumTerse    []string
	terse        string
}

func TestIssueCommentAndWorklogFlagsPublishClibMetadata(t *testing.T) {
	schema := loadIssueCommentWorklogMetadataSchema(t)

	for _, want := range []issueCommentWorklogMetadataWant{
		{commandPath: "jira issue mine", flagName: "--detail", group: "Output"},
		{commandPath: "jira issue mine", flagName: "--jql", group: "Filters", placeholder: "JQL"},
		{commandPath: "jira issue mine", flagName: "--as-jql", group: "Output"},
		{commandPath: "jira issue mine", flagName: "--status", group: "Filters", placeholder: "NAME", completion: "predictor=cachestatus,comma", terse: "by status"},
		{commandPath: "jira issue mine", flagName: "--project", group: "Filters", placeholder: "KEY", completion: "predictor=cacheproject,comma"},
		{commandPath: "jira issue mine", flagName: "--key", group: "Filters", placeholder: "KEY", completion: "predictor=issuekey,comma"},
		{commandPath: "jira issue mine", flagName: "--epic", group: "Filters", placeholder: "KEY", completion: "predictor=cacheepic,comma"},
		{commandPath: "jira issue mine", flagName: "--priority", group: "Filters", placeholder: "NAME", completion: "predictor=cachepriority,comma", terse: "by priority"},
		{commandPath: "jira issue mine", flagName: "--label", group: "Filters", placeholder: "NAME", completion: "predictor=cachelabel,comma"},
		{commandPath: "jira issue mine", flagName: "--type", group: "Filters", placeholder: "NAME", completion: "predictor=cacheissuetype,comma", terse: "by type"},
		{commandPath: "jira issue mine", flagName: "--updated", group: "Filters", placeholder: "DATE"},
		{commandPath: "jira issue mine", flagName: "--created", group: "Filters", placeholder: "DATE"},
		{commandPath: "jira issue mine", flagName: "--resolved", group: "Filters", placeholder: "DATE"},
		{commandPath: "jira issue mine", flagName: "--order-by", group: "Sort", placeholder: "FIELD", enumTerse: []string{"last-updated time", "creation time", "priority level", "workflow status", "issue key", "title text"}},
		{commandPath: "jira issue mine", flagName: "--desc", group: "Sort"},

		{commandPath: "jira issue list", flagName: "--count", group: "Output"},
		{commandPath: "jira search jql", flagName: "--count", group: "Output"},
		{commandPath: "jira jql validate", flagName: "--mode", group: "Validation", placeholder: "MODE", enumContains: []string{"strict", "warn", "none"}, enumTerse: []string{"strictest", "lenient", "no validation"}},
		{commandPath: "jira search jql", flagName: "--all", group: "Pagination"},
		{commandPath: "jira search jql", flagName: "--limit", group: "Pagination", placeholder: "N"},
		{commandPath: "jira search jql", flagName: "--unbounded", group: "Pagination"},
		{commandPath: "jira jql build", flagName: "--status", group: "Filters", placeholder: "NAME", completion: "predictor=cachestatus,comma", terse: "by status"},
		{commandPath: "jira jql build", flagName: "--priority", group: "Filters", placeholder: "NAME", completion: "predictor=cachepriority,comma", terse: "by priority"},
		{commandPath: "jira jql build", flagName: "--assignee", group: "Filters", placeholder: "USER", enumContains: []string{"me", "none"}, enumTerse: []string{"current user", "unassigned"}},
		{commandPath: "jira jql build", flagName: "--reporter", group: "Filters", placeholder: "USER", enumContains: []string{"me"}, enumTerse: []string{"current user"}},
		{commandPath: "jira issue list", flagName: "--columns", group: "Output", placeholder: "COLS", enumContains: []string{"key", "summary", "status", "assignee", "priority", "updated"}, enumTerse: []string{"issue key", "title text", "workflow status", "assigned user", "priority level", "last-updated time"}},
		{commandPath: "jira issue mine", flagName: "--columns", group: "Output", placeholder: "COLS", enumContains: []string{"key", "summary", "status", "assignee", "priority", "updated"}, enumTerse: []string{"issue key", "title text", "workflow status", "assigned user", "priority level", "last-updated time"}},

		{commandPath: "jira auth login", flagName: "--backend", placeholder: "BACKEND", enumContains: []string{"keyring", "1password"}, enumTerse: []string{"OS keychain", "1Password CLI"}},
		{commandPath: "jira auth migrate", flagName: "--backend", placeholder: "BACKEND", enumContains: []string{"keyring", "1password"}, enumTerse: []string{"OS keychain", "1Password CLI"}},
		// Self-describing theme names carry a short Terse and deliberately no
		// EnumTerse; the schema-wide guard below proves that omission is safe.
		{commandPath: "jira config theme", flagName: "--name", group: "Theme", placeholder: "NAME", enumContains: []string{"auto", "dark", "nord"}},

		{commandPath: "jira issue create", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue create", flagName: "--summary", group: "Fields", placeholder: "TEXT"},
		{commandPath: "jira issue create", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue create", flagName: "--assignee", group: "Fields", placeholder: "USER", enumContains: []string{"me"}, enumTerse: []string{"current user"}},
		{commandPath: "jira issue create", flagName: "--priority", group: "Fields", placeholder: "NAME", completion: "predictor=cachepriority", terse: "priority"},
		{commandPath: "jira issue create", flagName: "--type", group: "Fields", placeholder: "NAME", completion: "predictor=cacheissuetype", terse: "issue type"},

		{commandPath: "jira issue edit", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue edit", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue edit", flagName: "--summary", group: "Fields", placeholder: "TEXT"},
		{commandPath: "jira issue edit", flagName: "--assignee", group: "Fields", placeholder: "USER", enumContains: []string{"me", "none"}, enumTerse: []string{"current user", "unassign"}},

		{commandPath: "jira issue transition", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue transition", flagName: "--transition", group: "Transition", placeholder: "STATUS"},

		{commandPath: "jira issue clone", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue clone", flagName: "--force", group: "Safety"},
		{commandPath: "jira issue clone", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue move", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue move", flagName: "--force", group: "Safety"},
		{commandPath: "jira issue move", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue delete", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue delete", flagName: "--force", group: "Safety"},
		{commandPath: "jira issue delete", flagName: "--delete-subtasks", group: "Safety"},
		{commandPath: "jira issue delete", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},

		{commandPath: "jira issue weblink", flagName: "--url", group: "Link", placeholder: "URL", valueHint: "url"},
		{commandPath: "jira issue weblink", flagName: "--title", group: "Link", placeholder: "TEXT"},
		{commandPath: "jira issue weblink", flagName: "--dry-run", group: "Safety"},

		{commandPath: "jira issue comment", flagName: "--body-markdown", group: "Input", placeholder: "MARKDOWN"},
		{commandPath: "jira issue comment", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue comment", flagName: "--visibility-role", group: "Visibility", placeholder: "ROLE"},
		{commandPath: "jira issue comment", flagName: "--visibility-group", group: "Visibility", placeholder: "GROUP"},
		{commandPath: "jira issue comment", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue comment list", flagName: "--limit", group: "Pagination", placeholder: "N"},
		{commandPath: "jira issue comment list", flagName: "--all", group: "Pagination"},
		{commandPath: "jira issue comment add", flagName: "--body-markdown", group: "Input", placeholder: "MARKDOWN"},
		{commandPath: "jira issue comment add", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue comment add", flagName: "--visibility-role", group: "Visibility", placeholder: "ROLE"},
		{commandPath: "jira issue comment add", flagName: "--visibility-group", group: "Visibility", placeholder: "GROUP"},
		{commandPath: "jira issue comment add", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue comment edit", flagName: "--body-markdown", group: "Input", placeholder: "MARKDOWN"},
		{commandPath: "jira issue comment edit", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue comment edit", flagName: "--visibility-role", group: "Visibility", placeholder: "ROLE"},
		{commandPath: "jira issue comment edit", flagName: "--visibility-group", group: "Visibility", placeholder: "GROUP"},
		{commandPath: "jira issue comment edit", flagName: "--clear-visibility", group: "Visibility"},
		{commandPath: "jira issue comment edit", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue comment delete", flagName: "--force", group: "Safety"},
		{commandPath: "jira issue comment delete", flagName: "--dry-run", group: "Safety"},

		{commandPath: "jira worklog add", flagName: "--time-spent", group: "Worklog", placeholder: "DURATION"},
		{commandPath: "jira worklog add", flagName: "--started", group: "Worklog", placeholder: "TIME"},
		{commandPath: "jira worklog add", flagName: "--comment-markdown", group: "Input", placeholder: "MARKDOWN"},
		{commandPath: "jira worklog add", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira worklog add", flagName: "--dry-run", group: "Safety"},
	} {
		requireClibMetadata(t, schema, want)
	}
}

func loadIssueCommentWorklogMetadataSchema(t *testing.T) issueCommentWorklogMetadataEnvelope {
	t.Helper()
	bin := buildJiraBinary(t)
	out, err := exec.Command(bin, "--output=json", "agent", "schema").CombinedOutput()
	if err != nil {
		t.Fatalf("agent schema error = %v\n%s", err, out)
	}
	var schema issueCommentWorklogMetadataEnvelope
	if err := json.Unmarshal(out, &schema); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	return schema
}

func requireClibMetadata(t *testing.T, schema issueCommentWorklogMetadataEnvelope, want issueCommentWorklogMetadataWant) {
	t.Helper()
	cmd, ok := findIssueCommentWorklogMetadataCommand(schema.Data.Commands, want.commandPath)
	if !ok {
		t.Fatalf("schema missing command_path %q", want.commandPath)
	}
	for _, flag := range cmd.Flags {
		if flag.Name != want.flagName {
			continue
		}
		if flag.Group != want.group {
			t.Fatalf("%s %s group = %q, want %q", want.commandPath, want.flagName, flag.Group, want.group)
		}
		if flag.Placeholder != want.placeholder {
			t.Fatalf("%s %s placeholder = %q, want %q", want.commandPath, want.flagName, flag.Placeholder, want.placeholder)
		}
		if flag.Completion != want.completion {
			t.Fatalf("%s %s completion = %q, want %q", want.commandPath, want.flagName, flag.Completion, want.completion)
		}
		if flag.ValueHint != want.valueHint {
			t.Fatalf("%s %s value_hint = %q, want %q", want.commandPath, want.flagName, flag.ValueHint, want.valueHint)
		}
		for _, value := range want.enumContains {
			if !slices.Contains(flag.Enum, value) {
				t.Fatalf("%s %s enum = %v, want value %q", want.commandPath, want.flagName, flag.Enum, value)
			}
		}
		if want.terse != "" && flag.Terse != want.terse {
			t.Fatalf("%s %s terse = %q, want %q", want.commandPath, want.flagName, flag.Terse, want.terse)
		}
		if want.enumTerse != nil {
			if !slices.Equal(flag.EnumTerse, want.enumTerse) {
				t.Fatalf("%s %s enum_terse = %v, want %v", want.commandPath, want.flagName, flag.EnumTerse, want.enumTerse)
			}
			if len(flag.EnumTerse) != len(flag.Enum) {
				t.Fatalf("%s %s enum_terse len %d != enum len %d (a value would fall back to the flag usage)", want.commandPath, want.flagName, len(flag.EnumTerse), len(flag.Enum))
			}
		}
		return
	}
	t.Fatalf("schema missing flag %s on %s: %+v", want.flagName, want.commandPath, cmd.Flags)
}

// TestEnumTerseMatchesEnumLength walks every command in the schema and asserts
// that any flag carrying enum descriptions carries exactly one per enum value.
// clib pairs Enum and EnumTerse positionally and silently drops all
// descriptions when the lengths differ, so a mismatched pair would quietly
// reintroduce the flag-usage noise on every value. This guard fires for any
// enum flag — including ones nobody remembered to add a per-flag want row for.
func TestEnumTerseMatchesEnumLength(t *testing.T) {
	schema := loadIssueCommentWorklogMetadataSchema(t)

	var walk func(cmds []issueCommentWorklogMetadataCommand)
	walk = func(cmds []issueCommentWorklogMetadataCommand) {
		for _, cmd := range cmds {
			for _, flag := range cmd.Flags {
				if len(flag.EnumTerse) > 0 && len(flag.EnumTerse) != len(flag.Enum) {
					t.Errorf("%s %s: enum_terse len %d != enum len %d — every value would fall back to the flag usage", cmd.CommandPath, flag.Name, len(flag.EnumTerse), len(flag.Enum))
				}
			}
			walk(cmd.Subcommands)
		}
	}
	walk(schema.Data.Commands)
}

func findIssueCommentWorklogMetadataCommand(commands []issueCommentWorklogMetadataCommand, path string) (issueCommentWorklogMetadataCommand, bool) {
	for _, cmd := range commands {
		if cmd.CommandPath == path {
			return cmd, true
		}
		if found, ok := findIssueCommentWorklogMetadataCommand(cmd.Subcommands, path); ok {
			return found, true
		}
	}
	return issueCommentWorklogMetadataCommand{}, false
}
