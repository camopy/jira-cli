package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The local-state mutations (alias set/import, config init, cache refresh) must
// treat --dry-run the way the Jira mutations do: preview the change, report
// dry_run:true, and write nothing. These pin that they never persist.

func dryRunConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `default_profile = "default"
queries_path = "` + filepath.ToSlash(t.TempDir()) + `/queries"

[[profiles]]
name = "default"
base_url = "https://example.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAliasSetDryRunDoesNotPersist(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := dryRunConfig(t)

	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"alias", "set", "mine", "issue list --assignee me", "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("alias set --dry-run error = %v\n%s", err, out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Name   string `json:"name"`
			DryRun bool   `json:"dry_run"`
		} `json:"data"`
	}
	if e := json.Unmarshal(out, &env); e != nil {
		t.Fatalf("not JSON: %v\n%s", e, out)
	}
	if !env.OK || !env.Data.DryRun || env.Data.Name != "mine" {
		t.Fatalf("want ok+dry_run+name=mine, got %+v\n%s", env, out)
	}
	body, _ := os.ReadFile(cfg)
	if strings.Contains(string(body), "mine") {
		t.Fatalf("--dry-run persisted the alias into the config file:\n%s", body)
	}
}

func TestConfigInitDryRunWritesNoFile(t *testing.T) {
	bin := buildJiraBinary(t)
	cfgPath := filepath.Join(t.TempDir(), "new-config.toml")

	out, err := exec.Command(bin, "--config", cfgPath, "--output=json",
		"config", "init", "--base-url", "https://acme.atlassian.net",
		"--email", "me@example.com", "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("config init --dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			DryRun bool `json:"dry_run"`
		} `json:"data"`
	}
	if e := json.Unmarshal(out, &env); e != nil {
		t.Fatalf("not JSON: %v\n%s", e, out)
	}
	if !env.Data.DryRun {
		t.Fatalf("want dry_run:true, got %+v\n%s", env, out)
	}
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		t.Fatalf("config init --dry-run wrote a file at %s", cfgPath)
	}
}

// cache refresh --dry-run reports which resources are stale WITHOUT contacting
// Jira: the base URL points at a black-hole address, so any live fetch would
// hang/fail — a clean run proves the preview stays local.
func TestCacheRefreshDryRunStaysLocal(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := dryRunConfig(t)

	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"cache", "refresh", "labels", "--dry-run").CombinedOutput()
	if err != nil {
		t.Fatalf("cache refresh --dry-run error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "would-refresh") || !strings.Contains(string(out), `"dry_run":true`) {
		t.Fatalf("want a would-refresh dry_run row, got:\n%s", out)
	}
}
