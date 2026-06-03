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
}

type issueCommentWorklogMetadataWant struct {
	commandPath  string
	flagName     string
	group        string
	placeholder  string
	completion   string
	valueHint    string
	enumContains []string
}

func TestIssueCommentAndWorklogFlagsPublishClibMetadata(t *testing.T) {
	schema := loadIssueCommentWorklogMetadataSchema(t)

	for _, want := range []issueCommentWorklogMetadataWant{
		{commandPath: "jira issue mine", flagName: "--detail", group: "Output"},
		{commandPath: "jira issue mine", flagName: "--jql", group: "Filters", placeholder: "JQL"},
		{commandPath: "jira issue mine", flagName: "--as-jql", group: "Output"},
		{commandPath: "jira issue mine", flagName: "--status", group: "Filters", placeholder: "NAME", completion: "predictor=cachestatus,comma"},
		{commandPath: "jira issue mine", flagName: "--project", group: "Filters", placeholder: "KEY", completion: "predictor=cacheproject,comma"},
		{commandPath: "jira issue mine", flagName: "--key", group: "Filters", placeholder: "KEY", completion: "predictor=issuekey,comma"},
		{commandPath: "jira issue mine", flagName: "--epic", group: "Filters", placeholder: "KEY", completion: "predictor=cacheepic,comma"},
		{commandPath: "jira issue mine", flagName: "--priority", group: "Filters", placeholder: "NAME", completion: "predictor=cachepriority,comma"},
		{commandPath: "jira issue mine", flagName: "--label", group: "Filters", placeholder: "NAME", completion: "predictor=cachelabel,comma"},
		{commandPath: "jira issue mine", flagName: "--type", group: "Filters", placeholder: "NAME", completion: "predictor=cacheissuetype,comma"},
		{commandPath: "jira issue mine", flagName: "--updated", group: "Filters", placeholder: "DATE"},
		{commandPath: "jira issue mine", flagName: "--created", group: "Filters", placeholder: "DATE"},
		{commandPath: "jira issue mine", flagName: "--resolved", group: "Filters", placeholder: "DATE"},
		{commandPath: "jira issue mine", flagName: "--order-by", group: "Sort", placeholder: "FIELD"},
		{commandPath: "jira issue mine", flagName: "--desc", group: "Sort"},

		{commandPath: "jira issue list", flagName: "--count", group: "Output"},
		{commandPath: "jira search jql", flagName: "--count", group: "Output"},
		{commandPath: "jira jql validate", flagName: "--mode", group: "Validation", placeholder: "MODE", enumContains: []string{"strict", "warn", "none"}},
		{commandPath: "jira search jql", flagName: "--all", group: "Pagination"},
		{commandPath: "jira search jql", flagName: "--limit", group: "Pagination", placeholder: "N"},
		{commandPath: "jira search jql", flagName: "--unbounded", group: "Pagination"},
		{commandPath: "jira jql build", flagName: "--status", group: "Filters", placeholder: "NAME", completion: "predictor=cachestatus,comma"},
		{commandPath: "jira jql build", flagName: "--priority", group: "Filters", placeholder: "NAME", completion: "predictor=cachepriority,comma"},

		{commandPath: "jira issue create", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue create", flagName: "--summary", group: "Fields", placeholder: "TEXT"},
		{commandPath: "jira issue create", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue create", flagName: "--assignee", group: "Fields", placeholder: "USER"},

		{commandPath: "jira issue edit", flagName: "--dry-run", group: "Safety"},
		{commandPath: "jira issue edit", flagName: "--json-input", group: "Input", placeholder: "FILE", valueHint: "file"},
		{commandPath: "jira issue edit", flagName: "--summary", group: "Fields", placeholder: "TEXT"},
		{commandPath: "jira issue edit", flagName: "--assignee", group: "Fields", placeholder: "USER"},

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
		return
	}
	t.Fatalf("schema missing flag %s on %s: %+v", want.flagName, want.commandPath, cmd.Flags)
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
