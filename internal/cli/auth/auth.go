package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2"
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/gechr/clog"
	xhttp "github.com/gechr/x/http"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/cli"
	cachereg "github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

// NewCommand returns the `auth` command group for managing Jira authentication.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("auth", "Manage Jira authentication", "configuration")
	cmd.Long = "Manage how jira authenticates to each profile's site. `jira auth login` " +
		"stores an API token in the profile's secret backend, `jira auth status` and `jira " +
		"auth whoami` report the current identity, and `jira auth migrate` moves a credential " +
		"between the keyring and 1Password.\n\n" +
		"Tokens never live in the config file — only in the OS keyring, 1Password, or a " +
		"`JIRA_TOKEN_*` env var. `jira auth logout` removes a profile's stored token."
	cmd.Example = `$ jira auth login --base-url https://example.atlassian.net --email you@example.com

# Show the active identity
$ jira auth status

# Remove the stored token for a profile
$ jira auth logout`
	cmd.AddCommand(authLoginCommand())
	cmd.AddCommand(authStatusCommand())
	cmd.AddCommand(authLogoutCommand())
	cmd.AddCommand(authSwitchCommand())
	cmd.AddCommand(authRefreshCommand())
	cmd.AddCommand(authTokenCommand())
	cmd.AddCommand(authMigrateCommand())
	cmd.AddCommand(authWhoamiCommand())
	return cmd
}

