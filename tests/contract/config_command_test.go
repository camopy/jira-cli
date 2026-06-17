package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigInitProfileGetSetMetadataOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	cmd := exec.Command(
		"go", "run", "../../cmd/jira",
		"--config", path,
		"--output=json",
		"config", "init",
		"--no-input",
		"--profile", "default",
		"--base-url", "https://company.atlassian.net",
		"--email", "dev@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config init error = %v\n%s", err, out)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), "api-token") || strings.Contains(string(content), "password =") {
		t.Fatalf("config file contains secret material:\n%s", content)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", path, "--output=json", "config", "get", "default_profile")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config get error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("config get output is not JSON: %v\n%s", err, out)
	}
	if env.Data.Key != "default_profile" || env.Data.Value != "default" {
		t.Fatalf("default_profile envelope = %q", out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", path, "--output=json", "config", "set", "theme.path", "/tmp/theme.toml")
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("config set error = %v\n%s", err, out)
	}

	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", path, "--output=json", "config", "profile")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config --help error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "default") {
		t.Fatalf("profile output = %q", out)
	}
}

func TestConfigInitNoInputRequiresProfileFieldsAndDoesNotMutateConfig(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	before := []byte(`default_profile = "default"

[[profiles]]
name = "default"
base_url = "https://work.atlassian.net"
auth_type = "token"
email = "dev@example.com"
account_id = "account-123"
secret_backend = "keyring"
onepassword_account = "Team"
vault = "Private"
item = "jira-cli-work"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command(bin, "--config", path, "--output=json", "config", "init", "--no-input", "--profile", "default")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("config init without profile fields succeeded:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	assertValidationExitCode(t, err)
	var env struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	decodeErrorEnvelopeFromStdout(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, &env)
	if len(env.Errors) == 0 {
		t.Fatalf("config init envelope carried no errors:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if env.Errors[0].Code != "required_flag_missing" {
		t.Fatalf("config init error code = %q, want required_flag_missing\nstderr=%s", env.Errors[0].Code, stderr.String())
	}
	if !strings.Contains(env.Errors[0].Message, "--base-url") || !strings.Contains(env.Errors[0].Message, "--email") {
		t.Fatalf("config init error should name both required profile fields, got %q", env.Errors[0].Message)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("config init without required fields mutated config.toml\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestConfigThemeRejectsUnknownPreset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", path, "--output=json", "config", "theme", "--name", "no-such-theme")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("config theme accepted unknown preset:\n%s", out)
	}
	if !strings.Contains(string(out), "unknown theme") {
		t.Fatalf("error did not explain unknown theme:\n%s", out)
	}
}

func TestConfigThemeRejectsLegacyPresetName(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	before := []byte(`default_profile = "default"

[[profiles]]
name = "default"
auth_type = "token"

[theme]
name = "dark"
`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command(bin, "--config", path, "--output=json", "config", "theme", "--name", "default")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("config theme accepted legacy preset:\n%s", out)
	}
	if !strings.Contains(string(out), "unknown theme") {
		t.Fatalf("error did not explain unknown theme:\n%s", out)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("config theme with legacy preset mutated config.toml\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestConfigSetRejectsLegacyThemeName(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	before := []byte(`default_profile = "default"

[[profiles]]
name = "default"
auth_type = "token"

[theme]
name = "dark"
`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command(bin, "--config", path, "--output=json", "config", "set", "theme.name", "default")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("config set accepted legacy theme.name:\n%s", out)
	}
	if !strings.Contains(string(out), "unknown theme") {
		t.Fatalf("error did not explain unknown theme:\n%s", out)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("config set with legacy theme.name mutated config.toml\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestReadOnlyCommandToleratesUnknownThemeName(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`default_profile = "default"

[[profiles]]
name = "default"
auth_type = "token"

[theme]
name = "no-such-theme"
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command(bin, "--config", path, "--output=json", "config", "profile")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config profile rejected unknown theme.name: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "default") {
		t.Fatalf("profile output = %q", out)
	}
}

// A read-only command pointed at a missing explicit --config must not
// create that file on disk: an explicit-path typo is a hard error, not a
// silent default-config write.
func TestReadOnlyCommandDoesNotCreateConfigForExplicitMissingPath(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "missing.toml")
	cmd := exec.Command(bin, "--config", path, "--output=json", "config", "profile")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("config profile with missing --config succeeded:\n%s", out)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("read-only command created a config file for a missing explicit path: %v", statErr)
	}
}

// A config write command must persist only file-backed values: transient
// JIRA_* env overlays must not leak into the saved TOML.
func TestConfigSetDoesNotPersistEnvOverlay(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://work.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command(bin, "--config", path, "--output=json", "config", "set", "profiles.work.default_project", "REAL")
	cmd.Env = append(
		os.Environ(),
		"JIRA_PROFILE_WORK_BASE_URL=https://overlay.atlassian.net",
		"JIRA_PROFILE_WORK_DEFAULT_ISSUE_TYPE=OverlayType",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config set error = %v\n%s", err, out)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), "overlay.atlassian.net") {
		t.Fatalf("config set persisted JIRA_PROFILE_*_BASE_URL env overlay into TOML:\n%s", content)
	}
	if strings.Contains(string(content), "OverlayType") {
		t.Fatalf("config set persisted JIRA_PROFILE_*_DEFAULT_ISSUE_TYPE env overlay into TOML:\n%s", content)
	}
	// The file-backed base URL and the explicitly-set key must survive.
	if !strings.Contains(string(content), "work.atlassian.net") {
		t.Fatalf("config set dropped the file-backed base URL:\n%s", content)
	}
	if !strings.Contains(string(content), `default_project = "REAL"`) {
		t.Fatalf("config set did not persist the explicitly-set value:\n%s", content)
	}
}

// auth switch is a read-modify-write command; it must not bake a
// JIRA_DEFAULT_PROFILE env overlay into the persisted default_profile.
func TestAuthSwitchDoesNotPersistEnvDefaultProfile(t *testing.T) {
	bin := buildJiraBinary(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://work.atlassian.net"
auth_type = "token"
secret_backend = "keyring"

[[profiles]]
name = "play"
base_url = "https://play.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := exec.Command(bin, "--config", path, "--output=json", "auth", "switch", "play")
	cmd.Env = append(os.Environ(), "JIRA_DEFAULT_PROFILE=phantom")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth switch error = %v\n%s", err, out)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), "phantom") {
		t.Fatalf("auth switch persisted JIRA_DEFAULT_PROFILE env overlay into TOML:\n%s", content)
	}
	if !strings.Contains(string(content), `default_profile = "play"`) {
		t.Fatalf("auth switch did not persist the requested default_profile:\n%s", content)
	}
}

// Shell completion is a read-only path: generating the completion script
// must not create the default config file as a side effect.
func TestCompletionDoesNotCreateDefaultConfigFile(t *testing.T) {
	bin := buildJiraBinary(t)
	home := t.TempDir()
	xdg := filepath.Join(t.TempDir(), "xdg")
	cmd := exec.Command(bin, "completion", "bash")
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("completion bash error = %v\n%s", err, out)
	}
	cfgFile := filepath.Join(xdg, "jira-cli", "config.toml")
	if _, statErr := os.Stat(cfgFile); !os.IsNotExist(statErr) {
		t.Fatalf("completion created the default config file: %v", statErr)
	}
}
