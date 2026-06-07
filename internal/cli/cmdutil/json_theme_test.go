package cmdutil_test

import (
	"path/filepath"
	"testing"

	clogtheme "github.com/gechr/clog/theme"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
)

// TestHumanJSONPrintThemeFollowsJiraTheme checks that the human-mode JSON
// highlighting theme tracks the JIRA_THEME override's light/dark background, so
// highlighted JSON stays readable on a light terminal. The config path points
// at a nonexistent file, isolating the resolution to the env override.
func TestHumanJSONPrintThemeFollowsJiraTheme(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env       string
		wantLight bool
	}{
		{"light theme resolves a light JSON palette", "catppuccin-latte", true},
		{"dark theme resolves a dark JSON palette", "catppuccin-mocha", false},
		{"explicit dark", "dark", false},
		{"unset falls back to dark", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(config.EnvThemeName, tc.env)
			cmd := &cobra.Command{Use: "x"}
			cmd.PersistentFlags().String("config", filepath.Join(t.TempDir(), "absent.toml"), "")

			wantBackground := clogtheme.Dark().Background
			if tc.wantLight {
				wantBackground = clogtheme.Light().Background
			}
			got := cmdutil.HumanJSONPrintTheme(cmd)
			if got.Background != wantBackground {
				t.Errorf("HumanJSONPrintTheme(%q) background = %v, want %v", tc.env, got.Background, wantBackground)
			}
		})
	}
}
