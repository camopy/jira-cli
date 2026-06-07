// Package config implements the `jira config` cobra command tree, which
// manages the local configuration file, profiles, and theme settings.
package config

import (
	"fmt"
	"strings"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// NewCommand returns the `config` command group: init, profile, get, set, and
// theme.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("config", "Manage configuration", "configuration")
	cmd.AddCommand(configInitCommand())
	cmd.AddCommand(configProfileCommand())
	cmd.AddCommand(configGetCommand())
	cmd.AddCommand(configSetCommand())
	cmd.AddCommand(configThemeCommand())
	return cmd
}

func configThemeCommand() *cobra.Command {
	var name, path string
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "Manage theme configuration",
		Example: `# Show the current theme configuration
$ jira config theme

# Set a built-in theme by name
$ jira config theme --name dracula

# Load a theme from a TOML file
$ jira config theme --path ./my-theme.toml`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			changed := false
			if cmd.Flags().Changed("name") {
				// Validate here, not in cfg.Validate(): config load tolerates an
				// unrecognized theme.name (it falls back to dark), but setting one
				// should reject a typo up front.
				if err := config.ValidateThemeName(name); err != nil {
					return err
				}
				cfg.Theme.Name = name
				changed = true
			}
			if cmd.Flags().Changed("path") {
				cfg.Theme.Path = path
				changed = true
			}
			if changed {
				if err := cfg.Validate(); err != nil {
					return err
				}
				if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
					return err
				}
			}
			return cmdutil.WriteEnvelope(cmd, "config.theme", map[string]any{
				"name":    cfg.Theme.Name,
				"path":    cfg.Theme.Path,
				"changed": changed,
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Theme name")
	cmd.Flags().StringVar(&path, "path", "", "Theme TOML path")
	// Theme names are self-describing (dracula, nord, catppuccin-mocha), so a
	// per-value EnumTerse would just restate the value. A short Terse keeps the
	// completion description from falling back to the flag usage instead.
	clib.Extend(cmd.Flags().Lookup("name"), clib.FlagExtra{
		Group:       "Theme",
		Placeholder: "NAME",
		Terse:       "theme name",
		Enum:        config.ThemeNameValues,
		EnumDefault: "auto",
	})
	clib.Extend(cmd.Flags().Lookup("path"), clib.FlagExtra{
		Group:       "Theme",
		Placeholder: "PATH",
		Hint:        "file",
	})
	return cmd
}

func configInitCommand() *cobra.Command {
	var baseURL, email string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create initial configuration",
		Example: `# Create the initial config file for the default profile
$ jira config init --base-url https://acme.atlassian.net --email me@example.com

# Create config under a named profile
$ jira config init --profile work --base-url https://acme.atlassian.net --email me@example.com`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile := cmdutil.RequestedProfile(cmd)
			if profile == "" {
				profile = "default"
			}
			baseURL = config.NormalizeBaseURL(baseURL)
			email = strings.TrimSpace(email)
			if missing := missingConfigInitRequiredFlags(baseURL, email); len(missing) > 0 {
				return configInitRequiredFlagError(missing)
			}
			cfg := config.Defaults()
			cfg.DefaultProfile = profile
			cfg.Profiles = []config.Profile{{
				Name:            profile,
				BaseURL:         baseURL,
				AuthType:        config.AuthTypeToken,
				Email:           email,
				SecretBackend:   config.SecretBackendKeyring,
				RefreshInterval: config.DefaultRefreshIntervalSeconds,
				TimeoutSeconds:  config.DefaultTimeoutSeconds,
				WorkdaySeconds:  config.DefaultWorkdaySeconds,
			}}
			if err := cfg.Validate(); err != nil {
				return err
			}
			if err := config.Save(cmdutil.ConfigPath(cmd), &cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "config.init", map[string]any{
				"profile":     profile,
				"base_url":    baseURL,
				"auth_type":   string(config.AuthTypeToken),
				"stored_auth": false,
			})
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Jira base URL")
	cmd.Flags().StringVar(&email, "email", "", "Jira account email")
	return cmd
}

func missingConfigInitRequiredFlags(baseURL, email string) []string {
	var missing []string
	if strings.TrimSpace(baseURL) == "" {
		missing = append(missing, "base-url")
	}
	if strings.TrimSpace(email) == "" {
		missing = append(missing, "email")
	}
	return missing
}

func configInitRequiredFlagError(missing []string) error {
	quoted := make([]string, len(missing))
	for i, name := range missing {
		quoted[i] = "--" + name
	}
	err := cli.NewCLIInputError(
		cli.InputRequiredFlagMissing,
		fmt.Sprintf("required flag(s) %s not set", strings.Join(quoted, ", ")),
	)
	err.Flag = missing[0]
	return err
}

func configProfileCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "List configured profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profiles := make([]map[string]any, 0, len(cfg.Profiles))
			for _, p := range cfg.Profiles {
				profiles = append(profiles, map[string]any{
					"name":   p.Name,
					"active": p.Name == cfg.DefaultProfile,
				})
			}
			return cmdutil.WriteEnvelope(cmd, "config.profile", map[string]any{
				"active_profile": cfg.DefaultProfile,
				"profiles":       profiles,
			})
		},
	}
}

func configGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get KEY",
		Short: "Show a configuration value",
		Example: `# Read a configuration value
$ jira config get theme.name`,
		Args:              cobra.ExactArgs(1),
		Annotations:       map[string]string{"clib": "dynamic-args='configkey'"},
		ValidArgsFunction: completeConfigKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			val, ok := cfg.Get(args[0])
			if !ok {
				return fmt.Errorf("unknown config key %q", args[0])
			}
			return cmdutil.WriteEnvelope(cmd, "config.get", map[string]any{"key": args[0], "value": val})
		},
	}
}

func configSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Set a configuration value",
		Example: `# Set a configuration value
$ jira config set theme.name dracula`,
		Args:              cobra.ExactArgs(2),
		Annotations:       map[string]string{"clib": "dynamic-args='configkey,configvalue'"},
		ValidArgsFunction: completeConfigSetArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			if err := cfg.Set(args[0], args[1]); err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "config.set", map[string]any{"key": args[0], "value": args[1]})
		},
	}
}

// completeConfigKeys lists every valid config key (profile-scoped keys
// expanded for each present profile) along with its description. Falls back
// to template form when the config can't be loaded.
func completeConfigKeys(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cfg, _ := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
	keys := config.Keys(cfg)
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = k.Name + "\t" + k.Description
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeConfigSetArgs completes both positional args of `config set`:
// arg 0 is the key list (same as `get`); arg 1 is the value enum for
// closed-set keys, or no completion for freeform values.
func completeConfigSetArgs(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return completeConfigKeys(cmd, args, "")
	case 1:
		if choices := config.KeyChoices(args[0]); len(choices) > 0 {
			return choices, cobra.ShellCompDirectiveNoFileComp
		}
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