// authWhoamiCommand fetches /myself for the active profile and (optionally)
// caches the resolved accountId in the profile config so subsequent
// `--assignee me` / TUI `A` calls use the canonical Jira identifier.
func authWhoamiCommand() *cobra.Command {
	var save bool
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the active user's identity",
		Long: "Fetch `/myself` for the active profile and print the account identity Jira " +
			"returns. Use it to confirm which Atlassian account a profile is using.\n\n" +
			"With `--save`, the resolved account ID is written back to the file-backed " +
			"profile so `--assignee me` and TUI shortcuts can use Jira's canonical " +
			"identifier. `--save` refuses environment-only profile overlays to avoid " +
			"writing an account ID for the wrong tenant.",
		Example: `$ jira auth whoami

# Persist the resolved account ID to the active profile
$ jira auth whoami --save

# Check another profile without changing the active one
$ jira --profile prod auth whoami --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Without --save, fetch /myself for the env-overlaid active
			// profile (a read-only view). With --save, both the live
			// request and the persisted record must come from the same
			// file-backed profile, so resolve and Save that profile here
			// and build the client from it: a JIRA_PROFILE_*_BASE_URL
			// overlay must not redirect the request to a different tenant
			// whose account_id would then be written into the file.
			var cfg *config.Config
			var profile config.Profile
			var clientErr error
			var client *jira.Client
			var ok bool
			if save {
				var loadErr error
				cfg, loadErr = config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
				if loadErr != nil {
					return loadErr
				}
				resolved, resolveErr := cfg.ResolveProfile(cmdutil.RequestedProfile(cmd))
				if resolveErr != nil {
					if errors.Is(resolveErr, config.ErrProfileNotDefined) {
						return fmt.Errorf("validation: cannot --save profile %q: it is not defined in the config file (it exists only via a JIRA_* env overlay)", cmdutil.RequestedProfile(cmd))
					}
					return resolveErr
				}
				// A base_url env overlay would point the request — and the
				// profile's credential — at a tenant other than the
				// file-backed one. --save must operate only on a purely
				// file-backed profile, so refuse before any request.
				if envVar := profileBaseURLEnvVar(resolved.Name); os.Getenv(envVar) != "" {
					return fmt.Errorf("validation: cannot --save profile %q while %s is set: unset that environment variable so --save targets the file-backed Jira tenant", resolved.Name, envVar)
				}
				client, profile, ok, clientErr = cmdutil.JiraClientForProfile(cmd, resolved)
			} else {
				client, profile, ok, clientErr = cmdutil.JiraClientForCommand(cmd)
			}
			if clientErr != nil {
				return clientErr
			}
			if !ok {
				// The profile resolved but has no base URL — the same
				// incomplete-profile failure cmdutil.JiraClientForCommand
				// raises for a requested profile. Typed so it classifies as
				// profile_incomplete (exit 2) rather than being substring-
				// guessed as an auth failure off the "auth.whoami" text.
				return config.ProfileIncompleteError{Name: profile.Name}
			}
			var user *jira.CurrentUser
			if err := cmdutil.Spin(cmd, "auth.whoami", func(ctx context.Context) error {
				var e error
				user, _, e = cmdutil.ServicesForClient(client).User().Myself(ctx)
				return e
			}); err != nil {
				return err
			}
			data := map[string]any{
				"profile":       profile.Name,
				"account_id":    user.AccountID,
				"account_type":  user.AccountType,
				"display_name":  user.DisplayName,
				"email_address": user.EmailAddress,
				"time_zone":     user.TimeZone,
				"saved":         false,
			}
			if save {
				saved := false
				for i := range cfg.Profiles {
					if cfg.Profiles[i].Name == profile.Name {
						cfg.Profiles[i].AccountID = user.AccountID
						if cfg.Profiles[i].Email == "" && user.EmailAddress != "" {
							cfg.Profiles[i].Email = user.EmailAddress
						}
						saved = true
						break
					}
				}
				if !saved {
					return fmt.Errorf("validation: cannot --save profile %q: it is not defined in the config file (it exists only via a JIRA_* env overlay)", profile.Name)
				}
				if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
					return err
				}
				data["saved"] = true
			}
			return cmdutil.WriteEnvelope(cmd, "auth.whoami", data)
		},
	}
	cmdutil.AddBoolVar(cmd.Flags(), &save, "save", false, "Persist the resolved `account_id` (and email if blank) to the active profile", clib.FlagExtra{Group: "Configuration", Terse: "persist account id"})
	return cmd
}

func authLoginCommand() *cobra.Command {
	var profileName, baseURL, email, backend, onePasswordAccount, vault, item, credential, credentialEnv, jsonInput string
	var secretStdin, skipVerify bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Configure authentication for a profile",
		Long: "Configure a Jira profile and store its credential. The token type — classic " +
			"or scoped (granular) — is detected automatically: verification tries the site " +
			"first, and on rejection re-checks the Atlassian gateway, persisting the " +
			"discovered cloudId when the token turns out to be scoped. Nothing extra to pass.\n\n" +
			"With `--backend env` nothing is stored: the profile reads its token from the " +
			"`JIRA_TOKEN_<PROFILE>` environment variable at run time (for example " +
			"`JIRA_TOKEN_DEFAULT`), which suits hosts without an OS keyring — WSL, headless " +
			"Linux, containers — and per-process secret injectors such as `op run`. The " +
			"login verifies the variable's token when it is set and only warns when it is not.",
		Example: `# Configure a profile interactively (prompts for token)
$ jira auth login --profile-name work --base-url https://acme.atlassian.net --email me@example.com

# Headless login reading the token from an environment variable
$ jira auth login --no-input --profile-name ci --base-url https://acme.atlassian.net --email ci@example.com --credential-env JIRA_API_TOKEN

# Headless login reading the token from stdin (classic or scoped — auto-detected)
$ printf '%s' "$TOKEN" | jira auth login --no-input --profile-name work --base-url https://acme.atlassian.net --email me@example.com --secret-stdin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			noInput := cmdutil.NoInputRequested(cmd)
			// --json-input carries headless profile metadata. Combined with an
			// interactive login it would overwrite fields AFTER the user has
			// reviewed and confirmed them in the form, so the credential could
			// be verified and stored for metadata the user never saw. Reject
			// the combination: --json-input is a headless feature.
			if jsonInput != "" && !noInput {
				return fmt.Errorf("validation: --json-input requires --no-input; interactive login prompts for these fields")
			}
			if !noInput {
				if cmdutil.DetectorFromContext(cmd).Mode != cli.ModePlain && cmdutil.DetectorFromContext(cmd).Mode != cli.ModeTUI {
					return fmt.Errorf("login requires --no-input in JSON, agent, or non-TTY mode")
				}
				// Pre-fill the form from the persisted profile so a re-login
				// edits current values rather than blank fields. A flag the
				// user passed explicitly still wins. The load is deliberately
				// file-backed and non-creating: only preseed when a config
				// already exists (so aborting at the confirm step leaves no
				// file behind), and use LoadOrInit, which does NOT apply JIRA_*
				// env overlays — a transient env override must never be baked
				// into the saved profile. A load error just starts the form
				// empty.
				if _, statErr := os.Stat(cmdutil.ConfigPath(cmd)); statErr == nil {
					if existing, loadErr := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd))); loadErr == nil {
						preseedProfile := authLoginPreseedProfile(
							existing,
							cmdutil.RequestedProfile(cmd),
							profileName,
							cmd.Flags().Changed("profile-name"),
						)
						if !cmd.Flags().Changed("profile-name") && preseedProfile.Name != "" {
							profileName = preseedProfile.Name
						}
						applyLoginPreseed(
							preseedProfile, cmd.Flags().Changed,
							&baseURL, &email, &backend, &onePasswordAccount, &vault, &item,
						)
					}
				}
				// The form offers only backends that can work here; when the
				// default (keyring) is unavailable — WSL and headless Linux
				// often have no Secret Service — steer the initial selection
				// to the env backend so the select doesn't open on a choice
				// the option list no longer contains. An explicit --backend
				// or a preseeded non-keyring backend is left alone.
				if !cmd.Flags().Changed("backend") && config.SecretBackend(backend) == config.SecretBackendKeyring && !config.KeyringAvailable() {
					backend = string(config.SecretBackendEnv)
				}
				if err := promptAuthLogin(cmd, skipVerify, &profileName, &baseURL, &email, &backend, &onePasswordAccount, &vault, &item, &credential); err != nil {
					return err
				}
			}
			if jsonInput != "" {
				var input struct {
					ProfileName        string `json:"profile_name"`
					BaseURL            string `json:"base_url"`
					Email              string `json:"email"`
					Backend            string `json:"backend"`
					OnePasswordAccount string `json:"onepassword_account"`
					Vault              string `json:"vault"`
					Item               string `json:"item"`
				}
				if err := cmdutil.ReadJSONFile(jsonInput, &input); err != nil {
					return err
				}
				if input.ProfileName != "" && !cmd.Flags().Changed("profile-name") {
					profileName = input.ProfileName
				}
				if input.BaseURL != "" && !cmd.Flags().Changed("base-url") {
					baseURL = input.BaseURL
				}
				if input.Email != "" && !cmd.Flags().Changed("email") {
					email = input.Email
				}
				if input.Backend != "" && !cmd.Flags().Changed("backend") {
					backend = input.Backend
				}
				if input.OnePasswordAccount != "" && !cmd.Flags().Changed("onepassword-account") {
					onePasswordAccount = input.OnePasswordAccount
				}
				if input.Vault != "" && !cmd.Flags().Changed("vault") {
					vault = input.Vault
				}
				if input.Item != "" && !cmd.Flags().Changed("item") {
					item = input.Item
				}
			}
			// Under --no-input, refuse to silently fabricate profile
			// metadata. Require baseURL at minimum; profileName
			// defaults are reasonable but baseURL is the load-bearing
			// piece a profile needs.
			if noInput && xstrings.IsBlank(baseURL) {
				return fmt.Errorf("validation: --no-input requires --base-url (or --json-input with base_url)")
			}
			// Trim and validate the profile name on every path. The
			// interactive form already validates inline; this also covers the
			// headless --profile-name / --json-input path, and rejects a
			// namespace-unsafe name here — before the token is sent to Jira —
			// rather than late at credential-store time.
			profileName = strings.TrimSpace(profileName)
			if profileName == "" {
				profileName = "default"
			}
			if err := config.ValidateProfileName(profileName); err != nil {
				return err
			}
			// Normalise and validate the email on every path. The interactive
			// form already validates; this also covers the headless --email /
			// --json-input path, where a trailing space would otherwise be
			// stored and silently break Jira user lookups.
			email = strings.TrimSpace(email)
			if email != "" {
				if _, mErr := mail.ParseAddress(email); mErr != nil {
					return fmt.Errorf("validation: invalid account email %q: %w", email, mErr)
				}
			}
			flagBackend := config.SecretBackend(backend)
			if err := validateSecretBackend(backend); err != nil {
				return err
			}
			// The 1Password backend locates the credential by vault and item.
			// The interactive form validates both inline, but the headless
			// flag/--json-input path bypasses the form, so enforce the same
			// requirement here — before the token is verified or stored —
			// rather than failing late at store time with an obscure backend
			// error. Same validators the form binds, so the check cannot drift.
			if err := validateOnePasswordVault(flagBackend, vault); err != nil {
				return err
			}
			if err := validateOnePasswordItem(flagBackend, item); err != nil {
				return err
			}
			if secretStdin {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				// Trim only the trailing record delimiter; an explicitly
				// empty credential is rejected, not stored.
				credential, err = config.ReadSecret(string(b))
				if err != nil {
					return fmt.Errorf("validation: --secret-stdin: %w", err)
				}
			}
			if credentialEnv != "" {
				secret, err := config.ReadSecret(os.Getenv(credentialEnv))
				if err != nil {
					return fmt.Errorf("validation: credential environment variable %q: %w", credentialEnv, err)
				}
				credential = secret
			}
			// LoadOrInit seeds a placeholder profile named "default" when it
			// creates a config. This login already knows the name it is
			// configuring, and that profile is the first real one — so on a
			// freshly created config, start from no profiles and let the login
			// create exactly the profile requested, rather than leaving the
			// seed beside it as a phantom unconfigured "default".
			//
			// The existence probe must stat the RESOLVED path — an unset
			// --config means the default location, the same rule Load and Save
			// apply. Stat'ing the raw (empty) flag value declared every
			// default-path config "new" and wiped its existing profiles.
			configPath := cmdutil.ConfigPath(cmd)
			if configPath == "" {
				configPath = config.DefaultPath()
			}
			_, statErr := os.Stat(configPath)
			configExisted := statErr == nil
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			if !configExisted {
				cfg.Profiles = nil
			}
			// : merge with existing profile instead of wholesale replace.
			// Start from the persisted profile (if any) so that fields not
			// supplied to `auth login` (email, account_id, default_project,
			// read_only, editor, …) survive a partial update.
			previousProfile := cmdutil.ExistingProfileOrDefault(cfg, profileName)
			profile := previousProfile
			if cmd.Flags().Changed("profile-name") || profile.Name == "" {
				profile.Name = profileName
			}
			if baseURL != "" {
				profile.BaseURL = config.NormalizeBaseURL(baseURL)
			}
			profile.AuthType = config.AuthTypeToken
			if email != "" {
				profile.Email = email
			}
			// The interactive form's confirmed selection is as explicit as a
			// flag — the user reviewed it — so both re-point the profile;
			// otherwise the persisted backend is kept and the flag default
			// only seeds a brand-new profile.
			backendSelected := cmd.Flags().Changed("backend") || !noInput
			if backendSelected || profile.SecretBackend == "" {
				profile.SecretBackend = flagBackend
			}
			// A selected backend that differs from the profile's stored
			// backend would silently relocate a live credential to a different
			// store. Moving a secret between storing backends is `auth
			// migrate`'s job — it stages the new write and cleans up the old
			// one transactionally — so login refuses it rather than switching
			// as a side-effect. A switch to or from the env backend is exempt:
			// the env side stores nothing, so there is no secret to relocate —
			// login revokes a stale stored credential after commit instead.
			// Typed as a flag-value failure: this is bad command-line input
			// (validation, exit 3), not an authentication failure.
			if backendSelected && previousProfile.SecretBackend != "" && flagBackend != previousProfile.SecretBackend &&
				flagBackend != config.SecretBackendEnv && previousProfile.SecretBackend != config.SecretBackendEnv {
				mismatch := cli.NewCLIInputError(cli.InputFlagValueInvalid, fmt.Sprintf("profile %q stores its credential in %s; run `jira auth migrate --backend %s` to move it", profile.Name, previousProfile.SecretBackend, flagBackend))
				mismatch.Flag = "backend"
				return mismatch
			}
			if onePasswordAccount != "" {
				profile.OnePasswordAccount = onePasswordAccount
			}
			if vault != "" {
				profile.Vault = vault
			}
			if item != "" {
				profile.Item = item
			}
			if profile.RefreshInterval == 0 {
				profile.RefreshInterval = config.DefaultRefreshIntervalSeconds
			}
			if profile.TimeoutSeconds == 0 {
				profile.TimeoutSeconds = config.DefaultTimeoutSeconds
			}
			if profile.WorkdaySeconds == 0 {
				profile.WorkdaySeconds = config.DefaultWorkdaySeconds
			}
			// The env backend stores nothing — the profile's JIRA_TOKEN_*
			// variable is the credential's single source. A supplied secret
			// has nowhere to go, so reject it rather than silently dropping
			// it; verification (below) reads the variable instead. The
			// interactive form hides the token prompt for this backend.
			usingEnvBackend := profile.SecretBackend == config.SecretBackendEnv
			envTokenKey := ""
			if usingEnvBackend {
				if credential != "" || secretStdin || credentialEnv != "" {
					return fmt.Errorf("validation: the env backend reads the credential from the profile's JIRA_TOKEN_* environment variable at run time; do not supply a token to store")
				}
				envRef, refErr := cmdutil.SecretRefFor(profile, profile.SecretBackend)
				if refErr != nil {
					return refErr
				}
				envTokenKey = envRef.EnvTokenKey()
			}
			// verifyCredential is the token verification runs with: the
			// supplied secret for storing backends, the JIRA_TOKEN_* value
			// (when set) for the env backend. Empty means nothing to verify.
			verifyCredential := credential
			if usingEnvBackend {
				verifyCredential = os.Getenv(envTokenKey)
			}
			// A Jira Cloud API token is the basic-auth password, paired with
			// the account email as the username. Storing a token without an
			// email yields a credential that can never authenticate, so refuse
			// it before any network call or storage — even under --skip-verify.
			if verifyCredential != "" && xstrings.IsBlank(profile.Email) {
				return fmt.Errorf("validation: an account email is required to store a Jira Cloud API token; pass --email")
			}
			// A backend that cannot store in this build or environment fails
			// the login now — before the token is sent to Jira for
			// verification — instead of erroring at store time with the whole
			// flow already completed. This is where a WSL or headless-Linux
			// host with no Secret Service learns to use the env backend.
			if credential != "" {
				if err := ensureBackendStorable(profile.SecretBackend); err != nil {
					return err
				}
			}
			// The token is about to be sent over the network (verification) and
			// stored. Reject an unsafe base URL now — cfg.Validate runs only
			// after verification, so without this check the headless path would
			// dial a cleartext non-loopback host and leak the credential before
			// the URL is rejected. The interactive form already validates this.
			if verifyCredential != "" {
				if err := config.ValidateBaseURL(profile.BaseURL); err != nil {
					return fmt.Errorf("validation: jira base URL: %w", err)
				}
			}
			// Confirm the token actually authenticates before persisting it, so
			// a rejected token fails the login here instead of silently on first
			// real use, AND auto-detect its type while we're at it. A classic
			// token authenticates at the site; a scoped (granular) token is
			// rejected there and works only through the Atlassian gateway. So
			// verification tries the site first and, on an auth rejection,
			// discovers the cloudId and re-checks the gateway — a 2xx there means
			// the token is scoped, and the cloudId is stamped on the profile so
			// every later command routes through the gateway too. The token
			// itself carries no type marker (classic and scoped share the same
			// prefix), so this behavioral probe is the only reliable signal. The
			// resolved accountId is folded in so `--assignee me` works
			// immediately. --skip-verify opts out for offline/unreachable setup;
			// with no probe the type cannot be detected, so it stays whatever the
			// profile already carried (classic for a brand-new profile).
			var verifiedUser *jira.CurrentUser
			if verifyCredential != "" && !skipVerify {
				// .Silent() runs the task and returns its error without logging
				// a completion line — the failure is surfaced through the normal
				// command error path instead of a duplicate spinner line.
				verifyErr := clog.Spinner("Verifying Jira credentials").
					NonTTYSilent(true).
					Wait(cmd.Context(), func(ctx context.Context) error {
						user, cloudID, err := verifyAndDetectCredential(ctx, profile, verifyCredential, time.Duration(profile.TimeoutSeconds)*time.Second, cmdutil.MaxRetryWaitFor(cmd), config.GatewayBaseURL)
						if err != nil {
							return err
						}
						verifiedUser = user
						// Authoritative: empty for a classic token (clears any
						// stale cloud_id carried from a prior scoped login),
						// set for a scoped one.
						profile.CloudID = cloudID
						return nil
					}).Silent()
				if verifyErr != nil {
					// Distinguish a rejected credential (the email/token pair is
					// wrong) from a verification that could not complete (site
					// down, network, 5xx). Jira's raw 401 text ("Client must be
					// authenticated") reads cryptically, so name the real cause.
					var apiErr *jira.APIError
					// A tenant-not-found 404 means the base URL names no
					// Atlassian site at all — a typo, not a credential problem
					// and not a transient outage. Let the typed APIError
					// propagate: it already maps to jira_site_not_found with a
					// message naming the host, where the fallback below would
					// mislabel it "temporarily unavailable - retry".
					if errors.As(verifyErr, &apiErr) && apiErr.TenantNotFound {
						return verifyErr
					}
					if errors.As(verifyErr, &apiErr) && apiErr.Type == jira.ErrorTypeAuth {
						return &config.CredentialError{
							Type:        config.ErrorTypeAuth,
							ErrCode:     config.ErrorCodeCredentialRejected,
							Message:     fmt.Sprintf("invalid Atlassian account email or API token - Jira rejected the credential (HTTP %s)", strings.TrimSpace(xhttp.Status(apiErr.StatusCode))),
							HintMsg:     "check the email and that the API token is current at id.atlassian.com, or pass --skip-verify to store it without checking",
							IsRetryable: false,
						}
					}
					return &config.CredentialError{
						Type:        config.ErrorTypeAuth,
						ErrCode:     config.ErrorCodeCredentialVerifyUnavailable,
						Message:     "could not verify the credential against Jira",
						HintMsg:     "the site may be temporarily unavailable - retry, or pass --skip-verify to store it without checking",
						IsRetryable: true,
					}
				}
			}
			// Reconcile the profile's account_id with this login. A fresh
			// accountId from verification wins. Otherwise — verification
			// skipped, metadata-only, or a 2xx /myself that carried no
			// accountId — drop a carried-over account_id when the account email
			// changed: it belongs to the previous account and would mis-target
			// `--assignee me`. `auth whoami --save` repopulates it.
			freshAccountID := ""
			if verifiedUser != nil {
				freshAccountID = verifiedUser.AccountID
			}
			switch {
			case freshAccountID != "":
				profile.AccountID = freshAccountID
			case profile.Email != previousProfile.Email:
				profile.AccountID = ""
			}
			cmdutil.UpsertProfile(cfg, profile)
			cfg.DefaultProfile = profileName
			if err := cfg.Validate(); err != nil {
				return err
			}
			// Persist the profile, and — when a credential was supplied — the
			// credential, as one transaction: the credential is staged into
			// the backend, the config is saved, and a save failure rolls the
			// credential write back so a failed login never leaves an orphaned
			// secret in the keyring or 1Password.
			saveConfig := func() error { return config.Save(cmdutil.ConfigPath(cmd), cfg) }
			if credential != "" {
				ref, refErr := cmdutil.SecretRefFor(profile, profile.SecretBackend)
				if refErr != nil {
					return refErr
				}
				if err := config.StoreCredentialTransactionally(cmd.Context(), cmdutil.CredentialStoreFor(profile.SecretBackend), ref, credential, saveConfig); err != nil {
					return err
				}
			} else if err := saveConfig(); err != nil {
				return err
			}
			// An env-backed profile with the variable unset saved fine — the
			// metadata is valid — but no command can authenticate until the
			// variable exists, so say so now rather than at first use.
			if usingEnvBackend && verifyCredential == "" {
				cmdutil.RecordCredentialWarnings(cmd, []string{
					envTokenKey + " is not set — export it (or inject it via your secret manager, e.g. `op run`) so commands can authenticate",
				})
			}
			// When this login re-pointed an existing profile at a different
			// credential identity (a new site, a backend switch, a different
			// 1Password account/vault/item), the credential under the OLD
			// identity is now stale — revoke it so no live secret lingers in
			// the old keyring entry / old 1Password item. The new credential
			// and config save have already committed, so a cleanup failure is
			// surfaced as a note rather than failing the login.
			if note := revokeOldCredentialOnRelogin(cmd, previousProfile, profile); note != "" {
				cmdutil.RecordCredentialWarnings(cmd, []string{note})
			}
			// Warm the boards cache once on a verified login so the first
			// `--board`/completion use is served from disk. Best-effort: the
			// login has already committed, so a warm failure is a note, never a
			// login failure. Skipped on --skip-verify (offline/unreachable
			// setup) where a network fetch would contradict the intent.
			boardsCached := -1
			if verifiedUser != nil {
				count, warmNote := warmBoardsCache(cmd, profile)
				boardsCached = count
				if warmNote != "" {
					cmdutil.RecordCredentialWarnings(cmd, []string{warmNote})
				}
			}
			// On a human terminal, narrate the scoped-token flow and confirm
			// who the verified token belongs to, instead of a silent return.
			// Machine modes consume the same facts from the envelope below.
			if verifiedUser != nil {
				if mode := cmdutil.DetectorFromContext(cmd).Mode; mode == cli.ModePlain || mode == cli.ModeTUI {
					// Scoped tokens were auto-detected via the gateway; say so
					// and surface the discovered cloud ID, so the user sees WHAT
					// kind of token this is and WHY a cloud ID appeared rather
					// than an unexplained id. Phrasing comes from the
					// operation-verb registry so it matches the debug flow line.
					if profile.CloudID != "" {
						clog.Info().Parts(clog.PartMessage).Msg("Scoped token — " + cli.VerbFor("auth.login.discover").Pastf() + " id=" + profile.CloudID)
					}
					name := cmdutil.FirstNonEmpty(verifiedUser.DisplayName, verifiedUser.EmailAddress, profile.Email)
					msg := "✓ Logged in as " + name
					if boardsCached >= 0 {
						msg += fmt.Sprintf(" — cached %d board(s)", boardsCached)
					}
					clog.Info().Parts(clog.PartMessage).Msg(msg)
				}
			}
			data := map[string]any{
				"profile":             profileName,
				"auth_type":           string(config.AuthTypeToken),
				"token_type":          tokenType(profile),
				"scoped":              profile.Scoped(),
				"secret_backend":      string(profile.SecretBackend),
				"onepassword_account": onePasswordAccount,
				"stored_secret":       credential != "",
				"verified":            verifiedUser != nil,
				"skip_verify":         skipVerify,
			}
			if usingEnvBackend {
				data["credential_env"] = envTokenKey
			}
			if profile.CloudID != "" {
				data["cloud_id"] = profile.CloudID
			}
			if verifiedUser != nil {
				data["account_id"] = verifiedUser.AccountID
				data["display_name"] = verifiedUser.DisplayName
			}
			if boardsCached >= 0 {
				data["boards_cached"] = boardsCached
			}
			return cmdutil.WriteEnvelope(cmd, "auth.login", data)
		},
	}
	cmdutil.AddStringVar(cmd.Flags(), &profileName, "profile-name", "default", "Profile name to configure", clib.FlagExtra{Group: "Configuration", Placeholder: "NAME", Complete: "predictor=profile"})
	cmdutil.AddStringVar(cmd.Flags(), &baseURL, "base-url", "", "Jira base URL", clib.FlagExtra{Group: "Configuration", Placeholder: "URL", Terse: "site URL"})
	cmdutil.AddStringVar(cmd.Flags(), &email, "email", "", "Jira Cloud account email", clib.FlagExtra{Group: "Configuration", Placeholder: "EMAIL", Terse: "account email"})
	cmdutil.AddStringVar(cmd.Flags(), &backend, "backend", string(config.SecretBackendKeyring), "Secret backend for the credential", clib.FlagExtra{Placeholder: "BACKEND", Terse: "secret backend", Enum: []string{"keyring", "1password", "env"}, EnumTerse: []string{"OS keychain", "1Password CLI", "JIRA_TOKEN_* env var"}, EnumDefault: "keyring"})
	cmdutil.AddStringVar(cmd.Flags(), &onePasswordAccount, "onepassword-account", "", "1Password desktop app account name", clib.FlagExtra{Group: "1Password", Placeholder: "ACCOUNT"})
	cmdutil.AddStringVar(cmd.Flags(), &vault, "vault", "", "1Password vault name", clib.FlagExtra{Group: "1Password", Placeholder: "VAULT"})
	cmdutil.AddStringVar(cmd.Flags(), &item, "item", "", "1Password item name", clib.FlagExtra{Group: "1Password", Placeholder: "NAME"})
	cmdutil.AddBoolVar(cmd.Flags(), &secretStdin, "secret-stdin", false, "Read credential from stdin", clib.FlagExtra{Group: "Input", Terse: "read token from stdin"})
	cmdutil.AddBoolVar(cmd.Flags(), &skipVerify, "skip-verify", false, "Store the credential without verifying it against `/myself` first", clib.FlagExtra{Group: "Connection", Terse: "skip token check"})
	cmdutil.AddStringVar(cmd.Flags(), &credentialEnv, "credential-env", "", "Read the credential from a named environment variable [example: JIRA_API_TOKEN]", clib.FlagExtra{Group: "Input", Placeholder: "VAR"})
	cmdutil.AddFileFlag(cmd.Flags(), &jsonInput, "json-input", "", "Read auth profile metadata from JSON file", "Input", "FILE")
	// --secret-stdin and --credential-env both supply the credential. Passing
	// both is a syntactic conflict: one would silently win by processing
	// order, so reject it in Cobra validation before any source is read.
	// --json-input carries only profile metadata, never the credential, so it
	// is not part of this group.
	cmd.MarkFlagsMutuallyExclusive("secret-stdin", "credential-env")
	return cmd
}

