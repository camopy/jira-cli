package boardscope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestFromFlagsReturnsConfigErrorForExplicitBoard(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[profiles\n"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().String("config", configPath, "")
	cmd := &cobra.Command{Use: "test"}
	AddFlags(cmd)
	root.AddCommand(cmd)
	if err := cmd.Flags().Set("board", "Platform"); err != nil {
		t.Fatalf("set --board: %v", err)
	}

	_, precedence, err := FromFlags(cmd)
	if err == nil {
		t.Fatal("FromFlags() error = nil, want invalid-config failure")
	}
	if precedence != precedenceFlag {
		t.Fatalf("precedence = %q, want %q", precedence, precedenceFlag)
	}
}

func TestFromFlagsReturnsConfigErrorForExplicitBoardID(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[profiles\n"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().String("config", configPath, "")
	cmd := &cobra.Command{Use: "test"}
	AddFlags(cmd)
	root.AddCommand(cmd)
	if err := cmd.Flags().Set("board-id", "42"); err != nil {
		t.Fatalf("set --board-id: %v", err)
	}

	_, precedence, err := FromFlags(cmd)
	if err == nil {
		t.Fatal("FromFlags() error = nil, want invalid-config failure")
	}
	if precedence != precedenceFlag {
		t.Fatalf("precedence = %q, want %q", precedence, precedenceFlag)
	}
}

func TestFromFlagsAllowsExplicitBlankBoardWithInvalidConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[profiles\n"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().String("config", configPath, "")
	cmd := &cobra.Command{Use: "test"}
	AddFlags(cmd)
	root.AddCommand(cmd)
	if err := cmd.Flags().Set("board", ""); err != nil {
		t.Fatalf("set --board: %v", err)
	}

	scope, precedence, err := FromFlags(cmd)
	if err != nil {
		t.Fatalf("FromFlags() error = %v, want explicit blank to suppress config default", err)
	}
	if _, applied := scope.JQLClause(); precedence != precedenceNone || applied {
		t.Fatalf("FromFlags() = (%#v, %q), want no board scope", scope, precedence)
	}
}
