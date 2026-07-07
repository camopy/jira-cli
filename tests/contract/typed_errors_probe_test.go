// Every intentional command error must reach the envelope through a typed
// errtax.Coded error, never through classifyUntyped's legacy substring
// branches. The probe below is the objective standard: it fires for every
// error the substring classifier receives, so a scenario passes only when
// its real command error is typed AND lands on the intended code/exit.
package contract

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/root"
	"github.com/matcra587/jira-cli/internal/cli/runtime"
)

// typedErrorServer answers the read-only endpoints the scenarios may
// legitimately touch before their error fires, and 500s everything else so
// an unexpected wire call produces a loud, distinctive jira_server_error
// instead of silently satisfying a scenario.
func typedErrorServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/user/search", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/rest/api/3/issue/PROJ-1/editmeta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fields":{}}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errorMessages":["unexpected wire call in typed-error scenario"],"errors":{}}`))
	})
	return httptest.NewServer(mux)
}

// runCommandInProcess executes one command through the real root tree in
// this process and returns the raw RunE error, so the probe observes any
// trip through the substring classifier.
func runCommandInProcess(t *testing.T, args ...string) error {
	t.Helper()
	rt, err := runtime.New()
	if err != nil {
		t.Fatalf("runtime.New: %v", err)
	}
	rootCmd := root.New(rt)
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(args)
	return rootCmd.ExecuteContext(context.Background())
}

