package cmdutil

import (
	"context"
	"fmt"
	"net/url"
	"time"

	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

// RequestedProfile returns the value of the root --profile persistent flag.
func RequestedProfile(cmd *cobra.Command) string {
	return StringValue(cmd.Root().PersistentFlags(), "profile")
}

// ActiveProfile returns the env-overlaid profile selected by the --profile
// flag from cfg.
func ActiveProfile(cmd *cobra.Command, cfg *config.Config) config.Profile {
	return cfg.Profile(RequestedProfile(cmd))
}

// ProfileForCommand resolves the active profile for a command WITHOUT
// constructing a Jira client or touching any credential backend. Local
// preview and dry-run paths use this so a validation-only run cannot fail
// on a locked keyring or an offline 1Password backend. Commands that make
// live HTTP calls must still go through JiraClientForCommand.
func ProfileForCommand(cmd *cobra.Command) (config.Profile, error) {
	cfg, err := config.Load(config.WithPath(ConfigPath(cmd)))
	if err != nil {
		return config.Profile{}, err
	}
	return cfg.ResolveProfile(RequestedProfile(cmd))
}

// ProfileForEnvelope returns the profile name to record in an envelope: the
// requested profile when set, otherwise the configured default profile, or
// "default" when neither is available.
func ProfileForEnvelope(cmd *cobra.Command) string {
	if profile := RequestedProfile(cmd); profile != "" {
		return profile
	}
	cfg, err := config.Load(config.WithPath(ConfigPath(cmd)))
	if err != nil || cfg.DefaultProfile == "" {
		return "default"
	}
	return cfg.DefaultProfile
}

// CredentialStoreFor returns the credential store implementation for a secret
// backend.
func CredentialStoreFor(backend config.SecretBackend) config.CredentialStore {
	switch backend {
	case config.SecretBackendOnePassword:
		return config.OnePasswordStore{}
	case config.SecretBackendEnv:
		// The env backend reads the real environment even under the test
		// file-store override: the JIRA_TOKEN_* variable is already
		// test-controllable per process, and routing it through the file
		// store would silently change what the backend is.
		return config.EnvCredentialStore{}
	}
	if store, ok := config.FileCredentialStoreFromEnv(); ok {
		return store
	}
	return config.KeyringStore{}
}

// SecretRefFor derives the credential identity for a profile under a given
// backend. The credential is keyed by the profile's Jira site host and name;
// an unsafe profile name is rejected here rather than producing a malformed
// keyring entry.
func SecretRefFor(profile config.Profile, backend config.SecretBackend) (config.SecretRef, error) {
	scoped := profile
	scoped.SecretBackend = backend
	return config.CredentialIdentity(scoped)
}

// ExistingProfileOrDefault returns a copy of the named profile from cfg if it
// exists, or a new Profile with the given name. Used to merge a partial
// update instead of wholesale replacing the persisted profile.
func ExistingProfileOrDefault(cfg *config.Config, name string) config.Profile {
	for _, p := range cfg.Profiles {
		if p.Name == name {
			return p
		}
	}
	return config.Profile{Name: name}
}

// UpsertProfile replaces the matching profile in cfg by name, or appends it
// when no profile with that name exists.
func UpsertProfile(cfg *config.Config, profile config.Profile) {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == profile.Name {
			cfg.Profiles[i] = profile
			return
		}
	}
	cfg.Profiles = append(cfg.Profiles, profile)
}

// JiraClientForCommand builds a Jira client for the env-overlaid active
// profile selected by the --profile flag.
func JiraClientForCommand(cmd *cobra.Command) (*jira.Client, config.Profile, bool, error) {
	cfg, err := config.Load(config.WithPath(ConfigPath(cmd)))
	if err != nil {
		return nil, config.Profile{}, false, err
	}
	// An explicitly requested profile fails closed: a typo or an
	// unprovisioned name must never fall through to the ok=false path,
	// where commands degrade to empty results the caller reads as
	// success. Only the implicit default keeps that helpful degrade
	// (a fresh install with nothing configured).
	if name := RequestedProfile(cmd); name != "" {
		profile, err := cfg.ResolveProfile(name)
		if err != nil {
			return nil, config.Profile{}, false, err
		}
		if xstrings.AllEmpty(profile.BaseURL, profile.CloudID) {
			return nil, config.Profile{}, false, config.ProfileIncompleteError{Name: profile.Name}
		}
		return JiraClientForProfile(cmd, profile)
	}
	return JiraClientForProfile(cmd, ActiveProfile(cmd, cfg))
}

