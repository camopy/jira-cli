package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestClibMetadataForRemainingCommandFamilies(t *testing.T) {
	bin := buildJiraBinary(t)
	out, err := exec.Command(bin, "--output=json", "agent", "schema").CombinedOutput()
	if err != nil {
		t.Fatalf("agent schema error = %v\n%s", err, out)
	}

	var env remainingMetadataEnvelope
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}

	for _, want := range []remainingFlagWant{
		{path: "jira cache labels", name: "--refresh", group: "Cache"},
		{path: "jira cache labels", name: "--ttl-minutes", group: "Cache", placeholder: "N"},
		{path: "jira cache projects", name: "--refresh", group: "Cache"},
		{path: "jira cache epics", name: "--refresh", group: "Cache"},
		{path: "jira cache fields", name: "--refresh", group: "Cache"},
		{path: "jira cache issuetypes", name: "--refresh", group: "Cache"},
		{path: "jira cache linktypes", name: "--refresh", group: "Cache"},
		{path: "jira cache boards", name: "--refresh", group: "Cache"},
		{path: "jira cache boards", name: "--unbounded", group: "Pagination"},

		{path: "jira boards list", name: "--refresh", group: "Cache"},
		{path: "jira boards list", name: "--ttl-minutes", group: "Cache", placeholder: "N"},
		{path: "jira boards list", name: "--unbounded", group: "Pagination"},

		{path: "jira issue attachment list", name: "--limit", group: "Pagination", placeholder: "N"},
		{path: "jira issue attachment list", name: "--all", group: "Pagination"},
		{path: "jira issue attachment add", name: "--file", group: "Input", placeholder: "PATH", valueHint: "file"},
		{path: "jira issue attachment add", name: "--dry-run", group: "Safety"},
		{path: "jira issue attachment delete", name: "--force", group: "Safety"},
		{path: "jira issue attachment delete", name: "--dry-run", group: "Safety"},
		{path: "jira issue attachment download", name: "--to", group: "Output", placeholder: "PATH", valueHint: "file"},
		{path: "jira issue attachment download", name: "--force", group: "Safety"},
		{path: "jira issue attachment download", name: "--dry-run", group: "Safety"},

		{path: "jira issue link", name: "--to", group: "Link", placeholder: "KEY", completion: "predictor=issuekey"},
		{path: "jira issue link", name: "--type", group: "Link", placeholder: "NAME", completion: "predictor=cachelinktype"},
		{path: "jira issue link", name: "--dry-run", group: "Safety"},
		{path: "jira issue link delete", name: "--force", group: "Safety"},
		{path: "jira issue link delete", name: "--dry-run", group: "Safety"},
		{path: "jira issue link types", name: "--refresh", group: "Cache"},
		{path: "jira issue link types", name: "--ttl-minutes", group: "Cache", placeholder: "N"},

		{path: "jira issue watch", name: "--dry-run", group: "Safety"},
		{path: "jira issue watch", name: "--no-readback", group: "Validation"},
		{path: "jira issue watch", name: "--validate-remote", group: "Validation"},
		{path: "jira issue unwatch", name: "--dry-run", group: "Safety"},
		{path: "jira issue unwatch", name: "--no-readback", group: "Validation"},
		{path: "jira issue unwatch", name: "--validate-remote", group: "Validation"},
		{path: "jira issue watchers add", name: "--user", group: "User", placeholder: "IDENTIFIER"},
		{path: "jira issue watchers add", name: "--dry-run", group: "Safety"},
		{path: "jira issue watchers add", name: "--no-readback", group: "Validation"},
		{path: "jira issue watchers add", name: "--validate-remote", group: "Validation"},
		{path: "jira issue watchers remove", name: "--user", group: "User", placeholder: "IDENTIFIER"},
		{path: "jira issue watchers remove", name: "--dry-run", group: "Safety"},
		{path: "jira issue watchers remove", name: "--no-readback", group: "Validation"},
		{path: "jira issue watchers remove", name: "--validate-remote", group: "Validation"},

		{path: "jira epic add", name: "--dry-run", group: "Safety"},
		{path: "jira epic remove", name: "--dry-run", group: "Safety"},
		{path: "jira alias import", name: "--clobber", group: "Safety"},
	} {
		requireRemainingFlag(t, env.Data.Commands, want)
	}
}

type remainingMetadataEnvelope struct {
	Data struct {
		Commands []remainingMetadataCommand `json:"commands"`
	} `json:"data"`
}

type remainingMetadataCommand struct {
	CommandPath string                     `json:"command_path"`
	Flags       []remainingMetadataFlag    `json:"flags"`
	Subcommands []remainingMetadataCommand `json:"subcommands"`
}

type remainingMetadataFlag struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	Placeholder string `json:"placeholder"`
	Completion  string `json:"completion"`
	ValueHint   string `json:"value_hint"`
}

type remainingFlagWant struct {
	path        string
	name        string
	group       string
	placeholder string
	completion  string
	valueHint   string
}

func requireRemainingFlag(t *testing.T, commands []remainingMetadataCommand, want remainingFlagWant) {
	t.Helper()
	cmd, ok := findRemainingCommand(commands, want.path)
	if !ok {
		t.Fatalf("schema missing command_path %q", want.path)
	}
	for _, flag := range cmd.Flags {
		if flag.Name != want.name {
			continue
		}
		if flag.Group != want.group {
			t.Fatalf("%s %s group = %q, want %q", want.path, want.name, flag.Group, want.group)
		}
		if want.placeholder != "" && flag.Placeholder != want.placeholder {
			t.Fatalf("%s %s placeholder = %q, want %q", want.path, want.name, flag.Placeholder, want.placeholder)
		}
		if want.completion != "" && flag.Completion != want.completion {
			t.Fatalf("%s %s completion = %q, want %q", want.path, want.name, flag.Completion, want.completion)
		}
		if want.valueHint != "" && flag.ValueHint != want.valueHint {
			t.Fatalf("%s %s value_hint = %q, want %q", want.path, want.name, flag.ValueHint, want.valueHint)
		}
		return
	}
	t.Fatalf("%s missing flag %s: %+v", want.path, want.name, cmd.Flags)
}

func findRemainingCommand(commands []remainingMetadataCommand, path string) (remainingMetadataCommand, bool) {
	for _, cmd := range commands {
		if cmd.CommandPath == path {
			return cmd, true
		}
		if found, ok := findRemainingCommand(cmd.Subcommands, path); ok {
			return found, true
		}
	}
	return remainingMetadataCommand{}, false
}
