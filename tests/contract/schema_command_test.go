package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestSchemaCommandIncludesCommandTree(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--output=json", "agent", "schema")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	if env["data"] == nil {
		t.Fatalf("schema missing data: %+v", env)
	}
}

func TestSchemaCommandIncludesDetailedFlagSignatures(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--output=json", "agent", "schema")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Commands []struct {
				Flags []struct {
					Name      string `json:"name"`
					Type      string `json:"type"`
					Usage     string `json:"usage"`
					Shorthand string `json:"shorthand"`
					Default   string `json:"default"`
				} `json:"flags"`
			} `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	if len(env.Data.Commands) == 0 {
		t.Fatalf("schema missing root command:\n%s", out)
	}
	var found bool
	for _, flag := range env.Data.Commands[0].Flags {
		if flag.Name == "--profile" {
			found = true
			if flag.Type != "string" || flag.Shorthand != "p" || flag.Usage == "" {
				t.Fatalf("profile flag signature incomplete: %+v\n%s", flag, out)
			}
		}
	}
	if !found {
		t.Fatalf("schema missing --profile flag signature:\n%s", out)
	}
}

func TestAgentSchemaPublishesLiveLeafPathsAndFlagGroups(t *testing.T) {
	bin := buildJiraBinary(t)
	out, err := exec.Command(bin, "--output=json", "agent", "schema").CombinedOutput()
	if err != nil {
		t.Fatalf("agent schema error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			GlobalFlags []agentSchemaFlag    `json:"global_flags"`
			Commands    []agentSchemaCommand `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	outputFlag := findAgentSchemaFlag(env.Data.GlobalFlags, "--output")
	if outputFlag == nil {
		t.Fatalf("agent schema missing global --output flag: %+v", env.Data.GlobalFlags)
	}
	if outputFlag.EnumDefault != "auto" || len(outputFlag.EnumTerse) != 4 {
		t.Fatalf("agent schema dropped --output enum metadata: %+v", *outputFlag)
	}

	issueList := findAgentSchemaCommand(env.Data.Commands, "jira issue list")
	if issueList == nil {
		t.Fatalf("agent schema missing leaf command_path %q", "jira issue list")
	}
	for _, want := range []string{"--board", "--board-id", "--key"} {
		if !hasAgentSchemaFlag(issueList.Flags, want) {
			t.Fatalf("jira issue list schema missing local flag %s: %+v", want, issueList.Flags)
		}
	}
	if hasAgentSchemaFlag(issueList.Flags, "--output") {
		t.Fatalf("jira issue list duplicated global --output in local flags: %+v", issueList.Flags)
	}
	if !hasFlagGroup(issueList.MutuallyExclusiveFlags, "--board", "--board-id") {
		t.Fatalf("jira issue list missing board mutex group: %+v", issueList.MutuallyExclusiveFlags)
	}

	jqlBuild := findAgentSchemaCommand(env.Data.Commands, "jira jql build")
	if jqlBuild == nil {
		t.Fatalf("agent schema missing leaf command_path %q", "jira jql build")
	}
	if !hasAgentSchemaFlag(jqlBuild.Flags, "--key") {
		t.Fatalf("jira jql build schema missing local flag --key: %+v", jqlBuild.Flags)
	}

	issueLink := findAgentSchemaCommand(env.Data.Commands, "jira issue link")
	if issueLink == nil {
		t.Fatalf("agent schema missing leaf command_path %q", "jira issue link")
	}
	if !hasFlagGroup(issueLink.RequiredTogetherFlags, "--to", "--type") {
		t.Fatalf("jira issue link missing required-together --to/--type group: %+v", issueLink.RequiredTogetherFlags)
	}

	issueCreate := findAgentSchemaCommand(env.Data.Commands, "jira issue create")
	if issueCreate == nil {
		t.Fatalf("agent schema missing leaf command_path %q", "jira issue create")
	}
	if issueCreate.InputSchema != "issue.create" || issueCreate.OutputSchema != "issue.create" {
		t.Fatalf("jira issue create schema links = input %q output %q", issueCreate.InputSchema, issueCreate.OutputSchema)
	}
}

type agentSchemaCommand struct {
	Name                   string               `json:"name"`
	CommandPath            string               `json:"command_path"`
	Flags                  []agentSchemaFlag    `json:"flags"`
	Subcommands            []agentSchemaCommand `json:"subcommands"`
	MutuallyExclusiveFlags []stringSet          `json:"mutually_exclusive_flags"`
	RequiredTogetherFlags  []stringSet          `json:"required_together_flags"`
	InputSchema            string               `json:"input_schema"`
	OutputSchema           string               `json:"output_schema"`
}

type agentSchemaFlag struct {
	Name        string   `json:"name"`
	EnumDefault string   `json:"enum_default"`
	EnumTerse   []string `json:"enum_terse"`
}

type stringSet []string

func findAgentSchemaCommand(commands []agentSchemaCommand, path string) *agentSchemaCommand {
	for i := range commands {
		cmd := &commands[i]
		if cmd.CommandPath == path {
			return cmd
		}
		if found := findAgentSchemaCommand(cmd.Subcommands, path); found != nil {
			return found
		}
	}
	return nil
}

func hasAgentSchemaFlag(flags []agentSchemaFlag, name string) bool {
	return findAgentSchemaFlag(flags, name) != nil
}

func findAgentSchemaFlag(flags []agentSchemaFlag, name string) *agentSchemaFlag {
	for _, flag := range flags {
		if flag.Name == name {
			return &flag
		}
	}
	return nil
}

func hasFlagGroup(groups []stringSet, want ...string) bool {
	for _, group := range groups {
		if len(group) != len(want) {
			continue
		}
		seen := make(map[string]bool, len(group))
		for _, value := range group {
			seen[value] = true
		}
		ok := true
		for _, value := range want {
			if !seen[value] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
