package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/gechr/x/shell"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func aliasCommand() *cobra.Command {
	cmd := groupCommand("alias", "Manage command aliases", "configuration")
	cmd.AddCommand(aliasDeleteCommand())
	cmd.AddCommand(aliasImportCommand())
	cmd.AddCommand(aliasListCommand())
	cmd.AddCommand(aliasSetCommand())
	return cmd
}

func aliasListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List your aliases",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			return writeEnvelope(cmd, "alias.list", cfg.Aliases)
		},
	}
}

func aliasSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set NAME EXPANSION...",
		Short: "Create a shortcut for a jira command",
		Long: `Create a shortcut for a jira command.

The stored expansion is parsed back to an argv with POSIX shell grammar.
` + "`jira alias set`" + ` quotes each argument when it writes the config,
so a round-tripped alias is always faithful. A hand-edited config alias
must follow the same grammar: an unquoted '#' starts a comment and
everything after it is dropped. Quote a literal '#' (e.g. "'#tag'") to
keep it.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if err := validateAliasName(cmd.Root(), name); err != nil {
				return err
			}
			expansion := quoteAliasExpansion(args[1:])
			cfg, err := config.LoadOrInit(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			if cfg.Aliases == nil {
				cfg.Aliases = map[string]string{}
			}
			cfg.Aliases[name] = expansion
			if err := config.Save(configPath(cmd), cfg); err != nil {
				return err
			}
			return writeEnvelope(cmd, "alias.set", map[string]any{"name": name, "expansion": expansion})
		},
	}
}

func aliasDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "delete NAME",
		Aliases:           []string{"del", "rm"},
		Short:             "Delete set aliases",
		Args:              cobra.ExactArgs(1),
		Annotations:       map[string]string{"clib": "dynamic-args='alias'"},
		ValidArgsFunction: completeAliasNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrInit(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			_, existed := cfg.Aliases[args[0]]
			delete(cfg.Aliases, args[0])
			if err := config.Save(configPath(cmd), cfg); err != nil {
				return err
			}
			return writeEnvelope(cmd, "alias.delete", map[string]any{"name": args[0], "deleted": existed})
		},
	}
}

func aliasImportCommand() *cobra.Command {
	var clobber bool
	cmd := &cobra.Command{
		Use:   "import [FILENAME|-]",
		Short: "Import aliases from a YAML file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := "-"
			if len(args) > 0 {
				filename = args[0]
			}
			aliases, err := readAliasImport(cmd, filename)
			if err != nil {
				return err
			}
			cfg, err := config.LoadOrInit(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			if cfg.Aliases == nil {
				cfg.Aliases = map[string]string{}
			}
			imported := make([]string, 0, len(aliases))
			skipped := map[string]string{}
			for _, name := range sortedAliasNames(aliases) {
				if err := validateAliasName(cmd.Root(), name); err != nil {
					skipped[name] = err.Error()
					continue
				}
				if _, exists := cfg.Aliases[name]; exists && !clobber {
					skipped[name] = "name already taken"
					continue
				}
				expansion := strings.TrimSpace(aliases[name])
				if expansion == "" {
					skipped[name] = "expansion is empty"
					continue
				}
				if !validAliasExpansion(cmd.Root(), cfg, expansion) {
					skipped[name] = "expansion does not correspond to a jira command or alias"
					continue
				}
				cfg.Aliases[name] = expansion
				imported = append(imported, name)
			}
			if err := config.Save(configPath(cmd), cfg); err != nil {
				return err
			}
			return writeEnvelope(cmd, "alias.import", map[string]any{
				"imported": len(imported),
				"aliases":  imported,
				"skipped":  skipped,
			})
		},
	}
	cmd.Flags().BoolVar(&clobber, "clobber", false, "Overwrite existing aliases of the same name")
	return cmd
}

// completeAliasNames lists every alias defined in the loaded config.
func completeAliasNames(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]string, 0, len(cfg.Aliases))
	for name := range cfg.Aliases {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, cobra.ShellCompDirectiveNoFileComp
}

func readAliasImport(cmd *cobra.Command, filename string) (map[string]string, error) {
	var b []byte
	var err error
	if filename == "-" {
		b, err = io.ReadAll(cmd.InOrStdin())
	} else {
		b, err = os.ReadFile(filename)
	}
	if err != nil {
		return nil, err
	}
	aliases := map[string]string{}
	if err := yaml.Unmarshal(b, &aliases); err != nil {
		return nil, err
	}
	return aliases, nil
}

func expandAliasArgs(root *cobra.Command, args []string) ([]string, error) {
	prefix, name, rest, ok := splitFirstCommandArg(args)
	if !ok || name == "alias" || isRootCommand(root, name) {
		return args, nil
	}
	cfg, err := config.Load(config.WithPath(configPathFromArgs(args)))
	if err != nil {
		return nil, err
	}
	expansion, ok := cfg.Aliases[name]
	if !ok {
		return args, nil
	}
	expanded, err := splitAliasExpansion(expansion)
	if err != nil {
		return nil, fmt.Errorf("alias %q: %w", name, err)
	}
	if len(expanded) == 0 {
		return nil, fmt.Errorf("alias %q has empty expansion", name)
	}
	out := make([]string, 0, len(prefix)+len(expanded)+len(rest))
	out = append(out, prefix...)
	out = append(out, expanded...)
	out = append(out, rest...)
	return out, nil
}

func splitFirstCommandArg(args []string) (prefix []string, command string, rest []string, ok bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 >= len(args) {
				return args, "", nil, false
			}
			return args[:i+1], args[i+1], args[i+2:], true
		}
		if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "--profile=") || strings.HasPrefix(arg, "--color=") {
			continue
		}
		if slices.Contains([]string{"--config", "-c", "--profile", "-p", "--color"}, arg) {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return args[:i], arg, args[i+1:], true
	}
	return args, "", nil, false
}

func configPathFromArgs(args []string) string {
	for i, arg := range args {
		if value, ok := strings.CutPrefix(arg, "--config="); ok {
			return value
		}
		if arg == "--config" || arg == "-c" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
	}
	return ""
}

func validateAliasName(root *cobra.Command, name string) error {
	if name == "" {
		return errors.New("alias name is required")
	}
	if strings.HasPrefix(name, "-") {
		return errors.New("alias name cannot start with '-'")
	}
	if isRootCommand(root, name) {
		return fmt.Errorf("alias %q conflicts with a built-in command", name)
	}
	return nil
}

func isRootCommand(root *cobra.Command, name string) bool {
	for _, cmd := range root.Commands() {
		if cmd.Name() == name || slices.Contains(cmd.Aliases, name) {
			return true
		}
	}
	return false
}

func validAliasExpansion(root *cobra.Command, cfg *config.Config, expansion string) bool {
	_, name, _, ok := splitFirstCommandArg(mustSplitAliasExpansion(expansion))
	if !ok {
		return false
	}
	return isRootCommand(root, name) || cfg.Aliases[name] != ""
}

func mustSplitAliasExpansion(expansion string) []string {
	args, err := splitAliasExpansion(expansion)
	if err != nil {
		return nil
	}
	return args
}

func sortedAliasNames(aliases map[string]string) []string {
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// quoteAliasExpansion joins argv into a single stored alias string with
// POSIX shell quoting, so splitAliasExpansion reproduces the exact argv
// on the way back out.
func quoteAliasExpansion(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, shell.Quote(arg))
	}
	return strings.Join(parts, " ")
}

// splitAliasExpansion re-splits a stored alias string into argv using
// POSIX shell grammar. This is the canonical shell splitter shared with
// the editor command parser — a hand-rolled grammar previously
// mishandled backslashes inside single quotes.
func splitAliasExpansion(value string) ([]string, error) {
	return shell.Split(value)
}