// tokenType labels a profile's API-token flavor for envelopes and status
// output: "scoped" when it routes through the Atlassian gateway via a cloudId,
// "classic" otherwise. Both use HTTP Basic auth — the distinction is the base
// URL the token is accepted at.
func tokenType(profile config.Profile) string {
	if profile.Scoped() {
		return "scoped"
	}
	return "classic"
}

// warmBoardsCache primes the per-profile boards cache after a verified login.
// It returns the cached board count, or -1 with a note when the warm could not
// run or failed — the caller surfaces the note as a warning rather than failing
// the (already committed) login. The freshness window stamped on the warmed
// file is the registry's boards TTL, so login-warmed and `cache boards` /
// `boards list` reads agree on freshness.
func warmBoardsCache(cmd *cobra.Command, profile config.Profile) (int, string) {
	client, _, ok, err := cmdutil.JiraClientForProfile(cmd, profile)
	if err != nil || !ok || client == nil {
		return -1, ""
	}
	file, _, _, err := cmdutil.PrimeAndCacheBoards(cmd.Context(), cmdutil.CacheKeyForProfile(cmd, profile), client, cachereg.TTLMinutesFor("boards"), false)
	if err != nil {
		return -1, "boards cache not warmed (" + err.Error() + ") — run `jira cache boards --refresh` when ready"
	}
	return len(file.Items), ""
}

