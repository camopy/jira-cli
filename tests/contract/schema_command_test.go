package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestSchemaCommandIncludesCommandTree(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/jira", "--json", "schema")
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
	cmd := exec.Command("go", "run", "../../cmd/jira", "--json", "schema")
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
