package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// jiraConfigWithIncompleteProfile writes a config whose default profile is
// fully provisioned but whose "stub" profile has no base_url — the
// misconfigured shape a half-finished login or a hand-edited file leaves.
func jiraConfigWithIncompleteProfile(t *testing.T, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "default"
queries_path = "` + filepath.ToSlash(t.TempDir()) + `/queries"

[[profiles]]
name = "default"
base_url = "` + baseURL + `"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800

[[profiles]]
name = "stub"
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

type profileErrorEnvelope struct {
	OK   bool `json:"ok"`
	Meta struct {
		Command  string `json:"command"`
		ExitCode int    `json:"exit_code"`
	} `json:"meta"`
	Data   any `json:"data"`
	Errors []struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"errors"`
}

func assertProfileNotFound(t *testing.T, stdout []byte, exitCode int, args []string) {
	t.Helper()
	if exitCode != 2 {
		t.Errorf("jira %v: exit = %d, want 2", args, exitCode)
	}
	var env profileErrorEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("jira %v: stdout is not a JSON envelope: %v\nstdout=%s", args, err, stdout)
	}
	if env.OK {
		t.Errorf("jira %v: ok = true, want false", args)
	}
	if env.Data != nil {
		t.Errorf("jira %v: data = %v, want null (no fabricated result)", args, env.Data)
	}
	if len(env.Errors) != 1 {
		t.Fatalf("jira %v: len(errors) = %d, want 1\nstdout=%s", args, len(env.Errors), stdout)
	}
	if env.Errors[0].Type != "not_found" {
		t.Errorf("jira %v: errors[0].type = %q, want %q", args, env.Errors[0].Type, "not_found")
	}
	if env.Errors[0].Code != "profile_not_found" {
		t.Errorf("jira %v: errors[0].code = %q, want %q", args, env.Errors[0].Code, "profile_not_found")
	}
	if env.Errors[0].Hint == "" {
		t.Errorf("jira %v: errors[0].hint is empty, want remediation", args)
	}
}

// An unknown --profile must fail closed with exit 2 / profile_not_found on
// every command shape that used to fabricate success: view echoed the key
// back, list returned an "empty project", auth status ignored the flag.
func TestUnknownProfileFailsClosed(t *testing.T) {
	cfg := jiraConfig(t, "https://jira.invalid")
	for name, args := range map[string][]string{
		"issue_view":  {"--config", cfg, "--profile", "doesnotexist", "issue", "view", "PROJ-1", "--output=json"},
		"issue_list":  {"--config", cfg, "--profile", "doesnotexist", "issue", "list", "--output=json"},
		"me":          {"--config", cfg, "--profile", "doesnotexist", "me", "--output=json"},
		"auth_status": {"--config", cfg, "--profile", "doesnotexist", "auth", "status", "--no-probe", "--output=json"},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, _, exitCode := runJira(t, args...)
			assertProfileNotFound(t, stdout, exitCode, args)
		})
	}
}

// A profile that exists but has no base_url is just as unusable as an
// undefined one: same exit 2 / profile_not_found, never a fabricated result.
func TestIncompleteProfileFailsClosed(t *testing.T) {
	cfg := jiraConfigWithIncompleteProfile(t, "https://jira.invalid")
	for name, args := range map[string][]string{
		"issue_view": {"--config", cfg, "--profile", "stub", "issue", "view", "PROJ-1", "--output=json"},
		"issue_list": {"--config", cfg, "--profile", "stub", "issue", "list", "--output=json"},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, _, exitCode := runJira(t, args...)
			assertProfileNotFound(t, stdout, exitCode, args)
		})
	}
}

// auth status with an explicit --profile reports that profile — not the
// default set — so a preflight can actually observe the override.
func TestAuthStatusHonorsExplicitProfile(t *testing.T) {
	cfg := jiraConfigWithIncompleteProfile(t, "https://jira.invalid")
	args := []string{"--config", cfg, "--profile", "stub", "auth", "status", "--no-probe", "--output=json"}
	stdout, _, _ := runJira(t, args...)

	var env struct {
		Data struct {
			ActiveProfile string `json:"active_profile"`
			Profiles      []struct {
				Profile string `json:"profile"`
			} `json:"profiles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\nstdout=%s", err, stdout)
	}
	if env.Data.ActiveProfile != "stub" {
		t.Errorf("data.active_profile = %q, want %q", env.Data.ActiveProfile, "stub")
	}
	if len(env.Data.Profiles) != 1 || env.Data.Profiles[0].Profile != "stub" {
		t.Errorf("data.profiles = %+v, want exactly the requested profile", env.Data.Profiles)
	}
}
