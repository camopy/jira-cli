package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func TestSchemaRegistry(t *testing.T) {
	reg := cli.NewSchemaRegistry()
	reg.Register(cli.CommandSchema{
		Command: "issue.list",
		Flags:   []cli.FlagSchema{{Name: "detail", Type: "bool"}},
		OutputSchema: map[string]any{
			"type": "object",
		},
	})
	got, ok := reg.Get("issue.list")
	if !ok || got.Command != "issue.list" || len(got.Flags) != 1 {
		t.Fatalf("schema lookup = %+v ok=%v", got, ok)
	}
	if len(reg.Commands()) != 1 {
		t.Fatalf("commands = %+v", reg.Commands())
	}
}
