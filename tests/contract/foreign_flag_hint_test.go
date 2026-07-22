package contract

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// The flag_foreign envelope orients callers arriving with another Jira
// CLI's flags: the hint says the flag belongs to a different tool, and
// the suggestions field carries this CLI's actual equivalents.

func TestUnknownForeignFlagProducesOrientationAndSuggestions(t *testing.T) {
	// Output (not CombinedOutput): the envelope is stdout-only, and `go
	// run` appends its own exit-status line to stderr on failure.
	out, err := exec.Command(buildJiraBinary(t),
		"issue", "view", "PROJ-1", "--plain", "--output=json").Output()
	if err == nil {
		t.Fatalf("--plain must be rejected:\n%s", out)
	}
	var env struct {
		Errors []struct {
			Code        string   `json:"code"`
			Hint        string   `json:"hint"`
			Suggestions []string `json:"suggestions"`
		} `json:"errors"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || len(env.Errors) == 0 {
		t.Fatalf("expected error envelope: %v\n%s", jerr, out)
	}
	if env.Errors[0].Code != "flag_foreign" {
		t.Fatalf("code = %q, want flag_foreign", env.Errors[0].Code)
	}
	wantHint := "That flag belongs to a different Jira CLI — use this CLI's equivalent instead."
	if env.Errors[0].Hint != wantHint {
		t.Fatalf("hint = %q, want the flag_foreign registry hint", env.Errors[0].Hint)
	}
	wantSugs := []string{"--output=human", "--output=json"}
	if len(env.Errors[0].Suggestions) != len(wantSugs) {
		t.Fatalf("suggestions = %v, want %v", env.Errors[0].Suggestions, wantSugs)
	}
	for i, want := range wantSugs {
		if env.Errors[0].Suggestions[i] != want {
			t.Fatalf("suggestions = %v, want %v", env.Errors[0].Suggestions, wantSugs)
		}
	}
}

// The equivalents table may only claim a flag is foreign while no command
// here defines it — otherwise the orientation hint lies about the CLI's
// own surface. The list below mirrors foreignFlagEquivalents; a flag
// added to either side without the other fails here or in the unit tests.
func TestForeignFlagTableNamesNoRealFlags(t *testing.T) {
	foreign := []string{"plain", "gjq", "template", "no-headers", "no-truncate", "paginate"}
	root := loadAgentSchema(t)
	var walk func(cmd docentSchema, visit func(docentSchemaFlag))
	walk = func(cmd docentSchema, visit func(docentSchemaFlag)) {
		for _, flag := range cmd.Flags {
			visit(flag)
		}
		for _, child := range cmd.Children {
			walk(child, visit)
		}
	}
	names := map[string]bool{}
	walk(root, func(flag docentSchemaFlag) { names[flag.Name] = true })
	for _, flag := range foreign {
		if names[flag] {
			t.Fatalf("flag %q exists in the live command surface; the foreign-flag orientation would lie about it", flag)
		}
	}
	// And the offered equivalents must be real flags of this CLI, or the
	// suggestion sends the caller to another dead end.
	for _, equivalent := range []string{"output", "limit", "all", "cursor"} {
		if !names[equivalent] {
			t.Fatalf("suggested equivalent flag %q is not in the live command surface", equivalent)
		}
	}
}
