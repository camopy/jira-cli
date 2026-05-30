package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func listColumnsConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	body := `default_profile = "default"
queries_path = "` + dir + `/queries"

[[profiles]]
name = "default"
base_url = "https://acme.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
`
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	return cfg
}

// An unknown --columns value is a flag-value validation failure (exit 3),
// even when the bad value is a word the substring error classifier would
// otherwise bucket elsewhere ("auth" → auth, "server" → server). The typed
// error keeps the user-supplied column value off that classifier. Validation
// runs before any Jira call, so no network or live credential is needed.
func TestIssueListUnknownColumnIsValidationExit3(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := listColumnsConfig(t)
	for _, bad := range []string{"bogus", "auth", "server"} {
		var env struct {
			Errors []struct {
				Type string `json:"type"`
				Code string `json:"code"`
				Flag string `json:"flag"`
			} `json:"errors"`
		}
		cmd := exec.Command(bin, "--config", cfg, "issue", "list", "--columns", bad, "--output=json")
		_, _, runErr := runCommandExpectErrorEnvelope(t, cmd, &env)
		assertValidationExitCode(t, runErr)

		if len(env.Errors) != 1 {
			t.Fatalf("--columns %q want exactly one error, got %d", bad, len(env.Errors))
		}
		e := env.Errors[0]
		if e.Type != "validation" || e.Code != "flag_value_invalid" || e.Flag != "columns" {
			t.Fatalf("--columns %q error = {type:%q code:%q flag:%q}, want validation/flag_value_invalid/columns",
				bad, e.Type, e.Code, e.Flag)
		}
	}
}