// JiraClientForProfile builds a Jira client targeting an explicit profile
// rather than the env-overlaid active profile. Read-modify-write commands
// that persist server data (`auth whoami --save`) use this so the live
// request and the saved record come from the same file-backed profile: a
// JIRA_PROFILE_*_BASE_URL overlay cannot redirect the request to another
// tenant whose identity would then be written into the file profile.
// Credential env sources (token/password env vars) are still honored.
func JiraClientForProfile(cmd *cobra.Command, profile config.Profile) (*jira.Client, config.Profile, bool, error) {
	return JiraClientForProfileContext(cmd.Context(), cmd, profile)
}

// JiraClientForProfileContext builds a Jira client targeting an explicit
// profile while resolving its credential with ctx. Callers already operating
// inside a bounded or cancellable task use this form so credential resolution
// observes the same lifetime as the requests that follow.
func JiraClientForProfileContext(ctx context.Context, cmd *cobra.Command, profile config.Profile) (*jira.Client, config.Profile, bool, error) {
	if profile.BaseURL == "" {
		return nil, profile, false, nil
	}
	debug := BoolValue(cmd.Root().PersistentFlags(), "debug")
	opts := []jira.Option{
		// ClientBaseURL is the site URL for a classic token and the Atlassian
		// gateway (https://api.atlassian.com/ex/jira/<cloud_id>/) for a scoped
		// token. Routing diverges here and nowhere else: the client emits
		// relative paths against this base, so every REST/Agile call and the
		// search/jql read-whitelist follow automatically.
		jira.WithBaseURL(profile.ClientBaseURL()),
		jira.WithHTTPTimeout(time.Duration(profile.TimeoutSeconds) * time.Second),
		// Single source of truth for the read-only gate. Set on the client
		// so EVERY mutation across EVERY command is automatically refused
		// without per-command boilerplate that's easy to forget.
		jira.WithReadOnly(ReadOnlyEnabled(cmd)),
		// Service-level dry-run guard: when --dry-run is set, the client
		// refuses every state-changing request. Defense in depth behind
		// the command-layer dry-run branches.
		jira.WithDryRun(dryRunRequested(cmd)),
		jira.WithDebug(debug),
		// Turn on bounded rate-limit retry. The transport caps the wait at
		// the request's context deadline, so --timeout always wins.
		jira.WithMaxRetryWait(MaxRetryWaitFor(cmd)),
		// Surface Jira's near-limit signal as a non-fatal warning. The
		// observer records into the command's rate-warn sink (carried on the
		// request context), so no command needs to inspect the response.
		jira.WithRateObserver(RecordRateNearLimit),
	}
	ref, refErr := SecretRefFor(profile, profile.SecretBackend)
	if refErr != nil {
		return nil, profile, false, refErr
	}
	secret, secretErr := config.ResolveCredential(ctx, CredentialStoreFor(profile.SecretBackend), ref)
	if secretErr != nil && !isLocalBaseURL(profile.BaseURL) {
		return nil, profile, false, fmt.Errorf("credential for profile %q is required: %w", profile.Name, secretErr)
	}
	if secret != "" {
		// Jira Cloud token auth is HTTP Basic: the account email as the
		// username and the API token as the password.
		opts = append(opts, jira.WithBasicAuth(profile.Email, secret))
	}
	client, err := jira.NewClientE(opts...)
	if err != nil {
		return nil, profile, false, err
	}
	return client, profile, true, nil
}

// isLocalBaseURL reports whether raw points at a loopback host. A missing
// credential is tolerated for local URLs so a developer fixture server needs
// no keyring entry.
func isLocalBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