type authLoginQuestionKind string

const (
	authLoginQuestionInput  authLoginQuestionKind = "input"
	authLoginQuestionSelect authLoginQuestionKind = "select"
)

type authLoginOption struct {
	Label       string
	Value       string
	Description string
}

type authLoginQuestion struct {
	ID          string
	Kind        authLoginQuestionKind
	Title       string
	Description string
	Required    bool
	Secret      bool
	Options     []authLoginOption
}

func authLoginQuestions() []authLoginQuestion {
	return []authLoginQuestion{
		{
			ID:          "profile_name",
			Kind:        authLoginQuestionInput,
			Title:       "Profile name",
			Description: "Short local name for this Jira account, for example work or personal.",
			Required:    true,
		},
		{
			ID:          "base_url",
			Kind:        authLoginQuestionInput,
			Title:       "Jira site",
			Description: `Your Atlassian site name, e.g. "acme" for acme.atlassian.net. A full https:// URL also works.`,
			Required:    true,
		},
		{
			ID:          "account",
			Kind:        authLoginQuestionInput,
			Title:       "Atlassian account email",
			Description: "The Atlassian account email the API token belongs to; sent as the basic-auth username.",
			Required:    true,
		},
		{
			ID:          "secret_backend",
			Kind:        authLoginQuestionSelect,
			Title:       "Secret backend",
			Description: "Choose where jira stores the credential. Config TOML only stores metadata.",
			Required:    true,
			Options: []authLoginOption{
				{Label: "System keychain", Value: string(config.SecretBackendKeyring), Description: "macOS Keychain, Windows Credential Manager, or Linux Secret Service."},
				{Label: "1Password", Value: string(config.SecretBackendOnePassword), Description: "Store the credential in a 1Password vault using the Go SDK when configured."},
				{Label: "Environment variable", Value: string(config.SecretBackendEnv), Description: "Read the token from the profile's JIRA_TOKEN_* variable at run time; nothing is stored."},
			},
		},
		{
			ID:          "credential",
			Kind:        authLoginQuestionInput,
			Title:       "API token",
			Description: "Atlassian API token to store in the selected secret backend.",
			Required:    true,
			Secret:      true,
		},
		{
			ID:          "onepassword_account",
			Kind:        authLoginQuestionInput,
			Title:       "1Password account",
			Description: "Desktop app account name for SDK auth. Leave blank to use OP_SERVICE_ACCOUNT_TOKEN.",
		},
		{
			ID:          "vault",
			Kind:        authLoginQuestionInput,
			Title:       "1Password vault",
			Description: "Vault name used when the 1Password backend is selected.",
		},
		{
			ID:          "item",
			Kind:        authLoginQuestionInput,
			Title:       "1Password item",
			Description: "Item title for this Jira profile in the selected 1Password vault.",
		},
	}
}

func promptAuthLogin(cmd *cobra.Command, skipVerify bool, profileName, baseURL, email, backend, onePasswordAccount, vault, item, credential *string) error {
	var confirmed bool
	// The resolved default is shown only as a placeholder, not pre-filled into
	// the editable field (authLoginForm starts it empty); a blank entry falls
	// back to this hint after the form.
	nameHint := strings.TrimSpace(*profileName)
	// The profile-name field is bound to a caller-owned value that starts empty
	// (so a typed name is never appended to a pre-filled default); the typed
	// result is resolved back into *profileName after the form.
	var nameField string
	form := authLoginForm(skipVerify, &nameField, nameHint, baseURL, email, backend, onePasswordAccount, vault, item, credential, &confirmed).
		WithInput(cmd.InOrStdin()).
		WithOutput(cmd.ErrOrStderr())
	if err := form.RunWithContext(cmd.Context()); err != nil {
		// Return a typed PromptError so MapError classifies the outcome
		// via errors.As — never let "aborted"/"canceled" reach the
		// substring classifier, where it would be misread as an auth
		// failure.
		switch {
		case errors.Is(err, huh.ErrUserAborted):
			return cli.NewPromptError(cli.PromptAborted, "auth login", err)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return cli.NewPromptError(cli.PromptCanceled, "auth login", err)
		default:
			return err
		}
	}
	// The form completed, but the user declined at the review step: treat a
	// declined confirmation as an abort so nothing is stored.
	if !confirmed {
		return cli.NewPromptError(cli.PromptAborted, "auth login", errors.New("login not confirmed"))
	}
	// nameField is intentionally omitted from the trim: resolveProfileName
	// already trims the typed value (or falls back to the trimmed hint).
	trimAuthLoginValues(baseURL, email, backend, onePasswordAccount, vault, item, credential)
	*profileName = resolveProfileName(nameField, nameHint)
	return nil
}

// resolveProfileName picks the effective profile name from the interactive
// form: a typed value (trimmed) wins; a blank field falls back to the resolved
// default that was shown as the placeholder hint.
func resolveProfileName(typed, hint string) string {
	if t := strings.TrimSpace(typed); t != "" {
		return t
	}
	return hint
}

// profileNameInput builds the interactive profile-name field. The resolved
// default is shown only as a placeholder, never as the editable value: huh
// appends keystrokes to a pre-filled value, so a placeholder keeps a typed
// name from being mangled onto the default.
func profileNameInput(value *string, hint string) *huh.Input {
	return huh.NewInput().
		Title("Profile name").
		Description("Short local name for this Jira account, for example work or personal.").
		Placeholder(hint).
		Value(value).
		Validate(func(v string) error {
			// A blank field falls back to the placeholder default, which is
			// already a safe name; only a typed value needs checking.
			if v = strings.TrimSpace(v); v == "" {
				return nil
			}
			return config.ValidateProfileName(v)
		})
}

