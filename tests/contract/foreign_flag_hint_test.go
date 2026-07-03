package contract

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// The flag_unknown envelope teaches callers arriving with another Jira
// CLI's flags: the hint names the origin and this CLI's contract.

func TestUnknownForeignFlagProducesTeachingHint(t *testing.T) {
	// Output (not CombinedOutput): the envelope is stdout-only, and `go
	// run` appends its own exit-status line to stderr on failure.
	out, err := exec.Command("go", "run", "../../cmd/jira",
		"issue", "view", "PROJ-1", "--plain", "--output=json").Output()
	if err == nil {
		t.Fatalf("--plain must be rejected:\n%s", out)
	}
	var env struct {
		Errors []struct {
			Code string `json:"code"`
			Hint string `json:"hint"`
		} `json:"errors"`
	}
	if jerr := json.Unmarshal(out, &env); jerr != nil || len(env.Errors) == 0 {
		t.Fatalf("expected error envelope: %v\n%s", jerr, out)
	}
	if env.Errors[0].Code != "flag_unknown" {
		t.Fatalf("code = %q, want flag_unknown", env.Errors[0].Code)
	}
	hint := env.Errors[0].Hint
	for _, want := range []string{"ankitpokhrel/jira-cli", "--output=human", "--output=json"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint must teach the output contract, missing %q: %s", want, hint)
		}
	}
}

// The teaching table may only claim a flag is foreign while no command
// here defines it — otherwise the hint lies about the CLI's own surface.
// The list below mirrors foreignFlagHints; a flag added to either side
// without the other fails here or in the unit tests.
func TestForeignFlagTableNamesNoRealFlags(t *testing.T) {
	foreign := []string{"plain", "gjq", "template", "no-headers", "no-truncate", "paginate"}
	out, err := exec.Command("go", "run", "../../cmd/jira",
		"agent", "schema", "--output=compact").Output()
	if err != nil {
		t.Fatalf("agent schema: %v\n%s", err, out)
	}
	schema := string(out)
	for _, flag := range foreign {
		if strings.Contains(schema, `"`+flag+`"`) {
			t.Fatalf("flag %q exists in the live command surface; the foreign-flag hint would lie about it", flag)
		}
	}
}
