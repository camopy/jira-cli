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
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

// NewCommand returns the `auth` command group for managing Jira authentication.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("auth", "Manage Jira authentication", "configuration")
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
		Short: "Show the authenticated user's identity from /myself",
		Args:  cobra.NoArgs,
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
				return fmt.Errorf("jira base URL is required for auth.whoami")
			}
			user, _, err := jira.NewUserService(client).Myself(cmd.Context())
			if err != nil {
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
	cmd.Flags().BoolVar(&save, "save", false, "Persist the resolved account_id (and email if blank) to the active profile")
	return cmd
}

func authLoginCommand() *cobra.Command {
	var profileName, baseURL, email, backend, onePasswordAccount, vault, item, credential, credentialEnv, jsonInput string
	var secretStdin, skipVerify bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Configure authentication for a profile",
		Args:  cobra.NoArgs,
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
			if noInput && strings.TrimSpace(baseURL) == "" {
				return fmt.Errorf("validation: --no-input requires --base-url (or --json-input with base_url)")
			}
			if profileName == "" {
				profileName = "default"
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
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
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
			if cmd.Flags().Changed("backend") {
				profile.SecretBackend = flagBackend
			} else if profile.SecretBackend == "" {
				profile.SecretBackend = flagBackend
			}
			// An explicit --backend that differs from the profile's stored
			// backend would silently relocate a live credential to a different
			// store. Moving a secret between backends is `auth migrate`'s job —
			// it stages the new write and cleans up the old one transactionally
			// — so login refuses it rather than switching as a side-effect.
			if cmd.Flags().Changed("backend") && previousProfile.SecretBackend != "" && flagBackend != previousProfile.SecretBackend {
				return fmt.Errorf("validation: profile %q stores its credential in %s; run `jira auth migrate --backend %s` to move it", profile.Name, previousProfile.SecretBackend, flagBackend)
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
			// A Jira Cloud API token is the basic-auth password, paired with
			// the account email as the username. Storing a token without an
			// email yields a credential that can never authenticate, so refuse
			// it before any network call or storage — even under --skip-verify.
			if credential != "" && strings.TrimSpace(profile.Email) == "" {
				return fmt.Errorf("validation: an account email is required to store a Jira Cloud API token; pass --email")
			}
			// The token is about to be sent over the network (verification) and
			// stored. Reject an unsafe base URL now — cfg.Validate runs only
			// after verification, so without this check the headless path would
			// dial a cleartext non-loopback host and leak the credential before
			// the URL is rejected. The interactive form already validates this.
			if credential != "" {
				if err := config.ValidateBaseURL(profile.BaseURL); err != nil {
					return fmt.Errorf("validation: jira base URL: %w", err)
				}
			}
			// Confirm the token actually authenticates before persisting it, so
			// a rejected token fails the login here instead of silently on first
			// real use. The check runs against the in-memory credential, so it
			// covers every backend (keyring, 1Password) identically. The
			// resolved accountId is folded into the profile so `--assignee me`
			// works immediately without a separate `auth whoami`. --skip-verify
			// opts out for offline setup or an unreachable endpoint.
			var verifiedUser *jira.CurrentUser
			if credential != "" && !skipVerify {
				// .Silent() runs the task and returns its error without logging
				// a completion line — the failure is surfaced through the normal
				// command error path instead of a duplicate spinner line.
				verifyErr := clog.Spinner("Verifying Jira credentials").
					NonTTYSilent(true).
					Wait(cmd.Context(), func(ctx context.Context) error {
						user, err := verifyCredential(ctx, profile.BaseURL, profile.Email, credential, time.Duration(profile.TimeoutSeconds)*time.Second)
						if err != nil {
							return err
						}
						verifiedUser = user
						return nil
					}).Silent()
				if verifyErr != nil {
					// Distinguish a rejected credential (the email/token pair is
					// wrong) from a verification that could not complete (site
					// down, network, 5xx). Jira's raw 401 text ("Client must be
					// authenticated") reads cryptically, so name the real cause.
					var apiErr *jira.APIError
					if errors.As(verifyErr, &apiErr) && apiErr.Type == jira.ErrorTypeAuth {
						return &config.CredentialError{
							Type:        config.ErrorTypeAuth,
							ErrCode:     config.ErrorCode("auth_failed"),
							Message:     fmt.Sprintf("invalid Atlassian account email or API token - Jira rejected the credential (HTTP %d)", apiErr.StatusCode),
							HintMsg:     "check the email and that the API token is current at id.atlassian.com, or pass --skip-verify to store it without checking",
							IsRetryable: false,
						}
					}
					return &config.CredentialError{
						Type:        config.ErrorTypeAuth,
						ErrCode:     config.ErrorCodeCredentialBackendUnavailable,
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
			// On a human terminal, confirm who the verified token belongs to so
			// the user sees the identity instead of a silent return. Machine
			// modes consume the same identity from the envelope below.
			if verifiedUser != nil {
				if mode := cmdutil.DetectorFromContext(cmd).Mode; mode == cli.ModePlain || mode == cli.ModeTUI {
					name := cmdutil.FirstNonEmpty(verifiedUser.DisplayName, verifiedUser.EmailAddress, profile.Email)
					clog.Info().Parts(clog.PartMessage).Msg("✓ Logged in as " + name)
				}
			}
			data := map[string]any{
				"profile":             profileName,
				"auth_type":           string(config.AuthTypeToken),
				"secret_backend":      string(profile.SecretBackend),
				"onepassword_account": onePasswordAccount,
				"stored_secret":       credential != "",
				"verified":            verifiedUser != nil,
				"skip_verify":         skipVerify,
			}
			if verifiedUser != nil {
				data["account_id"] = verifiedUser.AccountID
				data["display_name"] = verifiedUser.DisplayName
			}
			return cmdutil.WriteEnvelope(cmd, "auth.login", data)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile-name", "default", "Profile name to configure")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Jira base URL")
	cmd.Flags().StringVar(&email, "email", "", "Jira Cloud account email")
	cmd.Flags().StringVar(&backend, "backend", string(config.SecretBackendKeyring), "Secret backend: keyring or 1password")
	cmd.Flags().StringVar(&onePasswordAccount, "onepassword-account", "", "1Password desktop app account name")
	cmd.Flags().StringVar(&vault, "vault", "", "1Password vault name")
	cmd.Flags().StringVar(&item, "item", "", "1Password item name")
	cmd.Flags().BoolVar(&secretStdin, "secret-stdin", false, "Read credential from stdin")
	cmd.Flags().BoolVar(&skipVerify, "skip-verify", false, "Store the credential without verifying it against /myself first")
	cmd.Flags().StringVar(&credentialEnv, "credential-env", "", "Read credential from environment variable")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read auth profile metadata from JSON file")
	// --secret-stdin and --credential-env both supply the credential. Passing
	// both is a syntactic conflict: one would silently win by processing
	// order, so reject it in Cobra validation before any source is read.
	// --json-input carries only profile metadata, never the credential, so it
	// is not part of this group.
	cmd.MarkFlagsMutuallyExclusive("secret-stdin", "credential-env")
	clib.Extend(cmd.Flags().Lookup("profile-name"), clib.FlagExtra{Placeholder: "NAME", Complete: "predictor=profile"})
	clib.Extend(cmd.Flags().Lookup("backend"), clib.FlagExtra{Placeholder: "BACKEND", Enum: []string{"keyring", "1password"}, EnumDefault: "keyring"})
	return cmd
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
	form := authLoginForm(skipVerify, profileName, nameHint, baseURL, email, backend, onePasswordAccount, vault, item, credential, &confirmed).
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
	trimAuthLoginValues(profileName, baseURL, email, backend, onePasswordAccount, vault, item, credential)
	*profileName = resolveProfileName(*profileName, nameHint)
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
		Value(value)
}

func authLoginForm(skipVerify bool, profileName *string, profileNameHint string, baseURL, email, backend, onePasswordAccount, vault, item, credential *string, confirmed *bool) *huh.Form {
	// Start the profile-name field empty with the resolved default shown only as
	// a placeholder: huh appends keystrokes to a pre-filled value, so pre-filling
	// the name would mangle a typed one and could overwrite the wrong profile.
	// promptAuthLogin restores the hint for a blank field after the form.
	*profileName = ""
	confirmDescription := "The token is verified against Jira before it is saved."
	if skipVerify {
		confirmDescription = "The credential is stored without verification (--skip-verify)."
	}
	return huh.NewForm(
		huh.NewGroup(
			profileNameInput(profileName, profileNameHint),
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
				Options(authLoginHuhOptions(authLoginQuestionByID(authLoginQuestions(), "secret_backend").Options)...).
				Value(backend).
				Validate(validateSecretBackend),
			huh.NewInput().
				Title("API token").
				Description("Atlassian API token to store in the selected secret backend.").
				EchoMode(huh.EchoModePassword).
				Value(credential).
				Validate(requiredString("API token is required")),
		).Title("Credential storage").Description("Secrets are written to the selected backend, not to config.toml."),
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
					if config.SecretBackend(*backend) == config.SecretBackendOnePassword && strings.TrimSpace(value) == "" {
						return errors.New("1Password vault is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("1Password item").
				Description("Item title for this Jira profile in the selected 1Password vault.").
				Value(item).
				Validate(func(value string) error {
					if config.SecretBackend(*backend) == config.SecretBackendOnePassword && strings.TrimSpace(value) == "" {
						return errors.New("1Password item is required")
					}
					return nil
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
					return loginReviewSummary(resolveProfileName(*profileName, profileNameHint), *baseURL, *email, *backend, *onePasswordAccount, *vault, *item)
				}, []*string{profileName, baseURL, email, backend, onePasswordAccount, vault, item}),
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
		if strings.TrimSpace(value) == "" {
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
	case config.SecretBackendKeyring, config.SecretBackendOnePassword:
		return nil
	default:
		return fmt.Errorf("unsupported secret backend %q", value)
	}
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
		case strings.TrimSpace(requestedProfile) != "":
			name = strings.TrimSpace(requestedProfile)
		case strings.TrimSpace(cfg.DefaultProfile) != "":
			name = strings.TrimSpace(cfg.DefaultProfile)
		case strings.TrimSpace(name) == "":
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
		Long: `Reports credential resolution and (by default) probes the live Jira API
to surface how the token actually behaves end-to-end. Pass --no-probe
to skip remote calls and run only the local credential check.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profiles := make([]map[string]any, 0, len(cfg.Profiles))
			for _, profile := range cfg.Profiles {
				entry := map[string]any{"profile": profile.Name}
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
					remote := probeRemoteAuth(cmd, profile, projectKey)
					entry["remote"] = remote
					if !remoteAuthValid(remote) {
						entry["valid"] = false
					}
				}
				profiles = append(profiles, entry)
			}
			data := map[string]any{
				"active_profile": cfg.DefaultProfile,
				"profiles":       profiles,
			}
			statusErrors := authStatusErrors(profiles)
			if len(statusErrors) > 0 {
				if err := cmdutil.WriteEnvelopeWithErrors(cmd, "auth.status", data, statusErrors); err != nil {
					return err
				}
				return cmdutil.EnvelopeWritten(fmt.Errorf("auth status found %d unhealthy profile(s)", len(statusErrors)))
			}
			return cmdutil.WriteEnvelope(cmd, "auth.status", data)
		},
	}
	cmd.Flags().BoolVar(&noProbe, "no-probe", false, "Skip the remote /myself + /mypermissions check")
	cmd.Flags().StringVar(&projectKey, "project", "", "Probe permissions in the context of this project")
	clib.Extend(cmd.Flags().Lookup("project"), clib.FlagExtra{Placeholder: "KEY", Complete: "predictor=cacheproject"})
	return cmd
}

// probeRemoteAuth makes lightweight HTTP calls to surface how the token
// actually behaves, distinguishing "credential resolves locally" from
// "credential is accepted by Jira and grants the permissions the CLI
// needs". Returns a map suitable for inclusion in the auth.status
// envelope. Never errors — every failure becomes a structured field so
// the user can see what went wrong without rerunning anything.
func probeRemoteAuth(cmd *cobra.Command, profile config.Profile, projectKey string) map[string]any {
	out := map[string]any{"site": profile.BaseURL}

	// Route the probe through the same client constructor normal commands
	// use, so the per-profile request timeout and an mTLS client certificate
	// both apply to the probe's live calls rather than silently defaulting.
	client, _, ok, err := cmdutil.JiraClientForProfile(cmd, profile)
	if err != nil {
		out["error"] = config.SanitizeCredentialError(err)
		return out
	}
	if !ok || client == nil {
		out["error"] = "profile has no base URL to probe"
		return out
	}
	user := jira.NewUserService(client)

	// /myself reveals scope-level auth issues (granular tokens missing
	// read:user/group/avatar/application-role 4-scope union).
	myselfOut := map[string]any{}
	if me, resp, err := user.Myself(cmd.Context()); err != nil {
		myselfOut["ok"] = false
		myselfOut["status"] = httpStatusOf(resp)
		myselfOut["error"] = err.Error()
		if httpStatusOf(resp) == 401 {
			myselfOut["hint"] = "token can't read /myself — for granular tokens, add read:user:jira, read:application-role:jira, read:group:jira, read:avatar:jira (Atlassian requires the union)"
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
	if pr, resp, err := user.MyPermissions(cmd.Context(), projectKey, keys); err != nil {
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
			permsOut["hint"] = "token authenticates but has zero CLI-relevant permissions — check the Resources tab of your scoped token at id.atlassian.com (KAN/SAM1 etc. must be selected)"
		}
	}
	out["permissions"] = permsOut

	return out
}

func remoteAuthValid(remote map[string]any) bool {
	if remote == nil {
		return true
	}
	if errText, _ := remote["error"].(string); strings.TrimSpace(errText) != "" {
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
		if strings.TrimSpace(errMsg) != "" {
			errorMessages = append(errorMessages, errMsg)
		}
		msg := "profile " + name + " is not authenticated"
		hint := "run `jira auth login --profile-name " + name + "` with a current Jira API token"
		if remote, _ := profile["remote"].(map[string]any); remote != nil {
			if remoteErr, _ := remote["error"].(string); strings.TrimSpace(remoteErr) != "" {
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
	if hint, _ := fields["hint"].(string); strings.TrimSpace(hint) != "" {
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
	return &cobra.Command{
		Use:               "logout PROFILE",
		Short:             "Remove stored credentials",
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
				return err
			}
			ref, err := cmdutil.SecretRefFor(profile, profile.SecretBackend)
			if err != nil {
				return err
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
			return cmdutil.WriteEnvelope(cmd, "auth.logout", map[string]any{"profile": profile.Name, "removed": removed})
		},
	}
}

func authSwitchCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "switch PROFILE",
		Short:             "Switch active profile",
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
			cfg.DefaultProfile = profile.Name
			if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "auth.switch", map[string]any{"active": profile.Name})
		},
	}
}

func authRefreshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Refresh credentials",
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target := config.SecretBackend(backend)
			if target != config.SecretBackendKeyring && target != config.SecretBackendOnePassword {
				return fmt.Errorf("unsupported secret backend %q", backend)
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
	cmd.Flags().StringVar(&backend, "backend", string(config.SecretBackendKeyring), "Target secret backend: keyring or 1password")
	cmd.Flags().StringVar(&onePasswordAccount, "onepassword-account", "", "1Password desktop app account name")
	cmd.Flags().StringVar(&vault, "vault", "", "1Password vault name")
	cmd.Flags().StringVar(&item, "item", "", "1Password item name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview credential migration")
	clib.Extend(cmd.Flags().Lookup("backend"), clib.FlagExtra{Placeholder: "BACKEND", Enum: []string{"keyring", "1password"}, EnumDefault: "keyring"})
	return cmd
}

func authTokenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Show redacted token diagnostics",
		Args:  cobra.NoArgs,
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
		return fmt.Sprintf(
			"the previous credential for profile %q (%s, site %s) could not be removed: %v — remove it manually",
			oldRef.Profile, oldRef.Backend, oldRef.Host, err,
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
