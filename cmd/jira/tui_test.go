package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTUIOptionsTreatMissingCredentialsAsInitialState(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://company.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().String("config", cfg, "")
	root.PersistentFlags().String("profile", "work", "")
	cmd := &cobra.Command{Use: "tui"}
	root.AddCommand(cmd)

	opts := tuiOptionsForCommand(cmd)
	if !strings.Contains(opts.InitialError, `credential for profile "work" is required`) {
		t.Fatalf("InitialError = %q", opts.InitialError)
	}
	if opts.IssueProvider != nil {
		t.Fatalf("IssueProvider should be nil when credentials are missing")
	}
}
