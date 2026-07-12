// Package config implements the `jira config` cobra command tree, which
// manages the local configuration file, profiles, and theme settings.
package config

import (
	"fmt"
	"strings"

	clib "github.com/gechr/clib/cli/cobra"
	xslices "github.com/gechr/x/slices"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// NewCommand returns the `config` command group: init, profile, get, set, and
// theme.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("config", "Manage configuration", "configuration")
	cmd.Long = "Read and edit the jira configuration file. `jira config init` creates it, " +
		"`jira config get` / `jira config set` read and write dotted keys, `jira config " +
		"profile` lists profiles, and `jira config theme` sets the output theme.\n\n" +
		"Config holds metadata only — never tokens (see `jira auth`). `set`, `init`, and " +
		"`theme` take `--dry-run` to validate and preview a change without writing the file."
	cmd.Example = `$ jira config init --base-url https://example.atlassian.net --email you@example.com

# Set a default project
$ jira config set profiles.default.default_project ENG

# Read a value back
$ jira config get profiles.default.default_project`
	cmd.AddCommand(configInitCommand())
	cmd.AddCommand(configProfileCommand())
	cmd.AddCommand(configGetCommand())
	cmd.AddCommand(configSetCommand())
	cmd.AddCommand(configThemeCommand())
	return cmd
}

func configThemeCommand() *cobra.Command {
	var name, path string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "Manage theme configuration",
		Long: "Show or update the theme settings in the local config file. Use it when " +
			"terminal colors need to match a built-in theme or a custom TOML file.\n\n" +
			"With no flags, the command only reports the current theme. Passing `--name` " +
			"or `--path` writes config and does not contact Jira.",
		Example: `$ jira config theme

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
				if !dryRun {
					if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
						return err
					}
				}
			}
			return cmdutil.WriteEnvelope(cmd, "config.theme", map[string]any{
				"name":    cfg.Theme.Name,
				"path":    cfg.Theme.Path,
				"changed": changed,
				"dry_run": dryRun,
			})
		},
	}
	// Theme names are self-describing (dracula, nord, catppuccin-mocha), so a
	// per-value EnumTerse would just restate the value. A short Terse keeps the
	// completion description from falling back to the flag usage instead.
	cmdutil.AddStringVar(cmd.Flags(), &name, "name", "", "Theme name", clib.FlagExtra{
		Group:       "Theme",
		Placeholder: "NAME",
		Terse:       "theme name",
		Enum:        config.ThemeNameValues,
		EnumDefault: "auto",
	})
	cmdutil.AddStringVar(cmd.Flags(), &path, "path", "", "Theme TOML path", clib.FlagExtra{
		Group:       "Theme",
		Placeholder: "PATH",
		Hint:        "file",
	})
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Validate and preview the theme change without writing the file")
	return cmd
}

func configInitCommand() *cobra.Command {
	var baseURL, email string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create initial configuration",
		Long: "Create a config file with one Jira profile and token-auth metadata. Use it " +
			"for a quick bootstrap when you want to add the credential later with " +
			"`jira auth login`.\n\n" +
			"The command requires a Jira base URL and account email. It writes config only; " +
			"no secret is stored and Jira is not contacted.",
		Example: `$ jira config init --base-url https://acme.atlassian.net --email me@example.com

# Create config under a named profile
$ jira config init --profile work --base-url https://acme.atlassian.net --email me@example.com

# Create config at an explicit path
$ jira --config ./jira.toml config init --base-url https://acme.atlassian.net --email me@example.com`,
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
			if !dryRun {
				if err := config.Save(cmdutil.ConfigPath(cmd), &cfg); err != nil {
					return err
				}
			}
			return cmdutil.WriteEnvelope(cmd, "config.init", map[string]any{
				"profile":     profile,
				"base_url":    baseURL,
				"auth_type":   string(config.AuthTypeToken),
				"stored_auth": false,
				"dry_run":     dryRun,
			})
		},
	}
	cmdutil.AddStringVar(cmd.Flags(), &baseURL, "base-url", "", "Jira base URL",
		clib.FlagExtra{Group: "Configuration", Placeholder: "URL", Terse: "Jira base URL"})
	cmdutil.AddStringVar(cmd.Flags(), &email, "email", "", "Jira account email",
		clib.FlagExtra{Group: "Configuration", Placeholder: "EMAIL", Terse: "account email"})
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Validate and preview the config without writing the file")
	return cmd
}

func missingConfigInitRequiredFlags(baseURL, email string) []string {
	var missing []string
	if xstrings.IsBlank(baseURL) {
		missing = append(missing, "base-url")
	}
	if xstrings.IsBlank(email) {
		missing = append(missing, "email")
	}
	return missing
}

func configInitRequiredFlagError(missing []string) error {
	quoted := xslices.Map(missing, func(name string) string { return "--" + name })
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
		Long: "List profiles in the local config file and mark the active default. Use it " +
			"before switching profiles or checking which names are available for `--profile`.\n\n" +
			"This command reads config only and does not verify credentials or contact Jira.",
		Example: `$ jira config profile

# Show the active marker in a parseable shape
$ jira config profile --output=json`,
		Args: cobra.NoArgs,
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
		Long: "Read one supported config key from the local config file. Use it in scripts " +
			"when you need a single value without parsing the whole file.\n\n" +
			"Profile-scoped keys include the profile name in the key path, for example " +
			"`profiles.default.base_url`.",
		Example: `$ jira config get theme.name

# Read the active profile's base URL
$ jira config get profiles.default.base_url`,
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
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Set a configuration value",
		Long: "Set one supported config key and save the file. Use it for small local edits " +
			"such as theme settings, default project values, and TUI preferences.\n\n" +
			"The new value is validated before saving. `--dry-run` runs the same validation " +
			"and reports the current and new value without writing the file. Secrets are not " +
			"written through `config set`; use `jira auth login` or `jira auth migrate` for " +
			"credentials.",
		Example: `$ jira config set theme.name dracula

# Set the default project for a profile
$ jira config set profiles.default.default_project PROJ

# Validate and preview the change without writing the file
$ jira config set theme.name dracula --dry-run --output=json`,
		Args:              cobra.ExactArgs(2),
		Annotations:       map[string]string{"clib": "dynamic-args='configkey,configvalue'"},
		ValidArgsFunction: completeConfigSetArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			// The pre-write value rides the envelope so a preview (and the
			// live write) reports what the change replaces.
			previous, _ := cfg.Get(args[0])
			if err := cfg.Set(args[0], args[1]); err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			data := map[string]any{
				"key":            args[0],
				"value":          args[1],
				"previous_value": previous,
				"dry_run":        dryRun,
			}
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "config.set", data)
			}
			if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "config.set", data)
		},
	}
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Validate and preview the change without writing the file")
	return cmd
}

// completeConfigKeys lists every valid config key (profile-scoped keys
// expanded for each present profile) along with its description. Falls back
// to template form when the config can't be loaded.
func completeConfigKeys(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cfg, _ := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
	keys := config.Keys(cfg)
	out := xslices.Map(keys, func(k config.KeyDesc) string { return k.Name + "\t" + k.Description })
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