// jiraConfigNonLocal writes a config whose base URL is NOT loopback, so the
// missing-credential guard in cmdutil.JiraClientForCommand actually fires
// (loopback URLs tolerate an absent credential by design).
func jiraConfigNonLocal(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "credmiss"

[[profiles]]
name = "credmiss"
base_url = "https://credmiss.invalid"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

// jiraConfigWithEmptyQueries writes a config whose queries_path EXISTS but
// holds no saved queries, so `search saved <name>` reaches the name lookup
// itself rather than failing to read the directory.
func jiraConfigWithEmptyQueries(t *testing.T, baseURL string) string {
	t.Helper()
	queriesDir := filepath.Join(t.TempDir(), "queries")
	if err := os.MkdirAll(queriesDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "default"
queries_path = "` + filepath.ToSlash(queriesDir) + `"

[[profiles]]
name = "default"
base_url = "` + baseURL + `"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

// jiraConfigDefaultNoBaseURL writes a config whose DEFAULT profile has no
// base URL, so the unrequested (ActiveProfile) client path returns
// ok=false without an error and `auth whoami` reaches its own
// incomplete-profile guard rather than the requested-profile guard.
func jiraConfigDefaultNoBaseURL(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "default"

[[profiles]]
name = "default"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

// TestCommandErrorsAreTypedNotSubstringClassified drives each scenario's
// REAL command path headless (non-TTY test process, no --force) and asserts
// the returned error (a) never reaches classifyUntyped and (b) maps to the
// intended stable code and exit. Must not run in parallel: the probe is a
// package-level seam.
func TestCommandErrorsAreTypedNotSubstringClassified(t *testing.T) {
	srv := typedErrorServer()
	defer srv.Close()

	scenarios := []struct {
		name string
		args []string // appended after {--config <cfg>}
		// config overrides the default jiraConfig for scenarios that need
		// a special one; receives the fake server's URL.
		config   func(t *testing.T, srvURL string) string
		wantCode string
		wantExit int
	}{
		// The 8 --force gates: destructive/local-state mutations refused
		// headless without --force. All must stay validation_failed / 3.
		{"force-gate/issue-delete", []string{"issue", "delete", "PROJ-1"}, nil, "validation_failed", 3},
		{"force-gate/issue-delete-multi", []string{"issue", "delete", "PROJ-1", "PROJ-2"}, nil, "validation_failed", 3},
		{"force-gate/issue-clone", []string{"issue", "clone", "PROJ-1"}, nil, "validation_failed", 3},
		{"force-gate/issue-move", []string{"issue", "move", "PROJ-1"}, nil, "validation_failed", 3},
		{"force-gate/comment-delete", []string{"issue", "comment", "delete", "PROJ-1", "500"}, nil, "validation_failed", 3},
		{"force-gate/attachment-delete", []string{"issue", "attachment", "delete", "PROJ-1", "99"}, nil, "validation_failed", 3},
		{"force-gate/link-delete", []string{"issue", "link", "delete", "PROJ-1", "10000"}, nil, "validation_failed", 3},
		{"force-gate/cache-clear", []string{"cache", "clear"}, nil, "validation_failed", 3},
		{"force-gate/auth-logout", []string{"auth", "logout", "default"}, nil, "validation_failed", 3},
		// (the update --force gate needs the package-local channel stub;
		// it is probed in internal/cli/update's own test.)

		// auth login --backend that differs from the profile's stored
		// backend: bad flag input, must be validation / 3 — NOT auth / 1.
		{
			"auth-login-backend-mismatch",
			[]string{"auth", "login", "--no-input", "--base-url", "https://company.atlassian.net", "--backend", "1password", "--vault", "Engineering", "--item", "jira-x"},
			nil, "flag_value_invalid", 3,
		},

		// Unknown guide section / saved query name: bad CLI argument
		// values, must be validation / 3 — NOT not_found / 2 (nothing was
		// looked up in Jira).
		{"agent-guide-unknown-section", []string{"agent", "guide", "no_such_section"}, nil, "arg_value_invalid", 3},
		{
			"search-saved-unknown-query",
			[]string{"search", "saved", "no-such-query"},
			jiraConfigWithEmptyQueries, "arg_value_invalid", 3,
		},

		// A genuinely absent Jira user (live /user/search returned zero
		// matches): stays not_found / 2, but must be typed.
		{
			"watchers-user-not-found",
			[]string{"issue", "watchers", "add", "PROJ-1", "--user", "missing@example.com", "--dry-run", "--validate-remote"},
			nil, "not_found", 2,
		},

		// Missing credential for a non-local profile: stays auth / 1 with
		// the credential-missing identity.
		{
			"credential-missing",
			[]string{"issue", "view", "PROJ-1"},
			func(t *testing.T, _ string) string { return jiraConfigNonLocal(t) },
			"credential_missing", 1,
		},

		// `auth whoami` against a default profile with no base URL: the
		// profile resolved but is incomplete — profile_incomplete / exit 2,
		// typed, NOT substring-guessed as auth / 1 off the "auth.whoami"
		// text.
		{
			"whoami-default-profile-no-base-url",
			[]string{"auth", "whoami"},
			func(t *testing.T, _ string) string { return jiraConfigDefaultNoBaseURL(t) },
			"profile_incomplete", 2,
		},

		// `--assignee me` on a profile with no account_id: the flag value
		// cannot be satisfied — validation `flag_value_invalid` / exit 3,
		// NOT auth / 1 off the "jira auth whoami" hint in the message.
		{
			"assignee-me-without-account-id",
			[]string{"issue", "edit", "PROJ-1", "--assignee", "me", "--no-input"},
			nil, "flag_value_invalid", 3,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			// A resolvable credential for the default profile so client
			// construction (clone / move resolve one up front) succeeds
			// without touching a keyring; the gates fire before any wire
			// call. The credential-missing scenario uses its own profile
			// name, so this env var does not satisfy it.
			t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")
			cfg := jiraConfig(t, srv.URL)
			if sc.config != nil {
				cfg = sc.config(t, srv.URL)
			}

			var probed []error
			cli.ClassifyUntypedProbe = func(err error) { probed = append(probed, err) }
			t.Cleanup(func() { cli.ClassifyUntypedProbe = nil })

			err := runCommandInProcess(t, append([]string{"--config", cfg}, sc.args...)...)
			if err == nil {
				t.Fatalf("command %v succeeded; want the scenario error", sc.args)
			}
			mapped := cli.MapError(err)
			for _, p := range probed {
				t.Errorf("error reached the substring classifier (must be typed): %v", p)
			}
			if mapped.Code != sc.wantCode {
				t.Errorf("code = %q, want %q (error: %v)", mapped.Code, sc.wantCode, err)
			}
			if got := cli.ExitCode(mapped); got != sc.wantExit {
				t.Errorf("exit = %d, want %d (error: %v)", got, sc.wantExit, err)
			}
		})
	}
}