func authLoginForm(skipVerify bool, nameField *string, profileNameHint string, baseURL, email, backend, onePasswordAccount, vault, item, credential *string, confirmed *bool) *huh.Form {
	// nameField is a caller-owned value that promptAuthLogin starts empty, so the
	// profile-name input begins blank with the resolved default shown only as a
	// placeholder: huh appends keystrokes to a pre-filled value, so a pre-filled
	// name would mangle a typed one and could overwrite the wrong profile.
	// promptAuthLogin resolves the typed value (or the hint, when blank) into the
	// real profile name after the form — the builder binds nameField as-is and
	// never mutates a caller argument as a side effect.
	confirmDescription := "The token is verified against Jira before it is saved."
	if skipVerify {
		confirmDescription = "The credential is stored without verification (--skip-verify)."
	}
	return huh.NewForm(
		huh.NewGroup(
			profileNameInput(nameField, profileNameHint),
			huh.NewInput().
				Title("Jira site").
				Description(`Your Atlassian site name, e.g. "acme" for acme.atlassian.net. A full https:// URL also works.`).
				Value(baseURL).
				Validate(func(value string) error {
					value = strings.TrimSpace(value)
					if value == "" {
						return errors.New("jira base URL is required")
					}
					return config.ValidateBaseURL(config.NormalizeBaseURL(value))
				}),
			huh.NewInput().
				Title("Atlassian account email").
				Description("The Atlassian account email the API token belongs to; sent as the basic-auth username.").
				Value(email).
				Validate(validateEmailField),
		).Title("Jira profile").Description("Configure the Jira instance and account identity."),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Secret backend").
				Description("Choose where jira stores the credential. Config TOML only stores metadata.").
				Options(authLoginHuhOptions(availableAuthLoginBackendOptions(authLoginQuestionByID(authLoginQuestions(), "secret_backend").Options))...).
				Value(backend).
				Validate(validateSecretBackend),
		).Title("Credential storage").Description("Secrets are written to the selected backend, not to config.toml."),
		// The token prompt is its own group so it can disappear for the env
		// backend, which stores nothing — the profile's JIRA_TOKEN_* variable
		// is the credential's source and is read at verification time instead.
		huh.NewGroup(
			huh.NewInput().
				Title("API token").
				Description("Atlassian API token to store in the selected secret backend.").
				EchoMode(huh.EchoModePassword).
				Value(credential).
				Validate(requiredString("API token is required")),
		).Title("API token").Description("Entered once, verified against Jira, then stored in the selected backend.").WithHideFunc(func() bool {
			return config.SecretBackend(*backend) == config.SecretBackendEnv
		}),
		huh.NewGroup(
			huh.NewInput().
				Title("1Password account").
				Description("Desktop app account name for SDK auth. Leave blank to use OP_SERVICE_ACCOUNT_TOKEN.").
				Value(onePasswordAccount),
			huh.NewInput().
				Title("1Password vault").
				Description("Vault name used when the 1Password backend is selected.").
				Value(vault).
				Validate(func(value string) error {
					return validateOnePasswordVault(config.SecretBackend(*backend), value)
				}),
			huh.NewInput().
				Title("1Password item").
				Description("Item title for this Jira profile in the selected 1Password vault.").
				Value(item).
				Validate(func(value string) error {
					return validateOnePasswordItem(config.SecretBackend(*backend), value)
				}),
		).Title("1Password").Description("Only used when the 1Password backend is selected.").WithHideFunc(func() bool {
			return config.SecretBackend(*backend) != config.SecretBackendOnePassword
		}),
		huh.NewGroup(
			// The review note recomputes from the live form values: binding it
			// to the field pointers makes huh re-render the summary whenever any
			// of them change, so the user confirms exactly what will be stored.
			huh.NewNote().
				Title("Review").
				DescriptionFunc(func() string {
					return loginReviewSummary(resolveProfileName(*nameField, profileNameHint), *baseURL, *email, *backend, *onePasswordAccount, *vault, *item)
				}, []*string{nameField, baseURL, email, backend, onePasswordAccount, vault, item}),
			huh.NewConfirm().
				Title("Store this credential?").
				Description(confirmDescription).
				Affirmative("Store").
				Negative("Cancel").
				Value(confirmed),
		).Title("Confirm"),
	)
}

func authLoginQuestionByID(questions []authLoginQuestion, id string) authLoginQuestion {
	for _, question := range questions {
		if question.ID == id {
			return question
		}
	}
	return authLoginQuestion{}
}

func authLoginHuhOptions(options []authLoginOption) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		out = append(out, huh.NewOption(option.Label+" - "+option.Description, option.Value))
	}
	return out
}

func requiredString(message string) func(string) error {
	return func(value string) error {
		if xstrings.IsBlank(value) {
			return errors.New(message)
		}
		return nil
	}
}

// validateEmailField requires a non-empty, well-formed email address. Trimming
// before parsing catches a trailing space that would otherwise be stored and
// silently break Jira user lookups.
func validateEmailField(value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return errors.New("account email is required")
	}
	if _, err := mail.ParseAddress(v); err != nil {
		return errors.New("enter a valid email address")
	}
	return nil
}

func validateSecretBackend(value string) error {
	switch config.SecretBackend(value) {
	case config.SecretBackendKeyring, config.SecretBackendOnePassword, config.SecretBackendEnv:
		return nil
	default:
		return fmt.Errorf("unsupported secret backend %q", value)
	}
}

// ensureBackendStorable fails when the selected backend cannot store a
// credential in this build or environment: 1Password in a no-CGO release
// build, the keyring on a host with no Secret Service (WSL, headless Linux).
// Login and migrate call it before a token is verified or any secret is
// read, so the failure is early, typed, and names the way out (the env
// backend) instead of surfacing as a raw D-Bus or SDK error at store time.
// The env backend always passes — it stores nothing.
func ensureBackendStorable(backend config.SecretBackend) error {
	switch backend {
	case config.SecretBackendOnePassword:
		if !config.OnePasswordSupported() {
			return config.OnePasswordUnsupportedBuildError()
		}
	case config.SecretBackendKeyring:
		if !config.KeyringAvailable() {
			return config.KeyringUnavailableError(nil)
		}
	}
	return nil
}

// availableAuthLoginBackendOptions filters the backend choices down to the
// ones that can actually work here: the keyring is dropped when no OS secret
// service answers (WSL, headless Linux), 1Password when this build compiled
// the SDK out. Offering a backend the login would refuse at store time wastes
// a fully filled form. The env backend always remains — it reads a variable.
func availableAuthLoginBackendOptions(options []authLoginOption) []authLoginOption {
	out := make([]authLoginOption, 0, len(options))
	for _, option := range options {
		switch config.SecretBackend(option.Value) {
		case config.SecretBackendKeyring:
			if !config.KeyringAvailable() {
				continue
			}
		case config.SecretBackendOnePassword:
			if !config.OnePasswordSupported() {
				continue
			}
		}
		out = append(out, option)
	}
	return out
}

// validateOnePasswordVault and validateOnePasswordItem require their locator
// when the 1Password backend is selected: together they form the secret
// reference that points at the credential, so neither can be blank. Both are
// no-ops for other backends. The interactive form binds these and the headless
// command body calls them, so the two input paths share one rule.
func validateOnePasswordVault(backend config.SecretBackend, value string) error {
	if backend == config.SecretBackendOnePassword && xstrings.IsBlank(value) {
		return errors.New("1Password backend requires a vault")
	}
	return nil
}

func validateOnePasswordItem(backend config.SecretBackend, value string) error {
	if backend == config.SecretBackendOnePassword && xstrings.IsBlank(value) {
		return errors.New("1Password backend requires an item")
	}
	return nil
}

func trimAuthLoginValues(values ...*string) {
	for _, value := range values {
		*value = strings.TrimSpace(*value)
	}
}

func authLoginPreseedProfile(cfg *config.Config, requestedProfile, profileName string, profileNameChanged bool) config.Profile {
	name := profileName
	if !profileNameChanged {
		switch {
		case !xstrings.IsBlank(requestedProfile):
			name = strings.TrimSpace(requestedProfile)
		case !xstrings.IsBlank(cfg.DefaultProfile):
			name = strings.TrimSpace(cfg.DefaultProfile)
		case xstrings.IsBlank(name):
			name = "default"
		}
	}
	return cmdutil.ExistingProfileOrDefault(cfg, name)
}

// applyLoginPreseed pre-fills the interactive form fields from a persisted
// profile so a re-login shows current values. A field whose flag the user
// passed explicitly (reported by changed) is left untouched, and an empty
// persisted value never overwrites a target. The API token is never
// preseeded — it is not stored retrievably and must be re-entered.
func applyLoginPreseed(profile config.Profile, changed func(string) bool, baseURL, email, backend, onePasswordAccount, vault, item *string) {
	preseed := func(flag string, target *string, value string) {
		if value != "" && !changed(flag) {
			*target = value
		}
	}
	preseed("base-url", baseURL, profile.BaseURL)
	preseed("email", email, profile.Email)
	preseed("backend", backend, string(profile.SecretBackend))
	preseed("onepassword-account", onePasswordAccount, profile.OnePasswordAccount)
	preseed("vault", vault, profile.Vault)
	preseed("item", item, profile.Item)
}

// loginReviewSummary renders the profile metadata the login will store, for
// the form's review step. It deliberately omits the API token: the summary
// is shown on screen and the secret must never appear there.
func loginReviewSummary(profileName, baseURL, email, backend, onePasswordAccount, vault, item string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Profile:  %s\n", profileName)
	fmt.Fprintf(&b, "Site:     %s\n", baseURL)
	fmt.Fprintf(&b, "Email:    %s\n", email)
	fmt.Fprintf(&b, "Backend:  %s", backend)
	if config.SecretBackend(backend) == config.SecretBackendEnv {
		// Name the exact variable so the user leaves the form knowing what
		// to export; the token itself is never shown here.
		fmt.Fprintf(&b, "\nToken:    read from %s at run time (not stored)", config.SecretRef{Profile: profileName}.EnvTokenKey())
	}
	if config.SecretBackend(backend) == config.SecretBackendOnePassword {
		// Only render coordinates that are filled: mid-form the user may have
		// selected 1Password before entering them, and blank "Vault:" / "Item:"
		// labels in the confirmation screen are noise.
		if onePasswordAccount != "" {
			fmt.Fprintf(&b, "\nAccount:  %s", onePasswordAccount)
		}
		if vault != "" {
			fmt.Fprintf(&b, "\nVault:    %s", vault)
		}
		if item != "" {
			fmt.Fprintf(&b, "\nItem:     %s", item)
		}
	}
	return b.String()
}

