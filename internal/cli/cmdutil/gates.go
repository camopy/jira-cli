package cmdutil

import (
	"os"
	"strings"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// ConfiguredEditorFor resolves the editor command for a command: the active
// profile's editor when set, otherwise the global config editor.
func ConfiguredEditorFor(cmd *cobra.Command) string {
	cfg, err := config.Load(config.WithPath(ConfigPath(cmd)))
	if err != nil {
		return ""
	}
	name := ProfileForEnvelope(cmd)
	if name != "" {
		profile := cfg.Profile(name)
		if v := strings.TrimSpace(profile.Editor); v != "" {
			return v
		}
	}
	return strings.TrimSpace(cfg.Editor)
}

// ADFModeFor resolves the ADF strict/best-effort mode for a single
// mutation invocation. mutation=true selects the mutation-submit
// default (strict) when nothing else overrides.
func ADFModeFor(cmd *cobra.Command, mutation bool) adfmode.Mode {
	flag := adfmode.FlagUnset
	if v, _ := cmd.Flags().GetBool("adf-strict"); v {
		flag |= adfmode.FlagStrict
	}
	if v, _ := cmd.Flags().GetBool("adf-best-effort"); v {
		flag |= adfmode.FlagBestEffort
	}
	env := os.Getenv("JIRA_ADF_STRICT")
	// TODO: thread profile.ADFStrict once a typed *bool field lands on
	// internal/config.Profile (currently only env + flag + default-by-path).
	var profilePtr *bool
	path := adfmode.PathRead
	if mutation {
		path = adfmode.PathMutationSubmit
	}
	mode, err := adfmode.Resolve(adfmode.Inputs{Flag: flag, Env: env, Profile: profilePtr, Path: path})
	if err != nil {
		// Resolver only errors on conflicting inputs; default to safe.
		if mutation {
			return adfmode.ModeStrict
		}
		return adfmode.ModeBestEffort
	}
	return mode
}

// ReadOnlyEnabled reports whether the active profile (or the JIRA_READ_ONLY
// env var) blocks mutations. Env wins on the OFF→ON direction so an agent
// shell can enforce read-only globally without editing config.
func ReadOnlyEnabled(cmd *cobra.Command) bool {
	if envReadOnlyEnabled() {
		return true
	}
	cfg, err := config.Load(config.WithPath(ConfigPath(cmd)))
	if err != nil {
		// On config-load failure, fail safe: treat as writable so the
		// real command surfaces the underlying error rather than masking
		// it with a read-only refusal.
		return false
	}
	return ActiveProfile(cmd, cfg).ReadOnly
}

// dryRunRequested reports whether the active command was invoked with
// --dry-run. The flag is declared per-command (not persistent), so the
// lookup tolerates its absence: commands without a --dry-run flag
// simply return false. Threading this into the Jira client lets the
// service layer refuse any mutating request as a dry-run safety net.
func dryRunRequested(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		// Command has no --dry-run flag — nothing to honor.
		return false
	}
	v, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		// The flag exists but is not a bool (a future redefinition).
		// Fail SAFE: a dry-run guard that cannot read its flag must
		// assume dry-run is ON rather than silently disabling itself.
		return true
	}
	return v
}

// envReadOnlyEnabled reports whether the JIRA_READ_ONLY env var is set to a
// truthy value.
func envReadOnlyEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("JIRA_READ_ONLY")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
