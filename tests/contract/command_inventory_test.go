package contract

import (
	"encoding/json"
	"os/exec"
	"sort"
	"testing"
)

// schemaCommand mirrors the `agent schema` command-tree node shape. Only
// the fields the inventory contract depends on are decoded.
type schemaCommand struct {
	Name     string          `json:"name"`
	Children []schemaCommand `json:"children"`
}

// expectedTopLevelCommands is the locked set of top-level command names
// the CLI's `agent schema` output must report. The per-instance root
// factory must produce exactly this surface: a behavior-preserving
// root/runtime refactor cannot drop or rename a command. The shell
// `completion` command is excluded by cobra, and the agent group and
// human guide door are excluded by design: the schema represents the host
// CLI without the discovery surface itself.
var expectedTopLevelCommands = []string{
	"alias",
	"auth",
	"boards",
	"cache",
	"config",
	"epic",
	"issue",
	"jql",
	"me",
	"release-notes",
	"search",
	"tui",
	"version",
	"worklog",
}

// runSchema runs the freshly built binary's `agent schema` command and
// returns the decoded root command-tree node.
func runSchema(t *testing.T) schemaCommand {
	t.Helper()
	bin := buildJiraBinary(t)
	out, err := exec.Command(bin, "--output=json", "agent", "schema").CombinedOutput()
	if err != nil {
		t.Fatalf("agent schema error = %v\n%s", err, out)
	}
	var root schemaCommand
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out)
	}
	if len(root.Children) == 0 {
		t.Fatalf("schema reported no command tree:\n%s", out)
	}
	return root
}

// TestCommandInventoryIsStable asserts every promised top-level command
// is present in the binary's reported command tree. It checks
// containment so a future command addition does not fail the test, but a
// removal or rename does.
func TestCommandInventoryIsStable(t *testing.T) {
	root := runSchema(t)

	have := map[string]bool{}
	var names []string
	for _, sub := range root.Children {
		have[sub.Name] = true
		names = append(names, sub.Name)
	}
	sort.Strings(names)

	for _, want := range expectedTopLevelCommands {
		if !have[want] {
			t.Errorf("command inventory missing top-level command %q; have %v", want, names)
		}
	}
}

// TestCommandInventoryHasNoDuplicateTopLevelNames asserts the per-instance
// root attaches each command once: a duplicated registration would mean
// root construction ran a registration step twice.
func TestCommandInventoryHasNoDuplicateTopLevelNames(t *testing.T) {
	root := runSchema(t)

	seen := map[string]int{}
	for _, sub := range root.Children {
		seen[sub.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("top-level command %q appears %d times; root construction registered it more than once", name, count)
		}
	}
}
