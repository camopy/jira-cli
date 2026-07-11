package alias

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	clib "github.com/gechr/clib/cli/cobra"
	xmaps "github.com/gechr/x/maps"
	"github.com/gechr/x/shell"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/startup"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewCommand returns the `alias` command group for managing command aliases.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("alias", "Manage command aliases", "configuration")
	cmd.Long = "Manage local command shortcuts that expand before jira dispatches. `jira " +
		"alias set` stores a shortcut, `jira alias list` shows them, `jira alias delete` " +
		"removes one, and `jira alias import` merges a YAML set.\n\n" +
		"Aliases are local config and expand before command parsing, so they never contact " +
		"Jira; a shortcut cannot shadow a built-in command."
	cmd.Example = `# Alias "mine" to your assigned issues
$ jira alias set mine "issue list --assignee me"

$ jira alias list

# Remove an alias (headless callers must pass --force)
$ jira alias delete mine --force`
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
		Long: "List command aliases stored in the active config file. Use it to see the " +
			"short names that are expanded before Cobra dispatches a command.\n\n" +
			"Alias expansion is local config behavior. It happens before command parsing " +
			"and never contacts Jira.",
		Example: `$ jira alias list

# Inspect aliases as a JSON map
$ jira alias list --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "alias.list", cfg.Aliases)
		},
	}
}

func aliasSetCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "set NAME EXPANSION...",
		Short: "Create a shortcut for a jira command",
		Example: `# Alias "mine" to list issues assigned to you
$ jira alias set mine "issue list --assignee me"

# Store a multi-word expansion as a single quoted string
$ jira alias set bugs "search jql 'type = Bug AND status = Open'"

# Alias a JSON-friendly search for agents
$ jira alias set mybugs "issue list --assignee me --output=json"`,
		Long: "Store a local shortcut that expands to another jira command before Cobra " +
			"parses argv. Use it for repeated searches, profile-specific workflows, or " +
			"long flag combinations.\n\n" +
			"The stored expansion is parsed back to an argv with POSIX shell grammar. " +
			"When EXPANSION reaches `jira alias set` as a single shell-quoted string, it " +
			"is stored verbatim. When EXPANSION arrives as multiple argv tokens, each " +
			"token is quoted before storage so the round trip stays faithful.\n\n" +
			"A hand-edited config alias must follow the same grammar: an unquoted `#` " +
			"starts a comment and everything after it is dropped. Quote a literal `#`, " +
			"for example `'#tag'`, to keep it.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if err := validateAliasName(cmd.Root(), name); err != nil {
				return err
			}
			expansion := storeAliasExpansion(args[1:])
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			if cfg.Aliases == nil {
				cfg.Aliases = map[string]string{}
			}
			previous := cfg.Aliases[name]
			cfg.Aliases[name] = expansion
			data := map[string]any{"name": name, "expansion": expansion, "previous": previous, "dry_run": dryRun}
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "alias.set", data)
			}
			if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "alias.set", data)
		},
	}
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview the alias without writing the config file")
	return cmd
}

func aliasDeleteCommand() *cobra.Command {
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:     "delete NAME",
		Aliases: []string{"del", "rm"},
		Short:   "Delete an alias",
		Long: "Remove one alias from the active config file. Use it when a shortcut is " +
			"stale or conflicts with the command you now want to run.\n\n" +
			"Removing local state is a mutation: a live delete requires `--force` in " +
			"headless, agent, or `--no-input` mode, matching `jira cache clear`; an " +
			"interactive terminal proceeds without a prompt, since an alias is one line to " +
			"re-add. `--dry-run` previews the delete without writing.\n\n" +
			"Deleting a missing alias still returns structured output showing that " +
			"nothing was removed.",
		Args:              cobra.ExactArgs(1),
		Annotations:       map[string]string{"clib": "dynamic-args='alias'"},
		ValidArgsFunction: completeAliasNames,
		Example: `$ jira alias delete mine

# Report whether the alias existed
$ jira alias delete mine --output=json

# Non-interactive (agent / script) delete
$ jira alias delete mine --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Deleting local state is a mutation: headless, agent, and
			// --no-input callers must consent with --force. --dry-run never
			// mutates, so it stays open; an interactive terminal proceeds
			// without a prompt — the alias is trivially re-added.
			det := cmdutil.DetectorFromContext(cmd)
			headless := !det.IsTTY || det.Agent || cmdutil.NoInputRequested(cmd)
			if !dryRun && !force && headless {
				return cli.NewCLIInputError(cli.InputForceRequired, "alias delete requires --force in headless / agent / --no-input mode")
			}
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			_, existed := cfg.Aliases[args[0]]
			data := map[string]any{"name": args[0], "deleted": existed, "dry_run": dryRun}
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "alias.delete", data)
			}
			delete(cfg.Aliases, args[0])
			if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "alias.delete", data)
		},
	}
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview the deletion without writing the config file")
	cmdutil.AddForceFlag(cmd.Flags(), &force, "Delete without the interactive-mode guard (required in headless / agent / --no-input mode)")
	return cmd
}

func aliasImportCommand() *cobra.Command {
	var clobber, dryRun bool
	cmd := &cobra.Command{
		Use:   "import [FILENAME|-]",
		Short: "Import aliases from a YAML file",
		Long: "Read alias definitions from a YAML map and merge them into the active config " +
			"file. Use it to share a team alias set or restore aliases from a backup.\n\n" +
			"Existing aliases are kept unless `--clobber` is set. Each imported " +
			"expansion is validated against built-in commands or aliases already present " +
			"in config; invalid entries are skipped and reported.",
		Args: cobra.MaximumNArgs(1),
		Example: `$ jira alias import aliases.yaml

# Read YAML from stdin and replace existing names
$ jira alias import - --clobber

# Import into a non-default config file
$ jira --config team-jira.yaml alias import aliases.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := "-"
			if len(args) > 0 {
				filename = args[0]
			}
			aliases, err := readAliasImport(cmd, filename)
			if err != nil {
				return err
			}
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
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
			data := map[string]any{
				"imported": len(imported),
				"aliases":  imported,
				"skipped":  skipped,
				"dry_run":  dryRun,
			}
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "alias.import", data)
			}
			if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "alias.import", data)
		},
	}
	cmdutil.AddBoolVar(cmd.Flags(), &clobber, "clobber", false, "Overwrite existing aliases of the same name", clib.FlagExtra{Group: "Safety", Terse: "overwrite existing"})
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Validate and preview the merge without writing the config file")
	return cmd
}

// completeAliasNames lists every alias defined in the loaded config.
func completeAliasNames(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := xmaps.KeysNatural(cfg.Aliases)
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

// ExpandAliasArgs rewrites argv so a leading alias name is replaced by its
// configured expansion before cobra dispatches the command.
func ExpandAliasArgs(root *cobra.Command, args []string) ([]string, error) {
	prefix, name, rest, ok := startup.SplitFirstCommandArg(args)
	if !ok || name == "alias" || isRootCommand(root, name) {
		return args, nil
	}
	cfg, err := config.Load(config.WithPath(startup.GlobalsFromArgs(args).ConfigPath))
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
	_, name, _, ok := startup.SplitFirstCommandArg(mustSplitAliasExpansion(expansion))
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
	names := xmaps.KeysNatural(aliases)
	return names
}

func storeAliasExpansion(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return quoteAliasExpansion(args)
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