func authStatusCommand() *cobra.Command {
	var noProbe bool
	var projectKey string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long: "Reports credential resolution and (by default) probes the live Jira API " +
			"to surface how the token actually behaves end-to-end. Pass `--no-probe` " +
			"to skip remote calls and run only the local credential check.",
		Example: `# Includes live Jira probes by default
$ jira auth status

# Run only the local credential check
$ jira auth status --no-probe

# Probe permissions in the context of a specific project
$ jira auth status --project PROJ

# Machine-readable health check
$ jira auth status --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			// An explicit --profile scopes the report to that profile and
			// fails closed when it does not resolve; silently reporting the
			// default set would hide exactly the misconfiguration a
			// preflight comes here to observe.
			checked := cfg.Profiles
			activeProfile := cfg.DefaultProfile
			if name := cmdutil.RequestedProfile(cmd); name != "" {
				requested, err := cfg.ResolveProfile(name)
				if err != nil {
					return err
				}
				checked = []config.Profile{requested}
				activeProfile = requested.Name
			}
			profiles := make([]map[string]any, 0, len(checked))
			for _, profile := range checked {
				entry := map[string]any{"profile": profile.Name, "token_type": tokenType(profile)}
				if profile.CloudID != "" {
					entry["cloud_id"] = profile.CloudID
				}
				ref, refErr := cmdutil.SecretRefFor(profile, profile.SecretBackend)
				if refErr != nil {
					entry["valid"] = false
					entry["error"] = refErr.Error()
					profiles = append(profiles, entry)
					continue
				}
				cred := config.CredentialStatus(
					cmd.Context(),
					cmdutil.CredentialStoreFor(profile.SecretBackend),
					ref,
				)
				entry["valid"] = cred.Valid
				entry["source"] = cred.Source
				entry["redacted"] = cred.Redacted
				if cred.Error != "" {
					entry["error"] = cred.Error
				}
				if !noProbe && cred.Valid && profile.BaseURL != "" {
					var remote map[string]any
					// One spinner around the whole remote probe — Myself plus
					// MyPermissions are several requests but one "checking" step.
					if err := cmdutil.Spin(cmd, "auth.status", func(ctx context.Context) error {
						remote = probeRemoteAuth(ctx, cmd, profile, projectKey)
						return nil
					}); err != nil {
						return err
					}
					entry["remote"] = remote
					if !remoteAuthValid(remote) {
						entry["valid"] = false
					}
				}
				profiles = append(profiles, entry)
			}
			data := map[string]any{
				"active_profile": activeProfile,
				"profiles":       profiles,
			}
			statusErrors := authStatusErrors(profiles)
			if len(statusErrors) > 0 {
				if err := cmdutil.WriteEnvelopeWithErrors(cmd, "auth.status", data, statusErrors); err != nil {
					return err
				}
				worst := statusErrors[0]
				for _, e := range statusErrors[1:] {
					if cli.ExitCode(e) > cli.ExitCode(worst) {
						worst = e
					}
				}
				msg := fmt.Sprintf("auth status found %d unhealthy profile(s)", len(statusErrors))
				return cmdutil.EnvelopeWritten(cli.NewCodedError(cli.AggregateCode(worst), msg))
			}
			return cmdutil.WriteEnvelope(cmd, "auth.status", data)
		},
	}
	cmdutil.AddBoolVar(cmd.Flags(), &noProbe, "no-probe", false, "Skip the remote `/myself` + `/mypermissions` check", clib.FlagExtra{Group: "Validation", Terse: "local check only"})
	cmdutil.AddStringVar(cmd.Flags(), &projectKey, "project", "", "Probe permissions in the context of this project", clib.FlagExtra{Group: "Validation", Placeholder: "KEY", Complete: "predictor=cacheproject"})
	return cmd
}

// probeRemoteAuth makes lightweight HTTP calls to surface how the token
// actually behaves, distinguishing "credential resolves locally" from
// "credential is accepted by Jira and grants the permissions the CLI
// needs". Returns a map suitable for inclusion in the auth.status
// envelope. Never errors — every failure becomes a structured field so
// the user can see what went wrong without rerunning anything.
func probeRemoteAuth(ctx context.Context, cmd *cobra.Command, profile config.Profile, projectKey string) map[string]any {
	out := map[string]any{"site": profile.BaseURL}

	// Route the probe through the same client constructor normal commands
	// use, so the per-profile request timeout and an mTLS client certificate
	// both apply to the probe's live calls rather than silently defaulting.
	// JiraClientForProfile only builds the client; the probe ctx is applied to
	// the live Myself / MyPermissions calls below, not to construction.
	client, _, ok, err := cmdutil.JiraClientForProfile(cmd, profile) //nolint:contextcheck // constructor takes no request context; ctx drives the probe calls below
	if err != nil {
		out["error"] = config.SanitizeCredentialError(err)
		return out
	}
	if !ok || client == nil {
		out["error"] = "profile has no base URL to probe"
		return out
	}
	user := cmdutil.ServicesForClient(client).User()

	// /myself reveals scope-level auth issues (granular tokens missing
	// read:user/group/avatar/application-role 4-scope union).
	myselfOut := map[string]any{}
	if me, resp, err := user.Myself(ctx); err != nil {
		myselfOut["ok"] = false
		myselfOut["status"] = httpStatusOf(resp)
		myselfOut["error"] = err.Error()
		if httpStatusOf(resp) == 401 {
			myselfOut["hint"] = "token can't read /myself — for granular tokens, add read:user:jira, read:application-role:jira, read:group:jira, read:avatar:jira (Atlassian requires the union)"
		}
		// A tenant-not-found 404 (Atlassian's Atl-Missing-Tcs signal) means
		// the base URL names no site at all — say "fix the site name" instead
		// of leaving the generic "/myself unreachable" reading as an outage.
		var apiErr *jira.APIError
		if errors.As(err, &apiErr) && apiErr.TenantNotFound {
			myselfOut["hint"] = "no Atlassian site exists at this base_url — check the site name and update it with `jira config set profiles.<name>.base_url`"
		}
	} else {
		myselfOut["ok"] = true
		myselfOut["account_id"] = me.AccountID
		myselfOut["email"] = me.EmailAddress
	}
	out["myself"] = myselfOut

	// /mypermissions surfaces resource-level access (the "scoped token
	// without resource grants" failure mode).
	keys := []string{
		"BROWSE_PROJECTS", "CREATE_ISSUES", "EDIT_ISSUES", "DELETE_ISSUES",
		"ADD_COMMENTS", "WORK_ON_ISSUES", "TRANSITION_ISSUES", "LINK_ISSUES",
	}
	permsOut := map[string]any{}
	if pr, resp, err := user.MyPermissions(ctx, projectKey, keys); err != nil {
		permsOut["ok"] = false
		permsOut["status"] = httpStatusOf(resp)
		permsOut["error"] = err.Error()
	} else {
		grants := map[string]bool{}
		anyGranted := false
		for _, k := range keys {
			have := pr.Permissions[k].HavePermission
			grants[k] = have
			if have {
				anyGranted = true
			}
		}
		permsOut["ok"] = anyGranted
		permsOut["grants"] = grants
		if !anyGranted {
			permsOut["hint"] = "token authenticates but has zero CLI-relevant permissions — check the Resources tab of your scoped token at id.atlassian.com (JCT/SAM1 etc. must be selected)"
		}
	}
	out["permissions"] = permsOut

	return out
}

func remoteAuthValid(remote map[string]any) bool {
	if remote == nil {
		return true
	}
	if errText, _ := remote["error"].(string); !xstrings.IsBlank(errText) {
		return false
	}
	for _, key := range []string{"myself", "permissions"} {
		if probe, _ := remote[key].(map[string]any); probe != nil {
			if ok, _ := probe["ok"].(bool); !ok {
				return false
			}
		}
	}
	return true
}

func authStatusErrors(profiles []map[string]any) []cli.Error {
	out := make([]cli.Error, 0)
	for _, profile := range profiles {
		if valid, _ := profile["valid"].(bool); valid {
			continue
		}
		name, _ := profile["profile"].(string)
		errMsg, _ := profile["error"].(string)
		errorMessages := make([]string, 0, 2)
		if !xstrings.IsBlank(errMsg) {
			errorMessages = append(errorMessages, errMsg)
		}
		msg := "profile " + name + " is not authenticated"
		hint := "run `jira auth login --profile-name " + name + "` with a current Jira API token"
		if remote, _ := profile["remote"].(map[string]any); remote != nil {
			if remoteErr, _ := remote["error"].(string); !xstrings.IsBlank(remoteErr) {
				if strings.TrimSpace(remoteErr) != strings.TrimSpace(errMsg) {
					errorMessages = append(errorMessages, remoteErr)
				}
			}
			myselfFailed := false
			if probe, _ := remote["myself"].(map[string]any); probe != nil {
				if ok, _ := probe["ok"].(bool); !ok {
					msg = "profile " + name + " failed remote auth probe"
					hint = authStatusHint(probe, hint)
					myselfFailed = true
				}
			}
			if probe, _ := remote["permissions"].(map[string]any); probe != nil {
				if ok, _ := probe["ok"].(bool); !ok && !myselfFailed {
					msg = "profile " + name + " has no CLI-relevant Jira permissions"
					hint = authStatusHint(probe, hint)
				}
			}
		}
		if len(errorMessages) > 0 {
			msg += ": " + strings.Join(errorMessages, "; ")
		}
		out = append(out, cli.Error{
			Type:      string(cli.ErrorTypeAuth),
			Code:      "auth_failed",
			Message:   msg,
			Hint:      hint,
			Retryable: false,
		})
	}
	return out
}

func authStatusHint(fields map[string]any, fallback string) string {
	if hint, _ := fields["hint"].(string); !xstrings.IsBlank(hint) {
		return hint
	}
	return fallback
}

func httpStatusOf(resp *jira.Response) int {
	if resp == nil || resp.Response == nil {
		return 0
	}
	return resp.Response.StatusCode
}

func authLogoutCommand() *cobra.Command {
	var baseURL string
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:   "logout PROFILE",
		Short: "Remove stored credentials",
		Long: "Remove the stored credential for one profile from its configured secret " +
			"backend. Use it when a token is no longer valid or a machine should stop " +
			"using that Jira account.\n\n" +
			"The profile entry remains in config, so site metadata and defaults are kept. " +
			"If no credential exists, the command succeeds and reports `removed: false`.\n\n" +
			"Removing a credential is destructive — getting it back means re-entering the " +
			"token. `--dry-run` reports the credential a live logout would remove without " +
			"touching the backend. A live logout requires `--force` in headless, agent, or " +
			"`--no-input` mode; an interactive terminal proceeds without a prompt — naming " +
			"the profile is the intent.\n\n" +
			"A credential can outlive its profile: deleting the profile from config does " +
			"not remove the stored secret. The keychain entry is keyed by site host and " +
			"profile name, so passing `--base-url` supplies the missing half of that " +
			"identity and lets logout purge the orphaned credential.",
		Example: `$ jira auth logout work

# Report whether a credential was removed
$ jira auth logout work --force --output=json

# Report which credential would be removed, without removing it
$ jira auth logout work --dry-run --output=json

# Purge the credential of a profile already deleted from config
$ jira auth logout old-work --base-url acme.atlassian.net`,
		Args:              cobra.ExactArgs(1),
		Annotations:       map[string]string{"clib": "dynamic-args='profile'"},
		ValidArgsFunction: cmdutil.CompleteProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(args[0])
			if err != nil {
				// A deleted profile leaves its credential behind. With an
				// explicit --base-url the keychain entry (<host>/<profile>)
				// is fully determined without the config entry, so the
				// orphan can still be purged. Without it, keep refusing:
				// fabricating a profile for a credential-admin command is
				// how a typo probes the wrong namespace.
				if !errors.Is(err, config.ErrProfileNotDefined) || baseURL == "" {
					return err
				}
				profile = config.Profile{Name: args[0], SecretBackend: config.SecretBackendKeyring}
			}
			if baseURL != "" {
				// The flag names the credential's site explicitly. This also
				// reaches a credential stranded under a previous host after
				// the profile was re-pointed at a new site.
				profile.BaseURL = baseURL
			}
			ref, err := cmdutil.SecretRefFor(profile, profile.SecretBackend)
			if err != nil {
				return err
			}
			if dryRun {
				// Preview names the credential a live run would revoke
				// without opening the secret backend at all.
				return cmdutil.WriteEnvelope(cmd, "auth.logout", map[string]any{
					"profile": profile.Name,
					"backend": string(profile.SecretBackend),
					"removed": false,
					"dry_run": true,
				})
			}
			// Destructive op safety: recovering a revoked credential means
			// re-entering the token, so headless / agent / --no-input callers
			// must consent with --force. A TTY human proceeds unprompted —
			// logout takes an explicit profile name, so the verb carries its
			// own intent.
			det := cmdutil.DetectorFromContext(cmd)
			if !force && (!det.IsTTY || det.Agent || cmdutil.NoInputRequested(cmd)) {
				return cli.NewCLIInputError(cli.InputForceRequired, "auth logout requires --force in headless / agent / --no-input mode")
			}
			// Revoke the credential from its backend. An absent credential is
			// not an error: removed=false reports there was nothing to remove.
			// A note is returned when revocation left a user-named 1Password
			// item in place; surface it as an informational warning.
			removed, note, err := config.RevokeProfileCredential(cmd.Context(), cmdutil.CredentialStoreFor(profile.SecretBackend), ref)
			if err != nil {
				return err
			}
			if note != "" {
				cmdutil.RecordCredentialWarnings(cmd, []string{note})
			}
			return cmdutil.WriteEnvelope(cmd, "auth.logout", map[string]any{"profile": profile.Name, "removed": removed, "dry_run": false})
		},
	}
	cmdutil.AddStringVar(cmd.Flags(), &baseURL, "base-url", "", "Jira site of the credential to remove; required to purge a credential whose profile was deleted from config", clib.FlagExtra{Group: "Configuration", Placeholder: "URL", Terse: "site URL"})
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Report the credential a live logout would remove without touching the backend")
	cmdutil.AddForceFlag(cmd.Flags(), &force, "Confirm the credential removal in headless / agent / --no-input mode")
	return cmd
}

func authSwitchCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "switch PROFILE",
		Short: "Switch active profile",
		Long: "Set the config file's default profile. Use it when most upcoming commands " +
			"should target a different Jira site or account without passing `--profile` " +
			"each time.\n\n" +
			"This changes local config only. It does not verify credentials or contact " +
			"Jira. `--dry-run` resolves the target profile and reports the switch without " +
			"writing the config file.",
		Example: `$ jira auth switch work

# Return the new active profile as JSON
$ jira auth switch work --output=json

# Resolve and preview the switch without writing config
$ jira auth switch work --dry-run --output=json`,
		Args:              cobra.ExactArgs(1),
		Annotations:       map[string]string{"clib": "dynamic-args='profile'"},
		ValidArgsFunction: cmdutil.CompleteProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(args[0])
			if err != nil {
				return err
			}
			data := map[string]any{
				"active":   profile.Name,
				"previous": cfg.DefaultProfile,
				"dry_run":  dryRun,
			}
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "auth.switch", data)
			}
			cfg.DefaultProfile = profile.Name
			if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "auth.switch", data)
		},
	}
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Resolve and preview the switch without writing config")
	return cmd
}

func authRefreshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Report credential refresh support",
		Long: "Report whether the active profile's auth type has a refresh flow. Use it " +
			"from scripts that share a generic auth lifecycle across CLIs.\n\n" +
			"Jira API-token profiles do not have refreshable credentials, so this command " +
			"returns `refreshed: false` with a reason instead of rotating a token.",
		Example: `# Token profiles report why no refresh happened
$ jira auth refresh

# Check another profile without changing the active one
$ jira --profile prod auth refresh --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(cmdutil.RequestedProfile(cmd))
			if err != nil {
				return err
			}
			data := map[string]any{
				"profile":   profile.Name,
				"auth_type": string(profile.AuthType),
				"refreshed": false,
			}
			data["reason"] = "selected auth type has no refresh flow"
			return cmdutil.WriteEnvelope(cmd, "auth.refresh", data)
		},
	}
}

