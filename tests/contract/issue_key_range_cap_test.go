package contract

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIssueKeyRangeExpansionCapIsEnforcedBeforeCredentialsOrNetwork(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Errorf("unexpected network call before range-cap validation: %s %s", r.Method, r.URL.String())
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := jiraConfig(t, srv.URL)
	tests := []struct {
		name string
		args []string
	}{
		{name: "issue view positional range", args: []string{"--config", cfg, "--output=json", "issue", "view", "PROJ-1..1001"}},
		{name: "issue edit dry-run range", args: []string{"--config", cfg, "--output=json", "issue", "edit", "PROJ-1..1001", "--summary", "bulk", "--dry-run"}},
		{name: "issue list key range", args: []string{"--config", cfg, "--output=json", "issue", "list", "--key", "PROJ-1..1001"}},
		{name: "jql build key range", args: []string{"--config", cfg, "--output=json", "jql", "build", "--key", "PROJ-1..1001"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := calls.Load()
			cmd := exec.Command(bin, tt.args...)
			cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=")
			var env struct {
				OK     bool `json:"ok"`
				Errors []struct {
					Type    string `json:"type"`
					Code    string `json:"code"`
					Message string `json:"message"`
					Hint    string `json:"hint"`
				} `json:"errors"`
			}
			_, _, err := runCommandExpectErrorEnvelope(t, cmd, &env)
			assertValidationExitCode(t, err)
			if env.OK || len(env.Errors) == 0 {
				t.Fatalf("%s range cap envelope = %+v", tt.name, env)
			}
			if !strings.Contains(env.Errors[0].Message, "issue key expansion exceeds maximum of 1000 keys") {
				t.Fatalf("%s range cap message = %q", tt.name, env.Errors[0].Message)
			}
			if !strings.Contains(env.Errors[0].Hint, "Split the key set into smaller invocations") {
				t.Fatalf("%s range cap hint = %q", tt.name, env.Errors[0].Hint)
			}
			if got := calls.Load(); got != before {
				t.Fatalf("%s made %d network calls before local range-cap validation", tt.name, got-before)
			}
		})
	}
}