func authMigrateCommand() *cobra.Command {
	var backend, onePasswordAccount, vault, item string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate credentials between backends",
		Long: "Move stored credentials between supported secret backends while keeping each " +
			"profile's Jira metadata. Use it when switching from the OS keyring to " +
			"1Password, or when moving a profile back to the keyring.\n\n" +
			"`--dry-run` previews the migration plan without reading or writing secrets. " +
			"Real migrations stage destination writes before saving config, then clean up " +
			"old storage where jira-cli owns it.",
		Example: `# Preview migrating every profile's credential to 1Password
$ jira auth migrate --backend 1password --vault Private --dry-run

# Migrate one profile's credential into a 1Password vault item
$ jira auth migrate --profile work --backend 1password --vault Private --item jira-work

# Move a profile's credential back to the OS keyring
$ jira auth migrate --profile work --backend keyring`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target := config.SecretBackend(backend)
			// The env backend cannot be a migration destination: a migration
			// copies the secret into the target store, and env only reads its
			// JIRA_TOKEN_* variable. Re-pointing a profile at env is a login
			// operation (metadata only, old credential revoked), so say so.
			if target == config.SecretBackendEnv {
				return fmt.Errorf("validation: cannot migrate credentials into the env backend — it reads the profile's JIRA_TOKEN_* variable at run time; export the variable and run `jira auth login --backend env` to re-point the profile")
			}
			if target != config.SecretBackendKeyring && target != config.SecretBackendOnePassword {
				return fmt.Errorf("unsupported secret backend %q", backend)
			}
			// Fail before any secret is read when the destination cannot store
			// in this build or environment (no-CGO 1Password, keyring with no
			// Secret Service). Dry-run skips the probe: it is a local preview
			// and must not depend on backend health.
			if !dryRun {
				if err := ensureBackendStorable(target); err != nil {
					return err
				}
			}
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			// When --profile is explicitly set, resolve it so a typo is
			// rejected here rather than silently matching no profile and
			// returning success with an empty result.
			profileName := cmdutil.RequestedProfile(cmd)
			if profileName != "" {
				resolved, rerr := cfg.ResolveProfile(profileName)
				if rerr != nil {
					return rerr
				}
				profileName = resolved.Name
			}
			// --item names ONE 1Password item; applying it across a whole
			// batch would point every profile.s credential at a single
			// vault item. Require a single selected profile when --item is set.
			if item != "" && profileName == "" && len(cfg.Profiles) > 1 {
				return fmt.Errorf("--item names a single 1Password item; use --profile to select one profile, or omit --item to let each profile use its own item")
			}
			ops := make([]map[string]any, 0, len(cfg.Profiles))
			// migrations collects the profiles that need a real backend
			// switch. The destination write for every one of them is staged
			// before the single config.Save, so a save failure rolls the
			// whole batch back rather than stranding new secrets. Each
			// CredentialMigration carries its profile index, so migrations is
			// the single ordered source of truth for which profiles flip.
			// migratingOps holds the matching op result entries, appended in
			// lockstep with migrations so the two never drift.
			var migrations []config.CredentialMigration
			var migratingOps []map[string]any
			for i := range cfg.Profiles {
				profile := &cfg.Profiles[i]
				if profileName != "" && profile.Name != profileName {
					continue
				}
				op := map[string]any{
					"profile":        profile.Name,
					"source_backend": string(profile.SecretBackend),
					"target_backend": string(target),
					"dry_run":        dryRun,
					"migrated":       false,
				}
				if profile.SecretBackend == target {
					op["reason"] = "already using target backend"
					ops = append(ops, op)
					continue
				}
				if target == config.SecretBackendOnePassword {
					if onePasswordAccount != "" {
						profile.OnePasswordAccount = onePasswordAccount
					}
					if vault != "" {
						profile.Vault = vault
					}
					if item != "" {
						profile.Item = item
					}
					// Leave profile.Item empty when the user did not name an
					// item: CredentialIdentity derives the default name and
					// marks the resulting 1Password item jira-cli-owned, so a
					// later migrate-away can safely delete it. Persisting the
					// default name would make jira-cli's own item look
					// user-named and strand it on the next migration.
					if profile.Vault == "" {
						op["error"] = "1Password migration requires --vault or existing profile vault metadata"
						ops = append(ops, op)
						continue
					}
				}
				if dryRun {
					op["reason"] = "dry run"
					ops = append(ops, op)
					continue
				}
				sourceRef, refErr := cmdutil.SecretRefFor(*profile, profile.SecretBackend)
				if refErr != nil {
					op["error"] = refErr.Error()
					ops = append(ops, op)
					continue
				}
				destRef, refErr := cmdutil.SecretRefFor(*profile, target)
				if refErr != nil {
					op["error"] = refErr.Error()
					ops = append(ops, op)
					continue
				}
				migrations = append(migrations, config.CredentialMigration{
					Profile:      profile.Name,
					ProfileIndex: i,
					Source:       cmdutil.CredentialStoreFor(profile.SecretBackend),
					Destination:  cmdutil.CredentialStoreFor(target),
					SourceRef:    sourceRef,
					DestRef:      destRef,
				})
				migratingOps = append(migratingOps, op)
				ops = append(ops, op)
			}
			cleanupFailures := []string{}
			cleanupNotes := []string{}
			if !dryRun && len(migrations) > 0 {
				report, migErr := config.MigrateCredentials(cmd.Context(), migrations, func() error {
					// The destination secrets are staged; persist the new
					// backend metadata. Only a durable save here lets the
					// source secrets be cleaned up.
					applyMigratedBackends(cfg, migrations, target)
					return config.Save(cmdutil.ConfigPath(cmd), cfg)
				})
				if migErr != nil {
					return fmt.Errorf("auth migrate: %w", migErr)
				}
				for _, op := range migratingOps {
					op["migrated"] = true
				}
				for _, failure := range report.CleanupFailures {
					cleanupFailures = append(cleanupFailures, failure.Profile)
				}
				// A cleanup note marks source storage left in place because
				// jira-cli does not own it. Surface it to the user so they
				// can remove the old credential by hand if they want to.
				for _, note := range report.CleanupNotes {
					cleanupNotes = append(cleanupNotes, note.Message)
				}
				cmdutil.RecordCredentialWarnings(cmd, cleanupNotes)
			} else if !dryRun {
				if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
					return err
				}
			}
			data := map[string]any{
				"target_backend": string(target),
				"dry_run":        dryRun,
				"profiles":       ops,
			}
			if len(cleanupFailures) > 0 {
				data["cleanup_failures"] = cleanupFailures
			}
			if len(cleanupNotes) > 0 {
				data["cleanup_notes"] = cleanupNotes
			}
			return cmdutil.WriteEnvelope(cmd, "auth.migrate", data)
		},
	}
	cmdutil.AddStringVar(cmd.Flags(), &backend, "backend", string(config.SecretBackendKeyring), "Target secret backend for the credential", clib.FlagExtra{Placeholder: "BACKEND", Terse: "secret backend", Enum: []string{"keyring", "1password"}, EnumTerse: []string{"OS keychain", "1Password CLI"}, EnumDefault: "keyring"})
	cmdutil.AddStringVar(cmd.Flags(), &onePasswordAccount, "onepassword-account", "", "1Password desktop app account name", clib.FlagExtra{Group: "1Password", Placeholder: "ACCOUNT"})
	cmdutil.AddStringVar(cmd.Flags(), &vault, "vault", "", "1Password vault name", clib.FlagExtra{Group: "1Password", Placeholder: "VAULT"})
	cmdutil.AddStringVar(cmd.Flags(), &item, "item", "", "1Password item name", clib.FlagExtra{Group: "1Password", Placeholder: "NAME"})
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview credential migration")
	return cmd
}

func authTokenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Show redacted token diagnostics",
		Long: "Inspect where the active profile's credential is stored and whether it can " +
			"be resolved. Use it to debug local secret backend problems without printing " +
			"the token.\n\n" +
			"The token value is always redacted. This command checks local credential " +
			"resolution only; use `jira auth status` when you also need live Jira probes.",
		Example: `$ jira auth token

# Check another profile without printing the secret
$ jira --profile prod auth token --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(cmdutil.RequestedProfile(cmd))
			if err != nil {
				return err
			}
			ref, refErr := cmdutil.SecretRefFor(profile, profile.SecretBackend)
			if refErr != nil {
				return refErr
			}
			status := config.CredentialStatus(cmd.Context(), cmdutil.CredentialStoreFor(profile.SecretBackend), ref)
			data := map[string]any{
				"profile":  profile.Name,
				"source":   status.Source,
				"backend":  string(profile.SecretBackend),
				"valid":    status.Valid,
				"redacted": status.Redacted,
				"expiry":   nil,
				"error":    status.Error,
			}
			// 1Password coordinates (account/vault/item) are meaningful
			// only for a 1Password-backed profile. Populating them on a
			// keyring profile prints fields irrelevant to the active
			// backend, so scope them to the 1Password backend.
			if profile.SecretBackend == config.SecretBackendOnePassword {
				data["onepassword_account"] = profile.OnePasswordAccount
				data["vault"] = profile.Vault
				data["item"] = cmdutil.FirstNonEmpty(profile.Item, "jira-cli-"+profile.Name)
			}
			// For the env backend the diagnostic that matters is which
			// variable the credential is read from — name it explicitly.
			if profile.SecretBackend == config.SecretBackendEnv {
				data["credential_env"] = ref.EnvTokenKey()
			}
			return cmdutil.WriteEnvelope(cmd, "auth.token", data)
		},
	}
}

// profileBaseURLEnvVar returns the name of the environment variable that
// overrides a profile's base_url. It mirrors the JIRA_PROFILE_<NAME>_*
// convention parsed by the config loader: the profile name is uppercased
// with '-' replaced by '_'.
func profileBaseURLEnvVar(name string) string {
	return "JIRA_PROFILE_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_BASE_URL"
}

// revokeOldCredentialOnRelogin revokes the previous credential after an auth
// login re-points an existing profile at a different credential identity (a
// new site, a backend switch, a different 1Password account/vault/item). The
// old credential would otherwise linger as a live secret in the old keyring
// entry / old 1Password item. It is called only after the new credential
// write and config save have committed.
//
// It returns an informational note when cleanup of the old credential failed;
// an empty string means there was nothing to revoke (fresh profile, unchanged
// identity) or the revocation succeeded. Cleanup failure is surfaced, never
// fatal — the login itself already succeeded.
func revokeOldCredentialOnRelogin(cmd *cobra.Command, previous, updated config.Profile) string {
	// A profile that was never persisted (no prior base_url) has no old
	// credential to revoke.
	if previous.BaseURL == "" {
		return ""
	}
	oldRef, err := cmdutil.SecretRefFor(previous, previous.SecretBackend)
	if err != nil {
		return ""
	}
	newRef, err := cmdutil.SecretRefFor(updated, updated.SecretBackend)
	if err != nil {
		return ""
	}
	return revokeOldCredential(cmd.Context(), cmdutil.CredentialStoreFor(previous.SecretBackend), oldRef, newRef)
}

// revokeOldCredential revokes the credential at oldRef when it addresses
// storage different from newRef. For the keyring backend the old entry is
// deleted; for 1Password the old item's managed credential field is stripped
// (the item itself is never destroyed). When the identities are the same it
// does nothing — the credential was not re-pointed. A cleanup failure is
// returned as an informational note, not an error: the login already
// succeeded and must not be undone by a failed best-effort cleanup.
func revokeOldCredential(ctx context.Context, store config.CredentialStore, oldRef, newRef config.SecretRef) string {
	if !config.CredentialIdentitiesDiffer(oldRef, newRef) {
		return ""
	}
	if _, _, err := config.RevokeProfileCredential(ctx, store, oldRef); err != nil {
		// SanitizeCredentialError keeps the note to the typed display
		// message: the full error chain would replay the hint text and the
		// raw backend cause (a D-Bus dump on WSL) inside a one-line warning.
		return fmt.Sprintf(
			"the previous credential for profile %q (%s, site %s) could not be removed: %s — remove it manually if one was stored",
			oldRef.Profile, oldRef.Backend, oldRef.Host, config.SanitizeCredentialError(err),
		)
	}
	return ""
}

// applyMigratedBackends flips secret_backend to target on exactly the
// profiles named by migrations, located by the ProfileIndex each migration
// carries. Profiles not in the batch are left untouched.
func applyMigratedBackends(cfg *config.Config, migrations []config.CredentialMigration, target config.SecretBackend) {
	for _, m := range migrations {
		if m.ProfileIndex >= 0 && m.ProfileIndex < len(cfg.Profiles) {
			cfg.Profiles[m.ProfileIndex].SecretBackend = target
		}
	}
}
